package browser

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"strings"

	"surf-backend/internal/cdp"
	"surf-backend/internal/protocol"
)

const editableBinding = "__surfEditableChanged"
const editableWorld = "surf-editable"
const editableIntentPlaceholder = "__SURF_KEYBOARD_INTENT_EVENT__"

// editableIntentSensor runs in the page's main world, where it can distinguish
// a focus call still nested in a trusted event listener from later promise,
// timer, or animation work. It never exposes Surf's binding. Direct fields and
// labels are authorized before their browser default action; synchronous site
// handlers are authorized while the original event still has a currentTarget.
const editableIntentSensor = `(function(){
  if(window.__surfKeyboardIntentSensor)return;
  window.__surfKeyboardIntentSensor=true;
  var activationEvent=null;
  var dispatch=EventTarget.prototype.dispatchEvent,IntentEvent=CustomEvent;
  function focused(){
    var e=document.activeElement;
    while(e&&e.shadowRoot&&e.shadowRoot.activeElement)e=e.shadowRoot.activeElement;
    return e;
  }
  function editable(e){
    if(!e||e.disabled||e.readOnly)return null;
    var t=(e.tagName||'').toUpperCase();
    if(t==='TEXTAREA'||e.isContentEditable)return e;
    if(t!=='INPUT')return null;
    var ty=(e.type||'text').toLowerCase();
    var skip={button:1,checkbox:1,radio:1,submit:1,reset:1,file:1,image:1,range:1,color:1,hidden:1,date:1,time:1};
    return skip[ty]?null:e;
  }
  function eventEditable(event){
    var path=event.composedPath?event.composedPath():[event.target];
    for(var i=0;i<path.length;i++){
      var e=path[i];
      if(e&&(e.tagName||'').toUpperCase()==='LABEL'&&e.control){var c=editable(e.control);if(c)return c;}
      var candidate=editable(e);if(candidate)return candidate;
    }
    return null;
  }
  function matches(a,b){
    if(a===b)return true;
    return !!(a&&b&&((a.isContentEditable&&a.contains&&a.contains(b))||(b.isContentEditable&&b.contains&&b.contains(a))));
  }
  function retapsFocused(event){
    var e=editable(focused());
    if(!e)return false;
    if(matches(e,eventEditable(event)))return true;
    var point=(event.changedTouches&&event.changedTouches[0])||event;
    if(typeof point.clientX!=='number'||typeof point.clientY!=='number')return false;
    var r=e.getBoundingClientRect();
    return r.width>0&&r.height>0&&point.clientX>=r.left&&point.clientX<=r.right&&point.clientY>=r.top&&point.clientY<=r.bottom;
  }
  function authorize(element){
    dispatch.call(element,new IntentEvent('` + editableIntentPlaceholder + `',{bubbles:true,composed:true}));
  }
  function activate(event){
    if(!event.isTrusted)return;
    if(event.type.indexOf('pointer')===0&&event.isPrimary===false)return;
    if(event.type.indexOf('touch')===0&&event.touches&&event.touches.length>1)return;
    activationEvent=event;
    var direct=eventEditable(event);
    if(direct)authorize(direct);
    else if((event.type==='pointerdown'||event.type==='touchstart'||event.type==='mousedown')&&retapsFocused(event))authorize(editable(focused()));
  }
  function focusChanged(){
    var e=editable(focused());
    if(e&&activationEvent&&activationEvent.currentTarget!==null)authorize(e);
  }
  var events=['pointerdown','pointerup','touchstart','touchend','mousedown','mouseup','click'];
  for(var i=0;i<events.length;i++)window.addEventListener(events[i],activate,true);
  document.addEventListener('focusin',focusChanged,true);
})()`

// editableObserver follows actual focus while keeping keyboard intent separate.
// A site may intentionally focus a field during a physical activation, but
// later script work and ambient autofocus cannot raise the native keyboard.
// The observer runs in an isolated world, walks open shadow roots, and is
// installed in every same-target frame.
const editableObserver = `(function(){
  if(window.__surfEditableObserver)return;
  window.__surfEditableObserver=true;
  var authorized=null;
  function focused(){
    var e=document.activeElement;
    while(e&&e.shadowRoot&&e.shadowRoot.activeElement)e=e.shadowRoot.activeElement;
    return e;
  }
  function editable(e){
    if(!e||e.disabled||e.readOnly)return null;
    var t=(e.tagName||'').toUpperCase();
    if(t==='TEXTAREA')return {element:e,kind:'textarea'};
    if(e.isContentEditable)return {element:e,kind:'text'};
    if(t==='INPUT'){
      var ty=(e.type||'text').toLowerCase();
      var skip={button:1,checkbox:1,radio:1,submit:1,reset:1,file:1,image:1,range:1,color:1,hidden:1,date:1,time:1};
      if(!skip[ty])return {element:e,kind:({password:'password',email:'email',number:'number',tel:'tel',url:'url',search:'search'})[ty]||'text'};
    }
    return null;
  }
  function matches(a,b){
    if(a===b)return true;
    return !!(a&&b&&((a.isContentEditable&&a.contains&&a.contains(b))||(b.isContentEditable&&b.contains&&b.contains(a))));
  }
  function state(){
    var item=editable(focused());
    if(!item){authorized=null;return {on:false};}
    var r=item.element.getBoundingClientRect();
    var show=matches(item.element,authorized);authorized=null;
    var v=window.visualViewport;
    var w=(v&&v.width)||window.innerWidth||1,h=(v&&v.height)||window.innerHeight||1;
    return {on:true,show:show,kind:item.kind,rect:[r.left/w,r.top/h,r.width/w,r.height/h]};
  }
  function report(){try{window.__surfEditableChanged(JSON.stringify(state()));}catch(_){}}
  function soon(){setTimeout(report,0);}
  function authorize(event){
    var item=editable(event.target)||editable(focused());
    if(item){authorized=item.element;soon();}
  }
  document.addEventListener('` + editableIntentPlaceholder + `',authorize,true);
  document.addEventListener('focusin',soon,true);
  document.addEventListener('focusout',soon,true);
  window.addEventListener('pagehide',function(){try{window.__surfEditableChanged('{"on":false}');}catch(_){}},false);
  if(window.visualViewport){window.visualViewport.addEventListener('resize',soon,false);window.visualViewport.addEventListener('scroll',soon,false);}
  report();
})()`

func newEditableScripts() (string, string, string) {
	event := "__surf_keyboard_intent_" + rand.Text()
	return event, strings.ReplaceAll(editableIntentSensor, editableIntentPlaceholder, event),
		strings.ReplaceAll(editableObserver, editableIntentPlaceholder, event)
}

func (b *Controller) setupEditableObserver(session string) {
	_, sensor, observer := newEditableScripts()
	_, selectSensor, selectObserver := newSelectScripts()
	_ = b.cdp.Dispatch(session, "Runtime.addBinding", editableBindingParams())
	_ = b.cdp.Dispatch(session, "Runtime.addBinding", selectBindingParams())
	_ = b.cdp.Dispatch(session, "Page.addScriptToEvaluateOnNewDocument", editableSensorParams(sensor, true))
	_ = b.cdp.Dispatch(session, "Page.addScriptToEvaluateOnNewDocument", editableObserverParams(observer, true))
	_ = b.cdp.Dispatch(session, "Page.addScriptToEvaluateOnNewDocument", selectSensorParams(selectSensor, true))
	_ = b.cdp.Dispatch(session, "Page.addScriptToEvaluateOnNewDocument", selectObserverParams(selectObserver, true))
}

func editableBindingParams() map[string]any {
	return map[string]any{"name": editableBinding, "executionContextName": editableWorld}
}

func editableSensorParams(source string, runImmediately bool) map[string]any {
	params := map[string]any{"source": source}
	if runImmediately {
		params["runImmediately"] = true
	}
	return params
}

func editableObserverParams(source string, runImmediately bool) map[string]any {
	params := map[string]any{
		"source": source, "worldName": editableWorld,
	}
	if runImmediately {
		params["runImmediately"] = true
	}
	return params
}

// Target.setAutoAttach is scoped to a target. Enabling it on every page and
// attached iframe observes isolated cross-origin frames recursively without
// auto-attaching unrelated top-level tabs.
func (b *Controller) setupEditableAutoAttach(session string) {
	_ = b.cdp.Dispatch(session, "Target.setAutoAttach", map[string]any{
		"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true,
		"filter": []any{
			map[string]any{"type": "iframe", "exclude": false},
			map[string]any{"exclude": true},
		},
	})
}

func (b *Controller) onEditableTargetAttached(event cdp.Event) {
	var attached struct {
		SessionID  string `json:"sessionId"`
		TargetInfo struct {
			Type string `json:"type"`
		} `json:"targetInfo"`
		WaitingForDebugger bool `json:"waitingForDebugger"`
	}
	if json.Unmarshal(event.Params, &attached) != nil || attached.SessionID == "" || attached.TargetInfo.Type != "iframe" {
		return
	}
	b.mu.Lock()
	tab := b.bySession[event.SessionID]
	if tab != nil {
		b.bySession[attached.SessionID] = tab
	}
	b.mu.Unlock()
	if tab == nil {
		return
	}
	go b.initializeEditableTarget(attached.SessionID, attached.WaitingForDebugger)
}

func (b *Controller) initializeEditableTarget(session string, waitingForDebugger bool) {
	_, sensor, observer := newEditableScripts()
	_, selectSensor, selectObserver := newSelectScripts()
	commands := []struct {
		method string
		params any
	}{
		{"Runtime.enable", nil},
		{"Runtime.addBinding", editableBindingParams()},
		{"Runtime.addBinding", selectBindingParams()},
		{"Page.addScriptToEvaluateOnNewDocument", editableSensorParams(sensor, !waitingForDebugger)},
		{"Page.addScriptToEvaluateOnNewDocument", editableObserverParams(observer, !waitingForDebugger)},
		{"Page.addScriptToEvaluateOnNewDocument", selectSensorParams(selectSensor, !waitingForDebugger)},
		{"Page.addScriptToEvaluateOnNewDocument", selectObserverParams(selectObserver, !waitingForDebugger)},
		{"Target.setAutoAttach", map[string]any{
			"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true,
			"filter": []any{
				map[string]any{"type": "iframe", "exclude": false},
				map[string]any{"exclude": true},
			},
		}},
	}
	for _, command := range commands {
		if _, err := b.cdp.Call(session, command.method, command.params); err != nil {
			log.Printf("editable: initialize iframe %s: %v", command.method, err)
			return
		}
	}
	if waitingForDebugger {
		if err := b.cdp.Dispatch(session, "Runtime.runIfWaitingForDebugger", nil); err != nil {
			log.Printf("editable: resume iframe: %v", err)
			return
		}
		if err := b.evaluateEditableCurrentDocument(session, sensor, observer, selectSensor, selectObserver); err != nil {
			// A navigation may replace this context between resume and evaluate.
			// The registered new-document scripts still cover the committed page.
			log.Printf("editable: initialize resumed iframe: %v", err)
		}
	}
}

func (b *Controller) evaluateEditableCurrentDocument(session, sensor, observer, selectSensor, selectObserver string) error {
	if _, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{"expression": sensor}); err != nil {
		return err
	}
	if _, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{"expression": selectSensor}); err != nil {
		return err
	}
	var tree struct {
		FrameTree struct {
			Frame struct {
				ID string `json:"id"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	if err := b.cdp.CallInto(session, "Page.getFrameTree", nil, &tree); err != nil {
		return err
	}
	if tree.FrameTree.Frame.ID == "" {
		return nil
	}
	var world struct {
		ExecutionContextID int64 `json:"executionContextId"`
	}
	if err := b.cdp.CallInto(session, "Page.createIsolatedWorld", map[string]any{
		"frameId": tree.FrameTree.Frame.ID, "worldName": editableWorld,
	}, &world); err != nil {
		return err
	}
	if world.ExecutionContextID == 0 {
		return nil
	}
	if _, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression": observer, "contextId": world.ExecutionContextID,
	}); err != nil {
		return err
	}
	_, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression": selectObserver, "contextId": world.ExecutionContextID,
	})
	return err
}

func (b *Controller) onEditableTargetDetached(event cdp.Event) {
	var detached struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(event.Params, &detached) != nil || detached.SessionID == "" {
		return
	}
	b.mu.Lock()
	if tab := b.bySession[detached.SessionID]; tab != nil && tab.Session != detached.SessionID {
		delete(b.bySession, detached.SessionID)
	}
	b.mu.Unlock()
}

func (b *Controller) onEditableBinding(ev cdp.Event) {
	var binding struct {
		Name    string `json:"name"`
		Payload string `json:"payload"`
	}
	if json.Unmarshal(ev.Params, &binding) != nil || binding.Name != editableBinding || !b.isActiveSession(ev.SessionID) {
		return
	}
	var state struct {
		On   bool      `json:"on"`
		Show bool      `json:"show"`
		Kind string    `json:"kind"`
		Rect []float64 `json:"rect"`
	}
	if len(binding.Payload) > 4096 || json.Unmarshal([]byte(binding.Payload), &state) != nil {
		return
	}
	event := protocol.EditableEvent{Type: "editable", On: state.On}
	if state.On {
		event.ShowKeyboard = state.Show
		event.Kind = state.Kind
		if state.Show {
			log.Printf("input: trusted keyboard intent kind=%s", state.Kind)
		}
		if len(state.Rect) == 4 {
			event.Rect = state.Rect
		}
	}
	b.hub.BroadcastJSON(event)
}
