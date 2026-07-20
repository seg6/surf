// Package browser drives headful Chromium over CDP: tab lifecycle with
// popup auto-focus, the screencast pipeline, and translation of client
// control messages into CDP input/navigation calls.
//
// Locking rule: b.mu guards all tab/view state and is NEVER held across a
// cdp.Call — copy what you need under the lock, then talk to Chromium.
package browser

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"os/exec"
	"sort"
	"sync"
	"time"

	"rbrowser/internal/cdp"
	"rbrowser/internal/config"
	"rbrowser/internal/protocol"
	"rbrowser/internal/stream"
	"rbrowser/internal/ws"
)

type Tab struct {
	ID       int
	TargetID string
	Session  string
	Title    string
	URL      string

	casting     bool
	castQuality int
	castMaxW    int
	castMaxH    int
	motion      bool
	recasting   bool
	settleTimer *time.Timer

	Zoom    float64 // page zoom, 1..3; applied via device metrics
	IconKey string  // favicon cache key (origin), "" when unknown
}

type Browser struct {
	cfg *config.Config
	cdp *cdp.Client
	cmd *exec.Cmd
	hub *ws.Hub

	mu        sync.Mutex
	tabs      map[int]*Tab
	byTarget  map[string]*Tab
	bySession map[string]*Tab
	seq       int
	activeID  int
	viewW     int
	viewH     int

	store        *Store
	icons        map[string]*favicon // origin -> icon, guarded by b.mu
	iconFetching map[string]bool     // guarded by b.mu

	dlMu    sync.Mutex
	dlNames map[string]string // download guid -> final filename

	streamer  *stream.Streamer
	screenMu  sync.Mutex
	videoMu   sync.Mutex
	videoSubs map[*ws.Client]*stream.Sub
}

func New(cfg *config.Config, hub *ws.Hub) *Browser {
	return &Browser{
		cfg: cfg, hub: hub,
		tabs:      map[int]*Tab{},
		byTarget:  map[string]*Tab{},
		bySession: map[string]*Tab{},
		viewW:     cfg.ViewW, viewH: cfg.ViewH,
		store:        NewStore(cfg.Profile),
		icons:        map[string]*favicon{},
		iconFetching: map[string]bool{},
		dlNames:      map[string]string{},
		streamer:     stream.New(streamConfig(cfg)),
		videoSubs:    map[*ws.Client]*stream.Sub{},
	}
}

// Start launches Chromium and wires target discovery; it returns once the
// browser is ready to serve clients.
func (b *Browser) Start() error {
	client, cmd, err := cdp.Launch(b.cfg.ChromePath, b.cfg.Profile, b.cfg.DisplayW, b.cfg.DisplayH)
	if err != nil {
		return err
	}
	b.cdp = client
	b.cmd = cmd
	client.OnEvent(b.onEvent)
	// targetCreated also fires for pre-existing targets on subscribe, so
	// startup and runtime tab discovery share one code path.
	_, err = client.Call("", "Target.setDiscoverTargets", map[string]any{"discover": true})
	if err != nil {
		return err
	}
	b.setupDownloads()
	log.Printf("browser ready, view %dx%d display %dx%d q%d (headful, profile %s)", b.viewW, b.viewH, b.cfg.DisplayW, b.cfg.DisplayH, b.cfg.Quality, b.cfg.Profile)
	return nil
}

// Died signals that the Chromium connection is gone (supervisor restarts us).
func (b *Browser) Died() <-chan struct{} { return b.cdp.Closed() }

func (b *Browser) onEvent(ev cdp.Event) {
	// Handlers below that issue blocking CDP calls (attach/drop) MUST run off
	// this goroutine: the dispatch loop also delivers Fetch.requestPaused, and
	// a page whose requests aren't being continued can't answer CDP calls —
	// blocking here deadlocks the whole browser.
	switch ev.Method {
	case "Target.targetCreated":
		var p struct {
			TargetInfo targetInfo `json:"targetInfo"`
		}
		if json.Unmarshal(ev.Params, &p) == nil && p.TargetInfo.Type == "page" {
			go b.attachTarget(p.TargetInfo)
		}
	case "Target.targetDestroyed":
		var p struct {
			TargetID string `json:"targetId"`
		}
		if json.Unmarshal(ev.Params, &p) == nil {
			go b.dropTarget(p.TargetID)
		}
	case "Target.targetInfoChanged":
		var p struct {
			TargetInfo targetInfo `json:"targetInfo"`
		}
		if json.Unmarshal(ev.Params, &p) == nil {
			b.targetInfoChanged(p.TargetInfo)
		}
	case "Page.screencastFrame":
		b.onScreencastFrame(ev)
	case "Page.frameNavigated":
		var p struct {
			Frame struct {
				ParentID string `json:"parentId"`
				URL      string `json:"url"`
			} `json:"frame"`
		}
		if json.Unmarshal(ev.Params, &p) == nil && p.Frame.ParentID == "" {
			b.tabNavigated(ev.SessionID, p.Frame.URL)
		}
	case "Page.navigatedWithinDocument":
		var p struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(ev.Params, &p) == nil {
			b.tabNavigated(ev.SessionID, p.URL)
		}
	case "Page.frameStartedLoading":
		if t, active := b.tabBySession(ev.SessionID); t != nil && active {
			b.hub.BroadcastJSON(map[string]any{"t": "loading", "on": true})
		}
	case "Page.frameStoppedLoading":
		if t, active := b.tabBySession(ev.SessionID); t != nil {
			if active {
				b.hub.BroadcastJSON(map[string]any{"t": "loading", "on": false})
				go b.sendFreshFrame(nil, b.cfg.SharpQuality)
				b.pushNavState()
			}
			go b.refreshFavicon(t) // Runtime.evaluate inside; must not block dispatch
		}
	case "Fetch.requestPaused":
		b.onRequestPaused(ev)
	case "Browser.downloadWillBegin":
		b.onDownloadBegin(ev)
	case "Browser.downloadProgress":
		b.onDownloadProgress(ev)
	}
}

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
	if _, err := b.cdp.Call(s, "Page.enable", nil); err != nil {
		log.Printf("tab %d Page.enable: %v", id, err)
	}
	_, _ = b.cdp.Call(s, "Runtime.enable", nil)
	b.installCompatScripts(s)
	b.setupFeatures(t)

	if first && (info.URL == "" || info.URL == "about:blank") {
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
		go b.onURLChanged(t, url)
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
	t.URL = url
	active := t.ID == b.activeID
	b.mu.Unlock()
	if active {
		b.hub.BroadcastJSON(b.urlMessage(url))
		b.pushNavState()
	}
	b.broadcastTabs()
	go b.onURLChanged(t, url)
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

// urlMessage carries the bookmark state so the star button stays in sync.
func (b *Browser) urlMessage(url string) map[string]any {
	return map[string]any{"t": "url", "url": url, "starred": b.store.IsBookmarked(url)}
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
		res, err := b.cdp.Call(s, "Page.getNavigationHistory", nil)
		if err != nil {
			return
		}
		var h struct {
			CurrentIndex int `json:"currentIndex"`
			Entries      []struct {
				ID int `json:"id"`
			} `json:"entries"`
		}
		if json.Unmarshal(res, &h) != nil {
			return
		}
		b.hub.BroadcastJSON(map[string]any{
			"t": "histstate", "back": h.CurrentIndex > 0, "fwd": h.CurrentIndex < len(h.Entries)-1,
		})
	}()
}

func (b *Browser) active() *Tab {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tabs[b.activeID]
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
	if _, err := b.cdp.Call(s, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width": w, "height": h, "deviceScaleFactor": z, "mobile": false,
	}); err != nil {
		log.Printf("setDeviceMetricsOverride tab %d (%dx%d z%.2f): %v", t.ID, w, h, z, err)
	}
}

func (b *Browser) onScreencastFrame(ev cdp.Event) {
	var p struct {
		Data      string `json:"data"`
		SessionID int    `json:"sessionId"`
		Metadata  struct {
			DeviceWidth   float64 `json:"deviceWidth"`
			DeviceHeight  float64 `json:"deviceHeight"`
			ScrollOffsetX float64 `json:"scrollOffsetX"`
			ScrollOffsetY float64 `json:"scrollOffsetY"`
		} `json:"metadata"`
	}
	if json.Unmarshal(ev.Params, &p) != nil {
		return
	}
	b.cdp.Send(ev.SessionID, "Page.screencastFrameAck", map[string]any{"sessionId": p.SessionID})
	t, active := b.tabBySession(ev.SessionID)
	if t == nil || !active || b.hub.ClientCount() == 0 {
		return
	}
	buf, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return
	}
	// Header dims must be actual frame pixels, not CSS: the screencast
	// downscales to the cast's max size (metadata only reports CSS).
	b.mu.Lock()
	w, h := t.castMaxW, t.castMaxH
	if w == 0 || h == 0 {
		w, h = b.viewW, b.viewH
	}
	b.mu.Unlock()
	// Scroll offsets travel in remote CSS px; the client reconciles its
	// locally-panned view against them.
	b.hub.QueueFrame(&protocol.Frame{
		W: w, H: h,
		ScrollX: clampScroll(p.Metadata.ScrollOffsetX),
		ScrollY: clampScroll(p.Metadata.ScrollOffsetY),
		Data:    buf,
	}, nil)
}
