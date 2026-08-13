package browser

import (
	"encoding/json"

	"surf-backend/internal/cdp"
	"surf-backend/internal/protocol"
)

const fullscreenBinding = "__surfFullscreenChanged"

// fullscreenObserver is installed before navigation and in the current
// document. The binding crosses the isolated CDP transport without polling,
// so a site's own player controls can immediately hide or restore native
// chrome while leaving the authenticated Surf socket untouched.
const fullscreenObserver = `(function(){
  if(window!==window.top||window.__surfFullscreenObserver)return;
  window.__surfFullscreenObserver=true;
  function active(){return !!(document.fullscreenElement||document.webkitFullscreenElement||document.webkitCurrentFullScreenElement);}
  function report(){try{window.__surfFullscreenChanged(active()?'1':'0');}catch(_){}}
  document.addEventListener('fullscreenchange',report,false);
  document.addEventListener('webkitfullscreenchange',report,false);
  report();
})()`

func (b *Controller) setupPageFullscreen(session string) {
	_ = b.cdp.Dispatch(session, "Runtime.addBinding", map[string]any{"name": fullscreenBinding})
	_ = b.cdp.Dispatch(session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": fullscreenObserver})
	_ = b.cdp.Dispatch(session, "Runtime.evaluate", map[string]any{"expression": fullscreenObserver})
}

func (b *Controller) onFullscreenBinding(ev cdp.Event) {
	var payload struct {
		Name    string `json:"name"`
		Payload string `json:"payload"`
	}
	if json.Unmarshal(ev.Params, &payload) != nil || payload.Name != fullscreenBinding {
		return
	}
	on := payload.Payload == "1"
	b.mu.Lock()
	tab := b.bySession[ev.SessionID]
	if tab == nil {
		b.mu.Unlock()
		return
	}
	changed := tab.Fullscreen != on
	tab.Fullscreen = on
	active := tab.ID == b.activeID
	b.mu.Unlock()
	if changed && active {
		b.broadcast(protocol.BoolEvent{Type: "fullscreen", On: on})
	}
}

func (b *Controller) setPageFullscreen(on bool) {
	tab := b.active()
	if tab == nil {
		return
	}
	b.mu.Lock()
	session := tab.Session
	b.mu.Unlock()
	expression := `(function(){
  var exit=document.exitFullscreen||document.webkitExitFullscreen||document.webkitCancelFullScreen;
  if(exit){var p=exit.call(document);if(p&&p.catch)p.catch(function(){});}
})()`
	if on {
		expression = `(function(){
  if(document.fullscreenElement||document.webkitFullscreenElement||document.webkitCurrentFullScreenElement)return;
  var video=document.querySelector('video');
  var player=video&&video.closest?video.closest('#movie_player,.html5-video-player,[data-fullscreen-container]'):null;
  var target=player||video||document.documentElement;
  var request=target&&(target.requestFullscreen||target.webkitRequestFullscreen||target.webkitRequestFullScreen);
  if(request){var p=request.call(target);if(p&&p.catch)p.catch(function(){});}
})()`
	}
	_ = b.cdp.Dispatch(session, "Runtime.evaluate", map[string]any{
		"expression":  expression,
		"userGesture": true,
	})
}
