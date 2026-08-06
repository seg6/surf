package browser

import (
	"encoding/json"
	"log"

	"surf-backend/internal/cdp"
	"surf-backend/internal/protocol"
)

const editableBinding = "__surfEditableChanged"

// editableObserver follows actual focus instead of guessing after a touch.
// It walks open shadow roots and is installed in every same-target frame.
const editableObserver = `(function(){
  if(window.__surfEditableObserver)return;
  window.__surfEditableObserver=true;
  function focused(){
    var e=document.activeElement;
    while(e&&e.shadowRoot&&e.shadowRoot.activeElement)e=e.shadowRoot.activeElement;
    return e;
  }
  function state(){
    var e=focused(),on=false,kind='text';
    if(e&&!e.disabled&&!e.readOnly){
      var t=(e.tagName||'').toUpperCase();
      if(t==='TEXTAREA'){on=true;kind='textarea';}
      else if(e.isContentEditable){on=true;kind='text';}
      else if(t==='INPUT'){
        var ty=(e.type||'text').toLowerCase();
        var skip={button:1,checkbox:1,radio:1,submit:1,reset:1,file:1,image:1,range:1,color:1,hidden:1,date:1,time:1};
        if(!skip[ty]){
          on=true;
          kind=({password:'password',email:'email',number:'number',tel:'tel',url:'url',search:'search'})[ty]||'text';
        }
      }
    }
    if(!on)return {on:false};
    var r=e.getBoundingClientRect(),v=window.visualViewport;
    var w=(v&&v.width)||window.innerWidth||1,h=(v&&v.height)||window.innerHeight||1;
    return {on:true,kind:kind,rect:[r.left/w,r.top/h,r.width/w,r.height/h]};
  }
  function report(){try{window.__surfEditableChanged(JSON.stringify(state()));}catch(_){}}
  function soon(){setTimeout(report,0);}
  document.addEventListener('focusin',soon,true);
  document.addEventListener('focusout',soon,true);
  window.addEventListener('pagehide',function(){try{window.__surfEditableChanged('{"on":false}');}catch(_){}},false);
  if(window.visualViewport){window.visualViewport.addEventListener('resize',soon,false);window.visualViewport.addEventListener('scroll',soon,false);}
  report();
})()`

func (b *Controller) setupEditableObserver(session string) {
	_ = b.cdp.Dispatch(session, "Runtime.addBinding", map[string]any{"name": editableBinding})
	_ = b.cdp.Dispatch(session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": editableObserver})
	_ = b.cdp.Dispatch(session, "Runtime.evaluate", map[string]any{"expression": editableObserver})
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
	if waitingForDebugger {
		defer func() {
			_ = b.cdp.Dispatch(session, "Runtime.runIfWaitingForDebugger", nil)
		}()
	}
	commands := []struct {
		method string
		params any
	}{
		{"Runtime.enable", nil},
		{"Runtime.addBinding", map[string]any{"name": editableBinding}},
		{"Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": editableObserver}},
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
	// Newly created OOPIF targets have no default execution context until they
	// resume; their new-document script is already installed above. Existing
	// targets do have a live document and need the observer evaluated now.
	if !waitingForDebugger {
		if _, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{"expression": editableObserver}); err != nil {
			log.Printf("editable: initialize iframe Runtime.evaluate: %v", err)
		}
	}
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

func (b *Controller) onEditableExecutionContext(event cdp.Event) {
	var created struct {
		Context struct {
			AuxData struct {
				IsDefault bool `json:"isDefault"`
			} `json:"auxData"`
		} `json:"context"`
	}
	if json.Unmarshal(event.Params, &created) != nil || !created.Context.AuxData.IsDefault {
		return
	}
	b.mu.Lock()
	tab := b.bySession[event.SessionID]
	isChild := tab != nil && tab.Session != event.SessionID
	b.mu.Unlock()
	if isChild {
		_ = b.cdp.Dispatch(event.SessionID, "Runtime.evaluate", map[string]any{"expression": editableObserver})
	}
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
		Kind string    `json:"kind"`
		Rect []float64 `json:"rect"`
	}
	if len(binding.Payload) > 4096 || json.Unmarshal([]byte(binding.Payload), &state) != nil {
		return
	}
	event := protocol.EditableEvent{Type: "editable", On: state.On}
	if state.On {
		event.Kind = state.Kind
		if len(state.Rect) == 4 {
			event.Rect = state.Rect
		}
	}
	b.hub.BroadcastJSON(event)
}
