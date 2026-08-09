package browser

import (
	"encoding/json"
	"log"
	"sort"
	"strings"
	"time"

	"surf-backend/internal/protocol"
)

type targetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

func (b *Controller) tabBySession(session string) (t *Tab, active bool) {
	if session == "" {
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	t = b.bySession[session]
	return t, t != nil && t.ID == b.activeID
}

// attachTarget sets up a newly discovered page target and focuses it
// (new tabs and OAuth popups should grab the screen, like server.js did).
func (b *Controller) attachTarget(info targetInfo) {
	b.mu.Lock()
	if b.byTarget[info.TargetID] != nil {
		b.mu.Unlock()
		return
	}
	// Full Chrome may publish its implicit launch about:blank just after the
	// initial getTargets cleanup. It can win the concurrent attach race, hang
	// activation, and leave the video source pointed at no usable page. Only
	// suppress blanks during the startup settling window and only when a real
	// page is already known; later window.open/about:blank tabs are legitimate.
	if time.Since(b.startedAt) < 5*time.Second &&
		(info.URL == "" || info.URL == "about:blank" || info.URL == "chrome://newtab/") {
		hasRealPage := false
		for _, existing := range b.tabs {
			if existing.URL != "" && existing.URL != "about:blank" && existing.URL != "chrome://newtab/" {
				hasRealPage = true
				break
			}
		}
		if hasRealPage {
			b.mu.Unlock()
			_ = b.cdp.Dispatch("", "Target.closeTarget", map[string]any{"targetId": info.TargetID})
			log.Printf("startup: discarded redundant blank target")
			return
		}
	}
	b.seq++
	id := b.seq
	t := &Tab{ID: id, TargetID: info.TargetID, Title: info.Title, URL: info.URL}
	b.tabs[id] = t
	b.byTarget[info.TargetID] = t
	first := id == 1
	b.mu.Unlock()

	res, err := b.cdp.Call("", "Target.attachToTarget", map[string]any{"targetId": info.TargetID, "flatten": true})
	if err != nil {
		b.mu.Lock()
		delete(b.tabs, id)
		delete(b.byTarget, info.TargetID)
		b.mu.Unlock()
		return
	}
	var att struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(res, &att) != nil || att.SessionID == "" {
		return
	}
	b.mu.Lock()
	if b.byTarget[info.TargetID] != t {
		b.mu.Unlock()
		// targetDestroyed won while attachToTarget was in flight. Do not let
		// the stale completion repopulate bySession or become active.
		_, _ = b.cdp.Call("", "Target.detachFromTarget", map[string]any{"sessionId": att.SessionID})
		return
	}
	t.Session = att.SessionID
	b.bySession[att.SessionID] = t
	b.mu.Unlock()

	s := att.SessionID
	log.Printf("tab %d attached: %.80s", id, info.URL)
	// Finish one-time session setup before capture starts. The previous tab
	// remains live during this short setup, then switchActive can produce the
	// new tab continuously from frame one.
	_ = b.cdp.Dispatch(s, "Page.enable", nil)
	_ = b.cdp.Dispatch(s, "Runtime.enable", nil)
	b.mu.Lock()
	userAgentParams := userAgentOverrideParams(b.userAgent, b.userAgentMetadata, b.mobile)
	b.mu.Unlock()
	if userAgentParams != nil {
		_ = b.cdp.Dispatch(s, "Network.enable", nil)
		_ = b.cdp.Dispatch(s, "Network.setUserAgentOverride", userAgentParams)
	}
	b.installCompatScripts(s)
	b.setupEditableAutoAttach(s)
	b.setupFeatures(t)
	// The compatibility override must be queued before the first real
	// navigation; otherwise the initial request and document expose different
	// user agents.
	if first && (info.URL == "" || info.URL == "about:blank" || info.URL == "chrome://newtab/") {
		_ = b.cdp.Dispatch(s, "Page.navigate", map[string]any{"url": b.cfg.StartURL})
	}
	b.switchActive(id)
}

func (b *Controller) dropTarget(targetID string) {
	b.mu.Lock()
	t := b.byTarget[targetID]
	if t == nil {
		b.mu.Unlock()
		return
	}
	delete(b.tabs, t.ID)
	delete(b.byTarget, targetID)
	if t.Session != "" {
		for session, attachedTab := range b.bySession {
			if attachedTab == t {
				delete(b.bySession, session)
			}
		}
	}
	wasActive := b.activeID == t.ID
	if wasActive {
		b.activeID = 0
		b.activeGen++
	}
	// Most recently created tab wins, like the Map-insertion-order pick before.
	nextID := 0
	for id := range b.tabs {
		if id > nextID {
			nextID = id
		}
	}
	b.mu.Unlock()

	if wasActive {
		if nextID != 0 {
			b.switchActive(nextID)
		} else {
			// Never leave zero tabs; targetCreated re-activates.
			_, _ = b.cdp.Call("", "Target.createTarget", b.newTargetParams("about:blank#surf-new"))
		}
	}
	b.broadcastTabs()
}

func (b *Controller) targetInfoChanged(info targetInfo) {
	b.mu.Lock()
	t := b.byTarget[info.TargetID]
	if t == nil {
		b.mu.Unlock()
		return
	}
	titleChanged := info.Title != "" && info.Title != t.Title
	urlChanged := info.URL != "" && info.URL != t.URL
	if titleChanged {
		t.Title = info.Title
	}
	if urlChanged {
		t.URL = info.URL
		t.awaitingPageFrame = true
	}
	active := t.ID == b.activeID
	url := t.URL
	b.mu.Unlock()

	if urlChanged && active {
		b.setTouchMode(t.Session)
		b.hub.BroadcastJSON(b.urlMessage(url))
		b.pushNavState()
	}
	if titleChanged || urlChanged {
		b.broadcastTabs()
	}
	if urlChanged {
		b.onURLChanged(t, url)
		if titleChanged {
			b.store.SetTitle(url, info.Title)
		}
	} else if titleChanged {
		b.store.SetTitle(url, info.Title)
	}
}

func (b *Controller) tabNavigated(session, url string) {
	b.mu.Lock()
	t := b.bySession[session]
	if t == nil || url == "" || t.URL == url {
		b.mu.Unlock()
		return
	}
	// A failed navigation lands on chrome-error://. Keep the last real URL in
	// the omnibox and raise the native error card instead (M2.5).
	if strings.HasPrefix(url, "chrome-error://") {
		b.mu.Unlock()
		b.noteNavigationError(t)
		return
	}
	if pageOrigin(t.URL) != pageOrigin(url) {
		t.IconKey = ""
	}
	t.URL = url
	t.awaitingPageFrame = true
	active := t.ID == b.activeID
	b.mu.Unlock()
	if active {
		b.touch.cancel(false)
		b.setTouchMode(session)
		b.hub.BroadcastJSON(b.urlMessage(url))
		b.pushNavState()
	}
	b.broadcastTabs()
	b.onURLChanged(t, url)
}

func (b *Controller) tabList() []protocol.TabInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]int, 0, len(b.tabs))
	for id := range b.tabs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	list := make([]protocol.TabInfo, 0, len(ids))
	for _, id := range ids {
		t := b.tabs[id]
		title := t.Title
		if title == "" {
			title = t.URL
		}
		if title == "" {
			title = "new tab"
		}
		list = append(list, protocol.TabInfo{
			ID: id, Title: title, URL: t.URL, Active: id == b.activeID, Icon: b.iconURLLocked(t),
		})
	}
	return list
}

func (b *Controller) broadcastTabs() {
	b.hub.BroadcastJSON(protocol.TabsEvent{Type: "tabs", Tabs: b.tabList()})
}

// urlMessage carries the bookmark state so the star button stays in sync,
// plus the active tab's TLS state for the omnibox padlock (additive; "" when
// unknown).
func (b *Controller) urlMessage(url string) protocol.URLStateEvent {
	b.mu.Lock()
	sec := ""
	if t := b.tabs[b.activeID]; t != nil {
		sec = t.Security
	}
	b.mu.Unlock()
	return protocol.URLStateEvent{
		Type: "url", URL: url, Starred: b.store.IsBookmarked(url), Security: sec,
	}
}

// pushNavState tells clients whether back/forward are possible for the active
// tab, so the buttons can disable like a real browser's. Async: it needs a
// CDP round-trip and callers sit on hot paths.
func (b *Controller) pushNavState() {
	t := b.active()
	if t == nil {
		return
	}
	b.mu.Lock()
	s := t.Session
	b.mu.Unlock()
	go func() {
		h, err := b.cdp.NavigationHistory(s)
		if err != nil {
			return
		}
		if !b.isActiveSession(s) {
			return
		}
		b.hub.BroadcastJSON(protocol.HistoryStateEvent{
			Type: "histstate", Back: h.CanGoBack(), Fwd: h.CanGoForward(),
		})
	}()
}

func (b *Controller) active() *Tab {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tabs[b.activeID]
}

func (b *Controller) isActiveSession(session string) bool {
	if session == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.bySession[session]
	return t != nil && t.ID == b.activeID
}

func (b *Controller) switchActive(id int) {
	b.touch.cancel(true)
	b.mu.Lock()
	next := b.tabs[id]
	if next == nil {
		b.mu.Unlock()
		return
	}
	b.activeID = id
	b.activeGen++
	generation := b.activeGen
	url := next.URL
	b.mu.Unlock()

	// Confirm activation before switching capture; fire-and-forget can briefly
	// deliver the previous tab's surface.
	started := time.Now()
	if _, err := b.cdp.Call("", "Target.activateTarget", map[string]any{"targetId": next.TargetID}); err != nil {
		log.Printf("tab %d activation failed after %s: %v", id, time.Since(started).Round(time.Millisecond), err)
		return
	}
	log.Printf("tab %d activated in %s", id, time.Since(started).Round(time.Millisecond))
	if !b.isActiveGeneration(id, generation) {
		return
	}
	if b.capture != nil {
		b.capture.SwitchActive()
	}
	b.applyView(next)
	if !b.isActiveGeneration(id, generation) {
		return
	}
	b.hub.BroadcastJSON(b.urlMessage(url))
	b.mu.Lock()
	fullscreen := next.Fullscreen
	b.mu.Unlock()
	b.hub.BroadcastJSON(protocol.BoolEvent{Type: "fullscreen", On: fullscreen})
	b.pushNavState()
	b.broadcastTabs()
}

func (b *Controller) isActiveGeneration(id int, generation uint64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.activeID == id && b.activeGen == generation && b.tabs[id] != nil
}

// applyView makes Chrome's viewport exactly the client's screen. Emulation is
// independent of Chrome's hidden platform window, so any dimensions work.
func (b *Controller) applyView(t *Tab) {
	b.mu.Lock()
	w := b.viewW
	h := b.viewH
	pixelW, pixelH := b.viewW, b.viewH
	mobile := b.mobile
	s := t.Session
	b.mu.Unlock()
	orientation, angle := "portraitPrimary", 0
	if w > h {
		orientation, angle = "landscapePrimary", 90
	}
	// tabCapture reads Chromium's compositor surface, so its real headless
	// window must follow the client too; otherwise Chromium scales a stale
	// surface into the requested encoder dimensions and text becomes soft.
	b.resizeCaptureSurface(t.TargetID, pixelW, pixelH)
	metrics := map[string]any{
		"width": w, "height": h, "deviceScaleFactor": 1, "mobile": mobile,
		"screenWidth": pixelW, "screenHeight": pixelH,
		"screenOrientation": map[string]any{
			"type": orientation, "angle": angle,
		},
	}
	_, _ = b.cdp.Call(s, "Emulation.setDeviceMetricsOverride", metrics)
	// Device metrics can replace Chromium's emulation configuration when the
	// mobile bit changes. Touch must be the final override; otherwise the page
	// still renders as mobile but Input.dispatchTouchEvent produces no DOM
	// pointer/touch events until a later navigation happens to restore it.
	b.setTouchMode(s)
}
