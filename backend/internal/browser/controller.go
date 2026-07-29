// Package browser implements Surf's Controller: ordered client commands and
// browser state. ScreencastSource, stream.VideoPipeline, ws.ClientTransport,
// and audio.AudioPipeline own their latency-sensitive domains independently.
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

	"surf-backend/internal/audio"
	"surf-backend/internal/cdp"
	"surf-backend/internal/config"
	"surf-backend/internal/proc"
	"surf-backend/internal/protocol"
	"surf-backend/internal/runenv"
	"surf-backend/internal/stream"
	"surf-backend/internal/telemetry"
	"surf-backend/internal/ws"
)

type videoResizeJob struct {
	w, h      int
	jpeg      []byte
	session   string
	bootstrap bool
}

type Tab struct {
	ID       int
	TargetID string
	Session  string
	Title    string
	URL      string

	loading bool

	Zoom     float64 // page zoom, 1..3; applied via device metrics
	IconKey  string  // favicon cache key (origin), "" when unknown
	Security string  // last Security.securityStateChanged state, "" unknown
}

type Controller struct {
	cfg      *config.Config
	cdp      *cdp.Client
	source   FrameSource
	cmd      *os.Process
	hub      *ws.Hub
	platform runenv.Handle
	commands chan controllerCommand
	events   chan cdp.Event

	mu        sync.Mutex
	tabs      map[int]*Tab
	byTarget  map[string]*Tab
	bySession map[string]*Tab
	seq       int
	activeID  int
	activeGen uint64
	viewW     int
	viewH     int
	// newTabNav is keyed by session. User-created tabs start on a settled
	// blank surface; their first captured frame consumes this URL and starts
	// navigation, avoiding captureScreenshot during renderer creation.
	newTabNav map[string]string

	store        *Store
	icons        map[string]*favicon // origin -> icon, guarded by b.mu
	iconFetching map[string]bool     // guarded by b.mu

	dlMu    sync.Mutex
	dlNames map[string]string // download guid -> final filename

	video     *stream.VideoPipeline
	audio     *audio.AudioPipeline
	mediaMu   sync.Mutex // guards client media subscriptions below
	videoSubs map[*ws.ClientTransport]*stream.Sub
	audioSubs map[*ws.ClientTransport]*audio.Sub

	videoResizeMu       sync.Mutex
	videoResizeWake     chan struct{}
	videoResizePending  videoResizeJob
	videoResizeSeq      uint64
	resizeMismatchOnce  sync.Once
	startedAt           time.Time
	adaptiveProfile     int
	adaptiveBaseQuality int
	adaptiveLastChange  time.Time
	adaptiveHealthy     int
	adaptiveUnhealthy   int
	// userAgent is populated from Browser.getVersion for full Chrome
	// headless-new. Chrome still exposes a HeadlessChrome token even though
	// this is the ordinary browser binary; normalize that single token while
	// preserving the real platform and version.
	userAgent         string
	captureCandidateW int
	captureCandidateH int
	captureCandidateN int

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
	videoSession       string
	sourceLatN         int
	sourceLatSumMS     float64
	sourceLatMaxMS     float64
	encodeLatN         int
	encodeLatSumMS     float64
	encodeLatMaxMS     float64

	// verbMu guards the small M2 state: pending JS dialogs and the pending
	// file-chooser interception (one at a time is plenty for one user).
	verbMu         sync.Mutex
	dialogSessions map[string]bool
	chooserSession string
	chooserNode    int64
	dlLastPush     map[string]time.Time // download guid -> last dlprogress
}

type controllerCommand struct {
	client     *ws.ClientTransport
	command    protocol.Command
	receivedNS uint64
}

func New(cfg *config.Config, hub *ws.Hub, platform runenv.Handle) *Controller {
	b := &Controller{
		cfg: cfg, hub: hub, platform: platform,
		adaptiveBaseQuality: cfg.SourceJPEGQuality,
		tabs:                map[int]*Tab{},
		byTarget:            map[string]*Tab{},
		bySession:           map[string]*Tab{},
		newTabNav:           map[string]string{},
		viewW:               cfg.ViewW, viewH: cfg.ViewH,
		store:           NewStore(cfg.Profile),
		icons:           map[string]*favicon{},
		iconFetching:    map[string]bool{},
		dlNames:         map[string]string{},
		dialogSessions:  map[string]bool{},
		dlLastPush:      map[string]time.Time{},
		video:           stream.New(streamConfig(cfg)),
		audio:           audio.New(audioConfig(cfg, platform)),
		videoSubs:       map[*ws.ClientTransport]*stream.Sub{},
		audioSubs:       map[*ws.ClientTransport]*audio.Sub{},
		perfCounts:      map[string]int{},
		videoResizeWake: make(chan struct{}, 1),
		commands:        make(chan controllerCommand, 256),
		events:          make(chan cdp.Event, 512),
		startedAt:       time.Now(),
	}
	go b.runVideoResize()
	go b.runController()
	return b
}

func (b *Controller) runController() {
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
		}
	}
}

// Start launches Chromium and wires target discovery; it returns once the
// browser is ready to serve clients.
func (b *Controller) Start() error {
	client, cmd, err := cdp.Launch(cdp.LaunchConfig{
		ChromePath:    b.cfg.ChromePath,
		Profile:       b.cfg.Profile,
		W:             b.cfg.ViewW,
		H:             b.cfg.ViewH,
		Env:           b.cfg.ChildEnv,
		NoSandbox:     b.cfg.ChromeNoSandbox,
		EnableGPU:     b.cfg.ChromeGPU,
		ExtensionPath: b.cfg.ContentBlockerPath,
	})
	if err != nil {
		return err
	}
	b.cdp = client
	if raw, versionErr := client.Call("", "Browser.getVersion", nil); versionErr == nil {
		var version struct {
			UserAgent string `json:"userAgent"`
		}
		if json.Unmarshal(raw, &version) == nil {
			b.userAgent = NormalizeHeadlessUserAgent(version.UserAgent)
		}
	}
	b.source = NewScreencastSource(client, b.onSourceFrame)
	b.cmd = cmd
	client.OnEvent(func(event cdp.Event) {
		if event.Method == "Page.screencastFrame" {
			// Media is not controller state. Route it straight to the
			// latest-frame pipeline so a blocking navigation/dialog/tab CDP
			// effect can never stall capture credits or video delivery.
			b.source.Handle(event)
			return
		}
		b.events <- event
	})
	// Full Chrome restores its previous pages and also creates the explicit
	// about:blank launch target. Remove only those redundant startup blanks
	// before discovery; otherwise an empty target can win the asynchronous
	// attach race and make a healthy screencast look frozen.
	b.normalizeStartupTargets()
	// targetCreated also fires for pre-existing targets on subscribe, so
	// startup and runtime tab discovery share one code path.
	_, err = client.Call("", "Target.setDiscoverTargets", map[string]any{"discover": true})
	if err != nil {
		return err
	}
	b.setupDownloads()
	log.Printf("browser ready, view %dx%d (headless-new, source=CDP screencast q%d, profile %s)",
		b.viewW, b.viewH, b.cfg.SourceJPEGQuality, b.cfg.Profile)
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
	if b.source != nil {
		b.source.Close()
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
		proc.Kill(b.cmd.Pid)
	}
}

// Died signals that the Chromium connection is gone (supervisor restarts us).
func (b *Controller) Died() <-chan struct{} { return b.cdp.Closed() }

// Stats is the /health?stats=1 runtime snapshot: enough to see
// what the server thinks is happening without ssh + log spelunking.
func (b *Controller) Stats() map[string]any {
	b.mu.Lock()
	tabs := len(b.tabs)
	var activeURL string
	if t := b.tabs[b.activeID]; t != nil {
		activeURL = t.URL
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
	windowSec := 0.0
	if !since.IsZero() {
		windowSec = time.Since(since).Seconds()
	}
	stats := map[string]any{
		"clients": b.hub.ClientCount(), "tabs": tabs, "activeURL": activeURL,
		"view": fmt.Sprintf("%dx%d", vw, vh), "casting": b.source != nil && b.source.Casting(),
		"videoSubs": vsubs, "audioSubs": asubs,
		"inputCounts": counts, "inputWindowSec": windowSec,
		"videoLatencyMeanMs": latMeanMS, "videoLatencyMaxMs": latMaxMS, "videoLatencyN": latN,
		"audioLatencyMeanMs": aLatMeanMS, "audioLatencyMaxMs": aLatMaxMS, "audioLatencyN": aLatN,
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
	case "Page.screencastFrame":
		if b.source != nil {
			b.source.Handle(ev)
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
	}
}

func (b *Controller) onSourceFrame(frame SourceFrame) {
	t, active := b.tabBySession(frame.Session)
	if t == nil || !active || b.hub.ClientCount() == 0 {
		return
	}
	b.noteSourceFrame()
	telemetry.Emit("source_frame", "capture", "cdp", nil)
	b.onSourceJPEG(t, frame)
	b.mu.Lock()
	navigate := b.newTabNav[frame.Session]
	delete(b.newTabNav, frame.Session)
	b.mu.Unlock()
	if navigate != "" {
		log.Printf("tab %d first frame ready, navigating to %s", t.ID, navigate)
		_ = b.cdp.Dispatch(frame.Session, "Page.navigate", map[string]any{"url": navigate})
	}
}

func (b *Controller) onSourceJPEG(t *Tab, frame SourceFrame) {
	buf, session := frame.JPEG, frame.Session
	if len(buf) == 0 || !b.isActiveSession(session) || b.hub.ClientCount() == 0 {
		return
	}
	b.perfMu.Lock()
	b.sourceSeq++
	sourceSeq, interactionID := b.sourceSeq, b.interactionID
	inputNS, cdpNS := b.interactionInputNS, b.interactionCDPNS
	b.perfMu.Unlock()
	b.pushVideoFrame(buf, session, sourceSeq, interactionID, stream.SourceMetadata{
		InputReceiveNS: inputNS, CDPAcceptedNS: cdpNS,
		ScrollX: frame.ScrollX, ScrollY: frame.ScrollY, PageScale: frame.PageScale,
		Profile: uint8(b.profileIndex()),
	})
}

// pushVideoFrame feeds a CDP JPEG to the H.264 transcoder while keeping its
// configured input size synchronized with the actual image. Mismatched
// transitional frames are withheld: feeding them to the old encoder can
// corrupt its pipeline, while restarting for every intermediate size causes
// a restart storm. Once one size persists, restart off the CDP dispatch
// goroutine and make that settled frame the new encoder's first input.
func (b *Controller) pushVideoFrame(buf []byte, session string, sourceSeq uint32, interactionID uint64, metadata stream.SourceMetadata) {
	w, h, ok := jpegSize(buf)
	if !ok {
		b.video.PushSourceMeta(buf, sourceSeq, interactionID, metadata)
		return
	}
	b.mu.Lock()
	wantW, wantH := b.viewW, b.viewH
	b.mu.Unlock()
	if w != wantW || h != wantH {
		b.resizeMismatchOnce.Do(func() {
			log.Printf("capture: source size differs got=%dx%d viewport=%dx%d", w, h, wantW, wantH)
		})
		// Chromium emits several old/intermediate compositor sizes around
		// viewport changes, but fullscreen content can also remain at a
		// different compositor size indefinitely. Debounce three identical
		// frames, then follow the actual source rather than black-holing every
		// frame while the client's frame-age counter climbs.
		if b.captureSizeSettled(w, h) {
			b.requestVideoResize(w, h, buf, session)
		}
		return
	}
	cur := b.video.Config()
	if cur.CaptureW == w && cur.CaptureH == h {
		b.resetCaptureCandidate()
		b.mu.Lock()
		sourceChanged := b.videoSession != "" && b.videoSession != session
		b.videoSession = session
		b.mu.Unlock()
		if sourceChanged {
			b.video.SwitchSourceMeta(buf, sourceSeq, interactionID, metadata)
		} else {
			b.video.PushSourceMeta(buf, sourceSeq, interactionID, metadata)
		}
		return
	}
	if b.captureSizeSettled(w, h) {
		b.requestVideoResize(w, h, buf, session)
	}
}

func (b *Controller) captureSizeSettled(w, h int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.captureCandidateW == w && b.captureCandidateH == h {
		b.captureCandidateN++
	} else {
		b.captureCandidateW, b.captureCandidateH, b.captureCandidateN = w, h, 1
	}
	return b.captureCandidateN >= 3
}

func (b *Controller) resetCaptureCandidate() {
	b.mu.Lock()
	b.captureCandidateW, b.captureCandidateH, b.captureCandidateN = 0, 0, 0
	b.mu.Unlock()
}

// requestVideoResize serializes every browser-originated SetSize call through
// one latest-wins worker. This prevents viewport changes, bootstrap captures,
// and JPEG drift corrections from racing and applying an obsolete size last.
func (b *Controller) requestVideoResize(w, h int, jpeg []byte, session string) {
	b.requestVideoResizeMode(w, h, jpeg, session, false)
}

func (b *Controller) requestVideoResizeMode(w, h int, jpeg []byte, session string, bootstrap bool) {
	job := videoResizeJob{w: w, h: h, jpeg: jpeg, session: session, bootstrap: bootstrap}
	b.videoResizeMu.Lock()
	b.videoResizePending = job
	b.videoResizeSeq++
	b.videoResizeMu.Unlock()
	select {
	case b.videoResizeWake <- struct{}{}:
	default:
	}
}

func (b *Controller) runVideoResize() {
	for range b.videoResizeWake {
		for {
			b.videoResizeMu.Lock()
			job, seq := b.videoResizePending, b.videoResizeSeq
			b.videoResizeMu.Unlock()
			if job.session != "" && !b.isActiveSession(job.session) {
				break
			}

			b.video.SetSize(job.w, job.h)
			if len(job.jpeg) > 0 && (job.session == "" || b.isActiveSession(job.session)) {
				if job.bootstrap {
					b.video.PushBootstrap(job.jpeg)
				} else {
					b.video.Push(job.jpeg)
				}
			}

			b.videoResizeMu.Lock()
			current := seq == b.videoResizeSeq
			b.videoResizeMu.Unlock()
			if current {
				break
			}
		}
	}
}
