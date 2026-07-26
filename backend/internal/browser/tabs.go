package browser

import (
	"encoding/json"
	"log"
	"sort"
	"strings"

	"surf-backend/internal/protocol"
)

type targetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

func (b *Browser) tabBySession(session string) (t *Tab, active bool) {
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
func (b *Browser) attachTarget(info targetInfo) {
	b.mu.Lock()
	if b.byTarget[info.TargetID] != nil {
		b.mu.Unlock()
		return
	}
	b.seq++
	id := b.seq
	t := &Tab{ID: id, TargetID: info.TargetID, Title: info.Title, URL: info.URL, Zoom: 1}
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
	t.Session = att.SessionID
	b.bySession[att.SessionID] = t
	b.mu.Unlock()

	s := att.SessionID
	log.Printf("tab %d attached: %.80s", id, info.URL)
	b.cdp.ForceFullscreen()
	if _, err := b.cdp.Call(s, "Page.enable", nil); err != nil {
		log.Printf("tab %d Page.enable: %v", id, err)
	}
	_, _ = b.cdp.Call(s, "Runtime.enable", nil)
	b.installCompatScripts(s)
	b.setupFeatures(t)

	if first && (info.URL == "" || info.URL == "about:blank" || info.URL == "chrome://newtab/") {
		_, _ = b.cdp.Call(s, "Page.navigate", map[string]any{"url": b.cfg.StartURL})
	}
	b.switchActive(id)
}

func (b *Browser) dropTarget(targetID string) {
	b.mu.Lock()
	t := b.byTarget[targetID]
	if t == nil {
		b.mu.Unlock()
		return
	}
	if t.settleTimer != nil {
		t.settleTimer.Stop()
	}
	delete(b.tabs, t.ID)
	delete(b.byTarget, targetID)
	if t.Session != "" {
		delete(b.bySession, t.Session)
	}
	wasActive := b.activeID == t.ID
	if wasActive {
		b.activeID = 0
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
			_, _ = b.cdp.Call("", "Target.createTarget", map[string]any{"url": b.cfg.StartURL})
		}
	}
	b.broadcastTabs()
}

func (b *Browser) targetInfoChanged(info targetInfo) {
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
	}
	active := t.ID == b.activeID
	url := t.URL
	b.mu.Unlock()

	if urlChanged && active {
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

func (b *Browser) tabNavigated(session, url string) {
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
	t.URL = url
	active := t.ID == b.activeID
	b.mu.Unlock()
	if active {
		b.hub.BroadcastJSON(b.urlMessage(url))
		b.pushNavState()
	}
	b.broadcastTabs()
	b.onURLChanged(t, url)
}

func (b *Browser) tabList() []protocol.TabInfo {
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

func (b *Browser) broadcastTabs() {
	b.hub.BroadcastJSON(map[string]any{"t": "tabs", "tabs": b.tabList()})
}

// urlMessage carries the bookmark state so the star button stays in sync,
// plus the active tab's TLS state for the omnibox padlock (additive; "" when
// unknown).
func (b *Browser) urlMessage(url string) map[string]any {
	b.mu.Lock()
	sec := ""
	if t := b.tabs[b.activeID]; t != nil {
		sec = t.Security
	}
	b.mu.Unlock()
	m := map[string]any{"t": "url", "url": url, "starred": b.store.IsBookmarked(url)}
	if sec != "" {
		m["security"] = sec
	}
	return m
}

// pushNavState tells clients whether back/forward are possible for the active
// tab, so the buttons can disable like a real browser's. Async: it needs a
// CDP round-trip and callers sit on hot paths.
func (b *Browser) pushNavState() {
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
		b.hub.BroadcastJSON(map[string]any{
			"t": "histstate", "back": h.CanGoBack(), "fwd": h.CanGoForward(),
		})
	}()
}

func (b *Browser) active() *Tab {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tabs[b.activeID]
}

func (b *Browser) isActiveSession(session string) bool {
	if session == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.tabs[b.activeID]
	return t != nil && t.Session == session
}

func (b *Browser) switchActive(id int) {
	b.mu.Lock()
	next := b.tabs[id]
	if next == nil {
		b.mu.Unlock()
		return
	}
	old := b.tabs[b.activeID]
	if old != nil && old != next && old.settleTimer != nil {
		old.settleTimer.Stop()
	}
	b.activeID = id
	url := next.URL
	b.mu.Unlock()

	if old != nil && old != next {
		b.stopCast(old)
	}
	_, _ = b.cdp.Call("", "Target.activateTarget", map[string]any{"targetId": next.TargetID})
	b.applyView(next)
	b.mu.Lock()
	next.motion = false
	b.mu.Unlock()
	b.ensureCast(next)
	go b.sendFreshFrame(nil, 0)
	b.hub.BroadcastJSON(b.urlMessage(url))
	b.pushNavState()
	b.broadcastTabs()
}

// applyView makes Chrome's viewport exactly the client's screen. Emulation is
// independent of the Xvfb window size, so any dimensions work. Zoom shrinks
// the CSS viewport and raises deviceScaleFactor by the same factor, so frames
// stay at the client's pixel size but everything renders larger and sharp.
func (b *Browser) applyView(t *Tab) {
	b.mu.Lock()
	z := t.Zoom
	if z < 1 {
		z = 1
	}
	w := int(float64(b.viewW)/z + 0.5)
	h := int(float64(b.viewH)/z + 0.5)
	s := t.Session
	b.mu.Unlock()
	if err := b.cdp.SetDeviceMetrics(s, w, h, z); err != nil {
		log.Printf("setDeviceMetricsOverride tab %d (%dx%d z%.2f): %v", t.ID, w, h, z, err)
	}
}
