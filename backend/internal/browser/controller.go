// Package browser implements Surf's Controller: ordered client commands and
// browser state. media pipelines and client transports own their
// latency-sensitive domains independently.
//
// Locking rule: b.mu guards all tab/view state and is NEVER held across a
// cdp.Call — copy what you need under the lock, then talk to Chromium.
package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/config"
	"surf-backend/internal/media"
	"surf-backend/internal/process"
	"surf-backend/internal/protocol"
	"surf-backend/internal/transport"
)

type Tab struct {
	ID       int
	TargetID string
	Session  string
	Title    string
	URL      string

	loading           bool
	awaitingPageFrame bool

	Zoom       float64 // page zoom, 1..3; applied via device metrics
	IconKey    string  // favicon cache key (origin), "" when unknown
	Security   string  // last Security.securityStateChanged state, "" unknown
	Fullscreen bool    // page Fullscreen API state
}

type Controller struct {
	cfg      *config.Config
	cdp      *cdp.Client
	cmd      *os.Process
	hub      *transport.Hub
	commands chan controllerCommand
	events   chan cdp.Event

	mu           sync.Mutex
	tabs         map[int]*Tab
	byTarget     map[string]*Tab
	bySession    map[string]*Tab
	seq          int
	activeID     int
	activeGen    uint64
	viewW        int
	viewH        int
	mobile       bool
	resizeMu     sync.Mutex
	resizeTimer  *time.Timer
	resizeGen    uint64
	resizeClosed bool

	store        *Store
	icons        map[string]*favicon // origin -> icon, guarded by b.mu
	iconFetching map[string]bool     // guarded by b.mu

	dlMu    sync.Mutex
	dlNames map[string]string // download guid -> final filename

	video     *media.VideoPipeline
	audio     *media.AudioPipeline
	capture   *media.Capture
	mediaMu   sync.Mutex // guards client media subscriptions below
	shutdown  sync.Once
	videoSubs map[*transport.Client]*media.VideoSubscription
	audioSubs map[*transport.Client]*media.AudioSubscription

	startedAt          time.Time
	adaptiveProfile    int
	adaptiveLastChange time.Time
	adaptiveHealthy    int
	adaptiveUnhealthy  int
	// userAgent is populated from Browser.getVersion for full Chrome
	// headless-new. Chrome still exposes a HeadlessChrome token even though
	// this is the ordinary browser binary; normalize that single token while
	// preserving the real platform and version.
	userAgent string
	// widevineState is probed through EME on the first trustworthy page,
	// making the diagnostic independent of host OS and browser brand.
	widevineState   string
	widevineDetail  string
	widevineProbing bool

	perfMu             sync.Mutex
	perfSince          time.Time
	perfCounts         map[string]int
	perfLatN           int     // video AUs measured in the current window
	perfLatSumMS       float64 // sum of per-subscriber queue-dwell time, for the mean
	perfLatMaxMS       float64
	perfALatN          int // audio chunks measured in the current window
	perfALatSumMS      float64
	perfALatMaxMS      float64
	interactionInputNS uint64
	interactionCDPNS   uint64
	lastRenderInputAt  time.Time
	interactionID      uint64
	sourceSeq          uint32
	sourceInteraction  uint64
	sourceInputNS      uint64
	sourceCDPNS        uint64
	auLatN             int
	auLatSumMS         float64
	auLatMaxMS         float64
	motionActive       bool
	motionLastAUAt     time.Time
	motionLastSourceAt time.Time
	motionStallLogged  bool
	motionStalls       uint64
	motionGapN         int
	motionGapSumMS     float64
	motionGapMaxMS     float64
	sourceGapN         int
	sourceGapSumMS     float64
	sourceGapMaxMS     float64
	duplicateAUs       int

	// verbMu guards the small M2 state: pending JS dialogs and the pending
	// file-chooser interception (one at a time is plenty for one user).
	verbMu         sync.Mutex
	dialogSessions map[string]bool
	chooserSession string
	chooserNode    int64
	dlLastPush     map[string]time.Time // download guid -> last dlprogress
}

type controllerCommand struct {
	client     *transport.Client
	command    protocol.Command
	receivedNS uint64
}

func New(cfg *config.Config, hub *transport.Hub) (*Controller, error) {
	capture, err := media.NewCapture(cfg.SurfHome)
	if err != nil {
		return nil, fmt.Errorf("initialize tab capture: %w", err)
	}
	audioCfg := media.AudioConfig{Capture: capture.OpenAudio}
	videoCfg := videoPipelineConfig(cfg)
	var b *Controller
	videoCfg.Start = func(width, height, bitrateK int) error {
		return capture.StartVideo(media.EncoderConfig{
			Codec: encoderCodec(width, height),
			Width: width, Height: height, BitrateK: bitrateK,
			Quantizer: cfg.StreamQuantizer,
		}, b.onVideoFrame)
	}
	videoCfg.Stop = capture.StopVideo
	videoCfg.Keyframe = capture.RequestVideoKeyframe
	b = &Controller{
		cfg: cfg, hub: hub,
		tabs:           map[int]*Tab{},
		byTarget:       map[string]*Tab{},
		bySession:      map[string]*Tab{},
		viewW:          cfg.ViewW,
		viewH:          cfg.ViewH,
		store:          NewStore(cfg.Profile),
		icons:          map[string]*favicon{},
		iconFetching:   map[string]bool{},
		dlNames:        map[string]string{},
		dialogSessions: map[string]bool{},
		dlLastPush:     map[string]time.Time{},
		video:          media.NewVideoPipeline(videoCfg),
		audio:          media.NewAudioPipeline(audioCfg),
		capture:        capture,
		videoSubs:      map[*transport.Client]*media.VideoSubscription{},
		audioSubs:      map[*transport.Client]*media.AudioSubscription{},
		perfCounts:     map[string]int{},
		commands:       make(chan controllerCommand, 256),
		events:         make(chan cdp.Event, 512),
		startedAt:      time.Now(),
		widevineState:  "unknown",
	}
	go b.runController()
	return b, nil
}

func (b *Controller) runController() {
	motionTicker := time.NewTicker(25 * time.Millisecond)
	defer motionTicker.Stop()
	for {
		select {
		case item := <-b.commands:
			select {
			case <-item.client.Closed():
				continue
			default:
				b.handleCommand(item.client, item.command, item.receivedNS)
			}
		case event := <-b.events:
			b.onEvent(event)
		case now := <-motionTicker.C:
			b.checkMotionStall(now)
		}
	}
}

// Start launches Chromium and wires target discovery; it returns once the
// browser is ready to serve clients.
func (b *Controller) Start() (err error) {
	started := false
	defer func() {
		if !started {
			b.Shutdown()
		}
	}()
	extensionPaths := []string{}
	if b.cfg.ContentBlockerPath != "" {
		extensionPaths = append(extensionPaths, b.cfg.ContentBlockerPath)
	}
	if b.capture != nil {
		extensionPaths = append(extensionPaths, b.capture.ExtensionPath())
	}
	client, cmd, err := cdp.Launch(cdp.LaunchConfig{
		ChromePath:     b.cfg.ChromePath,
		Profile:        b.cfg.Profile,
		W:              b.cfg.ViewW,
		H:              b.cfg.ViewH,
		NoSandbox:      b.cfg.ChromeNoSandbox,
		EnableGPU:      b.cfg.ChromeGPU,
		ExtensionPaths: extensionPaths,
	})
	if err != nil {
		return err
	}
	b.cdp = client
	b.cmd = cmd
	if b.capture != nil {
		if err := b.capture.Attach(client); err != nil {
			return err
		}
		if err := b.capture.Start(); err != nil {
			return fmt.Errorf("start host-isolating tab capture: %w", err)
		}
	}
	if raw, versionErr := client.Call("", "Browser.getVersion", nil); versionErr == nil {
		var version struct {
			UserAgent string `json:"userAgent"`
		}
		if json.Unmarshal(raw, &version) == nil {
			b.userAgent = NormalizeHeadlessUserAgent(version.UserAgent)
		}
	}
	client.OnEvent(func(event cdp.Event) { b.events <- event })
	// Full Chrome restores its previous pages and also creates the explicit
	// about:blank launch target. Remove only those redundant startup blanks
	// before discovery; otherwise an empty target can win the asynchronous
	// attach race and make a healthy capture look frozen.
	b.normalizeStartupTargets()
	// targetCreated also fires for pre-existing targets on subscribe, so
	// startup and runtime tab discovery share one code path.
	_, err = client.Call("", "Target.setDiscoverTargets", map[string]any{"discover": true})
	if err != nil {
		return err
	}
	b.setupDownloads()
	log.Printf("browser ready, view %dx%d (headless-new, source=tabCapture/WebCodecs, profile %s)",
		b.viewW, b.viewH, b.cfg.Profile)
	started = true
	return nil
}

func (b *Controller) normalizeStartupTargets() {
	raw, err := b.cdp.Call("", "Target.getTargets", nil)
	if err != nil {
		return
	}
	var result struct {
		Targets []targetInfo `json:"targetInfos"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return
	}
	var pages []targetInfo
	hasRealPage := false
	for _, target := range result.Targets {
		if target.Type != "page" {
			continue
		}
		pages = append(pages, target)
		if target.URL != "" && target.URL != "about:blank" && target.URL != "chrome://newtab/" {
			hasRealPage = true
		}
	}
	keptBlank := false
	for _, target := range pages {
		blank := target.URL == "" || target.URL == "about:blank" || target.URL == "chrome://newtab/"
		if !blank {
			continue
		}
		if !hasRealPage && !keptBlank {
			keptBlank = true
			continue
		}
		_, _ = b.cdp.Call("", "Target.closeTarget", map[string]any{"targetId": target.TargetID})
	}
}

func (b *Controller) newTargetParams(rawURL string) map[string]any {
	return map[string]any{"url": rawURL}
}

func (b *Controller) Shutdown() {
	b.shutdown.Do(func() {
		b.resizeMu.Lock()
		b.resizeClosed = true
		b.resizeGen++
		if b.resizeTimer != nil {
			b.resizeTimer.Stop()
			b.resizeTimer = nil
		}
		b.resizeMu.Unlock()
		// Close the source first: encoder/audio startup may be waiting for an
		// extension response while holding its pipeline lock. Closing capture
		// wakes those waits so shutdown cannot inherit their 12-second timeout.
		if b.capture != nil {
			_ = b.capture.Close()
		}
		if b.video != nil {
			b.video.Shutdown()
		}
		if b.audio != nil {
			b.audio.Shutdown()
		}
		if b.cdp != nil {
			b.cdp.Close()
		}
		if b.cmd != nil {
			process.Kill(b.cmd.Pid)
		}
	})
}

// Died signals that the Chromium connection is gone (supervisor restarts us).
func (b *Controller) Died() <-chan struct{} { return b.cdp.Closed() }

// Stats is the /api/v1/health?stats=1 runtime snapshot: enough to see
// what the server thinks is happening without ssh + log spelunking.
func (b *Controller) Stats() map[string]any {
	b.mu.Lock()
	tabs := len(b.tabs)
	var activeURL string
	if t := b.tabs[b.activeID]; t != nil {
		activeURL = t.URL
	}
	vw, vh := b.viewW, b.viewH
	widevine, widevineDetail := b.widevineState, b.widevineDetail
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
	motionStalls := b.motionStalls
	b.perfMu.Unlock()
	latMeanMS := 0.0
	if latN > 0 {
		latMeanMS = latSumMS / float64(latN)
	}
	aLatMeanMS := 0.0
	if aLatN > 0 {
		aLatMeanMS = aLatSumMS / float64(aLatN)
	}
	windowSec := 0.0
	if !since.IsZero() {
		windowSec = time.Since(since).Seconds()
	}
	stats := map[string]any{
		"clients": b.hub.ClientCount(), "tabs": tabs, "activeURL": activeURL,
		"view": fmt.Sprintf("%dx%d", vw, vh), "casting": b.video.Running(),
		"widevine": widevine, "widevineDetail": widevineDetail,
		"videoSubs": vsubs, "audioSubs": asubs,
		"inputCounts": counts, "inputWindowSec": windowSec,
		"videoLatencyMeanMs": latMeanMS, "videoLatencyMaxMs": latMaxMS, "videoLatencyN": latN,
		"audioLatencyMeanMs": aLatMeanMS, "audioLatencyMaxMs": aLatMaxMS, "audioLatencyN": aLatN,
		"motionSourceStalls": motionStalls,
	}
	for key, value := range b.hub.Stats() {
		stats[key] = value
	}
	for key, value := range b.audio.Stats() {
		stats[key] = value
	}
	for key, value := range b.video.Stats() {
		stats[key] = value
	}
	return stats
}

func (b *Controller) Health() error {
	if b.cdp == nil {
		return io.ErrClosedPipe
	}
	// Browser.getVersion is a browser-target command and is cheap enough for
	// the desktop supervisor's two-second liveness probe. "Controller" is not
	// a CDP domain; using it made every healthy backend report HTTP 503.
	_, err := b.cdp.Call("", "Browser.getVersion", nil)
	return err
}

func (b *Controller) onEvent(ev cdp.Event) {
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
			b.mu.Lock()
			t.loading = true
			b.mu.Unlock()
			b.hub.BroadcastJSON(protocol.BoolEvent{Type: "loading", On: true})
		}
	case "Page.frameStoppedLoading":
		if t, active := b.tabBySession(ev.SessionID); t != nil {
			b.mu.Lock()
			t.loading = false
			b.mu.Unlock()
			if active {
				b.hub.BroadcastJSON(protocol.BoolEvent{Type: "loading"})
				b.pushNavState()
			}
			go b.refreshFavicon(t) // Runtime.evaluate inside; must not block dispatch
		}
	case "Page.loadEventFired":
		// Some modern pages keep subresources active indefinitely and never
		// produce frameStoppedLoading.
		if t, active := b.tabBySession(ev.SessionID); t != nil && active {
			b.mu.Lock()
			t.loading = false
			b.mu.Unlock()
		}
		if t, _ := b.tabBySession(ev.SessionID); t != nil {
			go b.refreshFavicon(t)
		}
		go b.probeWidevine(ev.SessionID)
	case "Controller.downloadWillBegin":
		b.onDownloadBegin(ev)
	case "Controller.downloadProgress":
		b.onDownloadProgress(ev)
	case "Page.javascriptDialogOpening":
		b.onJavascriptDialog(ev)
	case "Page.javascriptDialogClosed":
		b.onJavascriptDialogClosed(ev)
	case "Page.fileChooserOpened":
		b.onFileChooserOpened(ev)
	case "Security.securityStateChanged":
		b.onSecurityStateChanged(ev)
	case "Runtime.bindingCalled":
		b.onFullscreenBinding(ev)
	}
}
