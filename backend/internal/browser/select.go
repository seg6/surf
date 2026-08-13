package browser

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"surf-backend/internal/cdp"
	"surf-backend/internal/protocol"
)

const selectBinding = "__surfSelectOpened"
const selectIntentPlaceholder = "__SURF_SELECT_INTENT_EVENT__"

// Chromium renders a select popup in browser UI, outside the captured tab.
// Intercept only a completed physical tap, suppress that invisible popup, and
// hand the exact select element to the isolated-world observer below. Pointer
// cancellation still leaves ordinary page scrolling alone.
const selectIntentSensor = `(function(){
  if(window.__surfSelectIntentSensor)return;
  window.__surfSelectIntentSensor=true;
  var armed=null,lastElement=null,lastOpened=-1;
  var dispatch=EventTarget.prototype.dispatchEvent,IntentEvent=CustomEvent;
  function selectFor(element){
    if(!element)return null;
    var tag=(element.tagName||'').toUpperCase();
    if(tag==='LABEL'&&element.control)element=element.control;
    return (element.tagName||'').toUpperCase()==='SELECT'&&!element.disabled?element:null;
  }
  function eventSelect(event){
    var path=event.composedPath?event.composedPath():[event.target];
    for(var i=0;i<path.length;i++){var found=selectFor(path[i]);if(found)return found;}
    return null;
  }
  function primary(event){
    return event.isTrusted&&event.isPrimary!==false&&
      !(event.touches&&event.touches.length>1)&&!(event.changedTouches&&event.changedTouches.length>1);
  }
  function point(event){return (event.changedTouches&&event.changedTouches[0])||event;}
  function suppress(event){if(event.cancelable)event.preventDefault();}
  function open(element){
    var now=performance.now();
    if(element===lastElement&&now-lastOpened<500)return;
    lastElement=element;lastOpened=now;
    dispatch.call(element,new IntentEvent('` + selectIntentPlaceholder + `',{bubbles:true,composed:true}));
  }
  function begin(event){
    if(!primary(event))return;
    var element=eventSelect(event);if(!element)return;
    var p=point(event);
    armed={element:element,id:event.pointerId,x:p.clientX,y:p.clientY};
    // Prevent Chromium's separate popup while preserving pointer delivery and
    // direct-manipulation scrolling governed by touch-action.
    suppress(event);
  }
  function move(event){
    if(!armed||!primary(event)||(event.pointerId!==undefined&&event.pointerId!==armed.id))return;
    var p=point(event),dx=p.clientX-armed.x,dy=p.clientY-armed.y;
    if(dx*dx+dy*dy>144)armed=null;
  }
  function end(event){
    if(!armed||!primary(event)||(event.pointerId!==undefined&&event.pointerId!==armed.id))return;
    var element=armed.element;armed=null;suppress(event);
    if(element&&element.isConnected)open(element);
  }
  function cancel(){armed=null;}
  function click(event){
    if(!primary(event))return;
    var element=eventSelect(event);if(!element)return;
    suppress(event);open(element);
  }
  if(window.PointerEvent){
    window.addEventListener('pointerdown',begin,{capture:true,passive:false});
    window.addEventListener('pointermove',move,{capture:true,passive:false});
    window.addEventListener('pointerup',end,{capture:true,passive:false});
    window.addEventListener('pointercancel',cancel,{capture:true,passive:false});
  }else{
    window.addEventListener('touchstart',begin,{capture:true,passive:false});
    window.addEventListener('touchmove',move,{capture:true,passive:false});
    window.addEventListener('touchend',end,{capture:true,passive:false});
    window.addEventListener('touchcancel',cancel,{capture:true,passive:false});
    window.addEventListener('mousedown',begin,true);
    window.addEventListener('mousemove',move,true);
    window.addEventListener('mouseup',end,true);
  }
  window.addEventListener('click',click,{capture:true,passive:false});
})()`

// The page cannot access this isolated-world binding or its element map. A
// reply is scoped to the exact execution context and one-shot local token that
// produced the option list, so navigation or replacement of the control turns
// a late reply into a harmless no-op.
const selectObserver = `(function(){
  if(window.__surfSelectObserver)return;
  window.__surfSelectObserver=true;
  var controls=new Map(),next=0;
  function labelFor(select){
    var label=select.getAttribute('aria-label')||select.getAttribute('title')||'';
    if(!label&&select.labels&&select.labels.length)label=select.labels[0].textContent||'';
    return String(label).replace(/\s+/g,' ').trim().slice(0,256);
  }
  function open(event){
    var select=event.target;
    if(!select||(select.tagName||'').toUpperCase()!=='SELECT'||select.disabled)return;
    var id=String(++next),options=[],all=select.options;
    controls.clear();controls.set(id,select);
    for(var i=0;i<all.length&&i<512;i++){
      var option=all[i],parent=option.parentElement;
      options.push({label:String(option.label||option.text||'').slice(0,1024),
        disabled:!!(option.disabled||(parent&&parent.tagName==='OPTGROUP'&&parent.disabled)),selected:!!option.selected});
    }
    var r=select.getBoundingClientRect(),v=window.visualViewport;
    var w=(v&&v.width)||window.innerWidth||1,h=(v&&v.height)||window.innerHeight||1;
    try{window.__surfSelectOpened(JSON.stringify({id:id,title:labelFor(select),multiple:!!select.multiple,
      options:options,rect:[r.left/w,r.top/h,r.width/w,r.height/h]}));}catch(_){}
  }
  Object.defineProperty(window,'__surfApplySelect',{configurable:false,writable:false,value:function(id,indices){
    var select=controls.get(String(id));controls.delete(String(id));
    if(!select||!select.isConnected||!Array.isArray(indices))return false;
    var wanted=new Set(),all=select.options;
    for(var i=0;i<indices.length;i++){
      var index=indices[i];
      if(Number.isInteger(index)&&index>=0&&index<all.length&&!all[index].disabled)wanted.add(index);
      if(!select.multiple&&wanted.size)break;
    }
    var changed=false;
    for(var j=0;j<all.length;j++){
      var selected=wanted.has(j);
      if(all[j].selected!==selected){all[j].selected=selected;changed=true;}
    }
    if(changed){
      select.dispatchEvent(new Event('input',{bubbles:true,composed:true}));
      select.dispatchEvent(new Event('change',{bubbles:true,composed:true}));
    }
    return changed;
  }});
  document.addEventListener('` + selectIntentPlaceholder + `',open,true);
  window.addEventListener('pagehide',function(){controls.clear();},false);
})()`

func newSelectScripts() (string, string, string) {
	event := "__surf_select_intent_" + rand.Text()
	return event, strings.ReplaceAll(selectIntentSensor, selectIntentPlaceholder, event),
		strings.ReplaceAll(selectObserver, selectIntentPlaceholder, event)
}

func selectBindingParams() map[string]any {
	return map[string]any{"name": selectBinding, "executionContextName": editableWorld}
}

func selectSensorParams(source string, runImmediately bool) map[string]any {
	params := map[string]any{"source": source}
	if runImmediately {
		params["runImmediately"] = true
	}
	return params
}

func selectObserverParams(source string, runImmediately bool) map[string]any {
	params := map[string]any{"source": source, "worldName": editableWorld}
	if runImmediately {
		params["runImmediately"] = true
	}
	return params
}

type selectRequest struct {
	requestID   string
	localID     string
	session     string
	contextID   int64
	optionCount int
}

func (b *Controller) onSelectBinding(ev cdp.Event) {
	var binding struct {
		Name               string `json:"name"`
		Payload            string `json:"payload"`
		ExecutionContextID int64  `json:"executionContextId"`
	}
	if json.Unmarshal(ev.Params, &binding) != nil || binding.Name != selectBinding ||
		binding.ExecutionContextID == 0 || !b.isActiveSession(ev.SessionID) || len(binding.Payload) > 256<<10 {
		return
	}
	var state struct {
		ID       string                  `json:"id"`
		Title    string                  `json:"title"`
		Multiple bool                    `json:"multiple"`
		Options  []protocol.SelectOption `json:"options"`
		Rect     []float64               `json:"rect"`
	}
	if json.Unmarshal([]byte(binding.Payload), &state) != nil || state.ID == "" || len(state.ID) > 64 ||
		len(state.Title) > 256 || len(state.Options) == 0 || len(state.Options) > 512 {
		return
	}
	for _, option := range state.Options {
		if len(option.Label) > 4096 {
			return
		}
	}
	requestID := rand.Text()
	b.verbMu.Lock()
	b.selectRequest = selectRequest{
		requestID: requestID, localID: state.ID, session: ev.SessionID,
		contextID: binding.ExecutionContextID, optionCount: len(state.Options),
	}
	b.verbMu.Unlock()
	event := protocol.SelectEvent{
		Type: "select", RequestID: requestID, Title: state.Title,
		Multiple: state.Multiple, Options: state.Options,
	}
	b.mu.Lock()
	tab := b.bySession[ev.SessionID]
	topLevel := tab != nil && tab.Session == ev.SessionID
	b.mu.Unlock()
	if topLevel && len(state.Rect) == 4 {
		valid := true
		for _, value := range state.Rect {
			valid = valid && finiteUnit(value)
		}
		if valid && state.Rect[2] > 0 && state.Rect[3] > 0 {
			event.Rect = state.Rect
		}
	}
	log.Printf("input: native select opened options=%d multiple=%t", len(state.Options), state.Multiple)
	b.broadcast(event)
}

func (b *Controller) handleSelectReply(reply *protocol.SelectReplyCommand) {
	b.verbMu.Lock()
	request := b.selectRequest
	if request.requestID == reply.RequestID {
		b.selectRequest = selectRequest{}
	}
	b.verbMu.Unlock()
	if request.requestID == "" || request.requestID != reply.RequestID || reply.Cancel {
		return
	}
	seen := make(map[int]bool, len(reply.Indices))
	for _, index := range reply.Indices {
		if index < 0 || index >= request.optionCount || seen[index] {
			return
		}
		seen[index] = true
	}
	localID, _ := json.Marshal(request.localID)
	indices, _ := json.Marshal(reply.Indices)
	expression := fmt.Sprintf("globalThis.__surfApplySelect&&globalThis.__surfApplySelect(%s,%s)", localID, indices)
	if err := b.cdp.Dispatch(request.session, "Runtime.evaluate", map[string]any{
		"expression": expression, "contextId": request.contextID,
	}); err != nil {
		log.Printf("select: apply reply: %v", err)
	}
}
