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
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"surf-backend/internal/audio"
	"surf-backend/internal/cdp"
	"surf-backend/internal/config"
	"surf-backend/internal/proc"
	"surf-backend/internal/protocol"
	"surf-backend/internal/runenv"
	"surf-backend/internal/stream"
	"surf-backend/internal/ws"
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
	settleSeq   uint64
	lastSharpAt time.Time

	Zoom     float64 // page zoom, 1..3; applied via device metrics
	IconKey  string  // favicon cache key (origin), "" when unknown
	Security string  // last Security.securityStateChanged state, "" unknown
}

type Browser struct {
	cfg      *config.Config
	cdp      *cdp.Client
	cmd      *exec.Cmd
	hub      *ws.Hub
	platform runenv.Handle

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
	audio     *audio.Streamer
	screenMu  sync.Mutex
	mediaMu   sync.Mutex // guards client media subscriptions below
	videoSubs map[*ws.Client]*stream.Sub
	audioSubs map[*ws.Client]*audio.Sub

	perfMu        sync.Mutex
	perfSince     time.Time
	perfCounts    map[string]int
	perfLatN      int     // video AUs measured in the current window
	perfLatSumMS  float64 // sum of per-subscriber queue-dwell time, for the mean
	perfLatMaxMS  float64
	perfALatN     int // audio chunks measured in the current window
	perfALatSumMS float64
	perfALatMaxMS float64

	// verbMu guards the small M2 state: pending JS dialogs and the pending
	// file-chooser interception (one at a time is plenty for one user).
	verbMu         sync.Mutex
	dialogSessions map[string]bool
	chooserSession string
	chooserNode    int64
	dlLastPush     map[string]time.Time // download guid -> last dlprogress
}

func New(cfg *config.Config, hub *ws.Hub, platform runenv.Handle) *Browser {
	return &Browser{
		cfg: cfg, hub: hub, platform: platform,
		tabs:      map[int]*Tab{},
		byTarget:  map[string]*Tab{},
		bySession: map[string]*Tab{},
		viewW:     cfg.ViewW, viewH: cfg.ViewH,
		store:          NewStore(cfg.Profile),
		icons:          map[string]*favicon{},
		iconFetching:   map[string]bool{},
		dlNames:        map[string]string{},
		dialogSessions: map[string]bool{},
		dlLastPush:     map[string]time.Time{},
		streamer:       stream.New(streamConfig(cfg, platform)),
		audio:          audio.New(audioConfig(cfg, platform)),
		videoSubs:      map[*ws.Client]*stream.Sub{},
		audioSubs:      map[*ws.Client]*audio.Sub{},
		perfCounts:     map[string]int{},
	}
}

// Start launches Chromium and wires target discovery; it returns once the
// browser is ready to serve clients.
func (b *Browser) Start() error {
	client, cmd, err := cdp.Launch(cdp.LaunchConfig{
		ChromePath:   b.cfg.ChromePath,
		Profile:      b.cfg.Profile,
		W:            b.cfg.DisplayW,
		H:            b.cfg.DisplayH,
		Env:          b.cfg.ChildEnv,
		NoSandbox:    b.cfg.ChromeNoSandbox,
		PlatformArgs: b.platform.ChromeArgs(),
	})
	if err != nil {
		return err
	}
	b.cdp = client
	b.cmd = cmd
	client.ForceFullscreen()
	client.OnEvent(b.onEvent)
	// targetCreated also fires for pre-existing targets on subscribe, so
	// startup and runtime tab discovery share one code path.
	_, err = client.Call("", "Target.setDiscoverTargets", map[string]any{"discover": true})
	if err != nil {
		return err
	}
	b.setupDownloads()
	log.Printf("browser ready, view %dx%d display %s %dx%d q%d (headful, profile %s)", b.viewW, b.viewH, b.cfg.Display, b.cfg.DisplayW, b.cfg.DisplayH, b.cfg.Quality, b.cfg.Profile)
	return nil
}

func (b *Browser) Shutdown() {
	if b.streamer != nil {
		b.streamer.Shutdown()
	}
	if b.audio != nil {
		b.audio.Shutdown()
	}
	if b.cdp != nil {
		b.cdp.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		proc.Kill(b.cmd)
	}
}

// Died signals that the Chromium connection is gone (supervisor restarts us).
func (b *Browser) Died() <-chan struct{} { return b.cdp.Closed() }

// Stats is the /health?stats=1 diagnostics snapshot (M1.1): enough to see
// what the server thinks is happening without ssh + log spelunking.
func (b *Browser) Stats() map[string]any {
	b.mu.Lock()
	tabs := len(b.tabs)
	var activeURL string
	var casting, motion bool
	if t := b.tabs[b.activeID]; t != nil {
		activeURL = t.URL
		casting = t.casting
		motion = t.motion
	}
	vw, vh := b.viewW, b.viewH
	b.mu.Unlock()
	b.mediaMu.Lock()
	vsubs, asubs := len(b.videoSubs), len(b.audioSubs)
	b.mediaMu.Unlock()
	b.perfMu.Lock()
	counts := map[string]int{}
	for k, v := range b.perfCounts {
		counts[k] = v
	}
	since := b.perfSince
	latN, latSumMS, latMaxMS := b.perfLatN, b.perfLatSumMS, b.perfLatMaxMS
	aLatN, aLatSumMS, aLatMaxMS := b.perfALatN, b.perfALatSumMS, b.perfALatMaxMS
	b.perfMu.Unlock()
	latMeanMS := 0.0
	if latN > 0 {
		latMeanMS = latSumMS / float64(latN)
	}
	aLatMeanMS := 0.0
	if aLatN > 0 {
		aLatMeanMS = aLatSumMS / float64(aLatN)
	}
	return map[string]any{
		"clients": b.hub.ClientCount(), "tabs": tabs, "activeURL": activeURL,
		"view": fmt.Sprintf("%dx%d", vw, vh), "casting": casting, "motion": motion,
		"videoSubs": vsubs, "audioSubs": asubs,
		"inputCounts": counts, "inputWindowSec": time.Since(since).Seconds(),
		"videoLatencyMeanMs": latMeanMS, "videoLatencyMaxMs": latMaxMS, "videoLatencyN": latN,
		"audioLatencyMeanMs": aLatMeanMS, "audioLatencyMaxMs": aLatMaxMS, "audioLatencyN": aLatN,
	}
}

func (b *Browser) Health() error {
	if b.cdp == nil {
		return io.ErrClosedPipe
	}
	_, err := b.cdp.Call("", "Browser.getVersion", nil)
	return err
}

func (b *Browser) onEvent(ev cdp.Event) {
	// Handlers below that issue blocking CDP calls (attach/drop) MUST run off
	// this goroutine: blocking the dispatch loop stalls every other event,
	// deadlocking the whole browser.
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
				go b.sendSharpFrame(nil, t)
				b.pushNavState()
			}
			go b.refreshFavicon(t) // Runtime.evaluate inside; must not block dispatch
		}
	case "Browser.downloadWillBegin":
		b.onDownloadBegin(ev)
	case "Browser.downloadProgress":
		b.onDownloadProgress(ev)
	case "Page.javascriptDialogOpening":
		b.onJavascriptDialog(ev)
	case "Page.javascriptDialogClosed":
		b.onJavascriptDialogClosed(ev)
	case "Page.fileChooserOpened":
		b.onFileChooserOpened(ev)
	case "Security.securityStateChanged":
		b.onSecurityStateChanged(ev)
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
