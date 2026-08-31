package browser

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"surf-backend/internal/logs"
	"surf-backend/internal/media"
	"surf-backend/internal/protocol"
	"surf-backend/internal/telemetry"
	"surf-backend/internal/transport"
)

var schemeRe = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*:`)

const (
	viewportResizeSettle = 120 * time.Millisecond
	minViewportDimension = 64
	maxViewportDimension = 1600
)

// NormalizeNavURL turns address-bar input into a URL: scheme kept as-is,
// bare hosts get https://, everything else becomes a search.
func NormalizeNavURL(input string) string {
	u := strings.TrimSpace(input)
	if u == "" {
		return ""
	}
	if schemeRe.MatchString(u) {
		return u
	}
	if strings.Index(u, ".") > 0 && !strings.Contains(u, " ") {
		return "https://" + u
	}
	return "https://www.google.com/search?q=" + url.QueryEscape(u)
}

// ClientConnected implements transport.Handler: greet, sync state, and automatically
// subscribe the client to the only visual lane (H.264).
func (b *Controller) ClientConnected(c *transport.Client) {
	b.mu.Lock()
	w, h := b.viewW, b.viewH
	b.mu.Unlock()
	b.send(c, protocol.HelloEvent{Type: "hello", W: w, H: h})
	b.send(c, protocol.TabsEvent{Type: "tabs", Tabs: b.tabList()})
	b.send(c, protocol.BoolEvent{Type: "loading", On: b.activeTabLoading()})
	// The native controller sends its laid-out viewport immediately after the
	// socket opens. Let that ordered message settle before starting capture so
	// startup does not encode at the config default and then restart twice.
	time.AfterFunc(150*time.Millisecond, func() {
		select {
		case <-c.Closed():
			return
		case <-b.stop:
			return
		default:
			b.subscribeVideo(c)
		}
	})
	if t := b.active(); t != nil {
		b.mu.Lock()
		u := t.URL
		fullscreen := t.Fullscreen
		b.mu.Unlock()
		b.send(c, b.urlMessage(u))
		b.send(c, protocol.BoolEvent{Type: "fullscreen", On: fullscreen})
		b.pushNavState()
	}
}

// HandleMessage implements transport.Handler. It runs on the client's read goroutine,
// and only enqueues immutable typed commands. The controller goroutine is the
// sole ordered executor (press before release, key order, tab commands).
func (b *Controller) HandleMessage(c *transport.Client, command protocol.Command) {
	if touch, ok := command.(*protocol.TouchCommand); ok {
		b.touch.enqueue(c, touch, telemetry.MonoNS())
		return
	}
	select {
	case b.commands <- controllerCommand{client: c, command: command, receivedNS: telemetry.MonoNS()}:
	case <-c.Closed():
	case <-b.stop:
	default:
		c.Close()
	}
}

func (b *Controller) handleCommand(c *transport.Client, command protocol.Command, receivedNS uint64) {
	kind := command.Kind()
	telemetry.Emit("client_command", "input", "controller", map[string]any{"type": kind})
	iid, _ := command.Causal()
	b.noteClientMessage(kind)
	if iid != 0 && renderCommand(kind) {
		b.noteRenderInput()
		defer func() {
			b.perfMu.Lock()
			b.interactionID = iid
			b.interactionInputNS = receivedNS
			b.interactionCDPNS = telemetry.MonoNS()
			b.perfMu.Unlock()
		}()
	}
	switch m := command.(type) {
	case *protocol.SizeCommand:
		b.handleSize(m)
		return
	case *protocol.TabCommand:
		b.handleTab(m)
		return
	case *protocol.ToggleCommand:
		if kind == "audio" {
			b.handleAudio(c, m.On)
		} else if kind == "mobile" {
			b.handleMobileLayout(m.On)
		} else if kind == "dark" {
			b.handleDarkMode(m.On)
		} else if kind == "fullscreen" {
			b.setPageFullscreen(m.On)
		}
		return
	case *protocol.DialogReplyCommand:
		b.handleDialogReply(m)
		return
	case *protocol.SelectReplyCommand:
		b.handleSelectReply(m)
		return
	case *protocol.MediaStatsCommand:
		b.handleMediaStats(c, m)
		return
	case *protocol.URLCommand:
		if kind == "opennew" {
			b.openInNewTab(m.URL)
			return
		}
	}
	switch kind {
	case "video-retry":
		b.mediaMu.Lock()
		_, subscribed := b.videoSubs[c]
		b.mediaMu.Unlock()
		if subscribed {
			b.send(c, protocol.VideoConfigEvent{Type: "video-config", State: "starting", Generation: b.video.Generation(), Profile: b.profileName()})
			b.video.Restart()
		} else {
			// A failed first-AU wait removes this client's subscription while
			// the five-second idle timer is still running. Stop that stale
			// generation before subscribing again.
			b.video.Restart()
			b.subscribeVideo(c)
		}
		return
	case "reqkeyframe":
		// Client lost decode sync (renderer/VT session error or bad SPS/PPS) and
		// needs a recovery IDR; healthy streams have no periodic keyframe cadence.
		b.video.RequestKeyframe()
		return
	}
	t := b.active()
	if t == nil {
		return
	}
	b.mu.Lock()
	s := t.Session
	b.mu.Unlock()
	switch m := command.(type) {
	case *protocol.URLCommand:
		if kind != "nav" {
			b.handleFeatureMessage(c, t, s, command)
			return
		}
		// Fire-and-forget: Page.navigate doesn't answer until the navigation
		// commits, and the WS reader must stay responsive meanwhile.
		if u := NormalizeNavURL(m.URL); u != "" {
			log.Printf("nav -> %s", u)
			b.setTabLoading(t, true)
			if err := b.cdp.Dispatch(s, "Page.navigate", map[string]any{"url": u}); err != nil {
				b.setTabLoading(t, false)
			}
		}
	case *protocol.EmptyCommand:
		switch kind {
		case "reload":
			b.setTabLoading(t, true)
			if err := b.cdp.Dispatch(s, "Page.reload", map[string]any{}); err != nil {
				b.setTabLoading(t, false)
			}
		case "stop":
			_, _ = b.cdp.Call(s, "Page.stopLoading", nil)
			b.setTabLoading(t, false)
		case "back", "fwd":
			b.setTabLoading(t, true)
			if !b.navigateHistory(s, kind == "back") {
				b.setTabLoading(t, false)
			}
		case "media-playpause":
			b.controlPageMedia(c, s, "playpause", 0)
		case "media-mute":
			b.controlPageMedia(c, s, "mute", 0)
		case "media-query":
			go b.queryPageMedia(c, s)
		default:
			b.handleFeatureMessage(c, t, s, command)
		}
	case *protocol.VolumeCommand:
		b.controlPageMedia(c, s, "volume", m.Value)
	case *protocol.KeyCommand:
		if m.Text != "" {
			// Key messages must stay ordered. cdp.Send launches a goroutine per
			// command, which can race rapid typing before Chromium sees it.
			_ = b.cdp.Dispatch(s, "Input.insertText", map[string]any{"text": m.Text})
		} else {
			typ := "keyUp"
			if m.Down {
				typ = "rawKeyDown"
			}
			params := map[string]any{
				"type": typ, "key": m.Key, "code": m.Code,
				"windowsVirtualKeyCode": m.KeyCode, "nativeVirtualKeyCode": m.KeyCode,
			}
			// Enter must be a full keyDown carrying \r, or Chromium never
			// submits forms / activates default buttons (rawKeyDown skips
			// text processing entirely).
			if m.Down && m.KeyCode == 13 {
				params["type"] = "keyDown"
				params["text"] = "\r"
				params["unmodifiedText"] = "\r"
			}
			_ = b.cdp.Dispatch(s, "Input.dispatchKeyEvent", params)
		}
	case *protocol.CompositionCommand:
		switch m.Phase {
		case "update":
			_ = b.cdp.Dispatch(s, "Input.imeSetComposition", map[string]any{
				"text": m.Text, "selectionStart": m.SelectionStart, "selectionEnd": m.SelectionEnd,
			})
		case "commit":
			_ = b.cdp.Dispatch(s, "Input.insertText", map[string]any{"text": m.Text})
		case "cancel":
			_ = b.cdp.Dispatch(s, "Input.imeSetComposition", map[string]any{
				"text": "", "selectionStart": 0, "selectionEnd": 0,
			})
		}
	default:
		b.handleFeatureMessage(c, t, s, command)
	}
}

func (b *Controller) handleMobileLayout(on bool) {
	b.mu.Lock()
	changed := b.mobile != on
	b.mobile = on
	t := b.tabs[b.activeID]
	var sessions []string
	for _, tab := range b.tabs {
		if tab.Session != "" {
			sessions = append(sessions, tab.Session)
		}
	}
	userAgentParams := userAgentOverrideParams(b.userAgent, b.userAgentMetadata, on)
	b.mu.Unlock()
	log.Printf("layout: request mobile sites=%t changed=%t", on, changed)
	if !changed {
		return
	}
	b.touch.cancel(true)
	for _, session := range sessions {
		_ = b.cdp.Dispatch(session, "Network.enable", nil)
		if userAgentParams != nil {
			_ = b.cdp.Dispatch(session, "Network.setUserAgentOverride", userAgentParams)
		}
	}
	if t == nil {
		return
	}
	b.applyView(t)
	b.mu.Lock()
	session, pageURL := t.Session, t.URL
	b.mu.Unlock()
	if strings.HasPrefix(pageURL, "http://") || strings.HasPrefix(pageURL, "https://") {
		b.mu.Lock()
		t.loading = true
		b.mu.Unlock()
		_ = b.cdp.Dispatch(session, "Page.reload", map[string]any{})
	}
}

func darkModeMediaParams(dark bool) map[string]any {
	value := "light"
	if dark {
		value = "dark"
	}
	return map[string]any{
		"features": []map[string]any{{"name": "prefers-color-scheme", "value": value}},
	}
}

func (b *Controller) applyDarkMode(session string, dark bool) {
	if session == "" {
		return
	}
	// Advertise the user's preference and let each page use its authored theme.
	// This is per target, so it must also run during every future tab attach.
	_ = b.cdp.Dispatch(session, "Emulation.setEmulatedMedia", darkModeMediaParams(dark))
}

func (b *Controller) handleDarkMode(on bool) {
	b.mu.Lock()
	changed := b.dark != on
	b.dark = on
	var sessions []string
	for _, tab := range b.tabs {
		if tab.Session != "" {
			sessions = append(sessions, tab.Session)
		}
	}
	b.mu.Unlock()
	log.Printf("appearance: dark mode=%t changed=%t", on, changed)
	if !changed {
		return
	}
	for _, session := range sessions {
		b.applyDarkMode(session, on)
	}
	b.video.RequestKeyframe()
}

func (b *Controller) controlPageMedia(c *transport.Client, session, command string, value float64) {
	action := `var media=document.querySelectorAll('video,audio');
var shouldPause=Array.prototype.some.call(media,function(m){return !m.paused;});
Array.prototype.forEach.call(media,function(m){
  if(shouldPause)m.pause();else{var p=m.play();if(p&&p.catch)p.catch(function(){});}
});`
	if command == "mute" {
		action = `var media=document.querySelectorAll('video,audio');
var shouldMute=Array.prototype.some.call(media,function(m){return !m.muted;});
Array.prototype.forEach.call(media,function(m){m.muted=shouldMute;});`
	} else if command == "volume" {
		value = math.Max(0, math.Min(1, value))
		action = fmt.Sprintf(`var media=document.querySelectorAll('video,audio');
Array.prototype.forEach.call(media,function(m){m.volume=%0.4f;if(m.volume>0)m.muted=false;});`, value)
	}
	_ = b.cdp.Dispatch(session, "Runtime.evaluate", map[string]any{
		"expression":  "(function(){" + action + "})()",
		"userGesture": true,
	})
	time.AfterFunc(120*time.Millisecond, func() { b.queryPageMedia(c, session) })
}

const mediaStateExpr = `(function(){
  var all=Array.prototype.slice.call(document.querySelectorAll('video,audio'));
  if(!all.length)return {available:false,count:0,paused:true,muted:false,volume:1,currentTime:0,duration:0,title:''};
  var active=all.filter(function(m){return !m.paused;})[0]||all[0];
  var d=Number(active.duration), t=Number(active.currentTime);
  return {
    available:true,count:all.length,paused:all.every(function(m){return m.paused;}),
    muted:all.every(function(m){return m.muted||m.volume===0;}),volume:Number(active.volume),
    currentTime:isFinite(t)?t:0,duration:isFinite(d)?d:0,
    title:(active.getAttribute('aria-label')||active.getAttribute('title')||document.title||'').slice(0,160)
  };
})()`

func (b *Controller) queryPageMedia(c *transport.Client, session string) {
	if c == nil || !b.isActiveSession(session) {
		return
	}
	res, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression": mediaStateExpr, "returnByValue": true,
	})
	if err != nil || !b.isActiveSession(session) {
		return
	}
	var reply struct {
		Result struct {
			Value protocol.MediaStateEvent `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(res, &reply) != nil {
		return
	}
	reply.Result.Value.Type = "media-state"
	b.send(c, reply.Result.Value)
}

func renderCommand(t string) bool {
	switch t {
	case "touch", "key", "paste", "compose", "selectreply", "nav", "reload", "back", "fwd":
		return true
	}
	return false
}

func (b *Controller) noteMotionPhase(phase string) {
	b.perfMu.Lock()
	switch phase {
	case "begin":
		b.motionActive = true
		b.motionLastAUAt = time.Time{}
		b.motionLastSourceAt = time.Time{}
		b.motionStallLogged = false
	case "move":
		if !b.motionActive {
			b.motionActive = true
			b.motionLastAUAt = time.Time{}
			b.motionLastSourceAt = time.Time{}
			b.motionStallLogged = false
		}
	case "end":
		b.motionActive = false
		b.motionStallLogged = false
	}
	b.perfMu.Unlock()
}

func (b *Controller) checkMotionStall(now time.Time) {
	b.perfMu.Lock()
	if !b.motionActive || b.motionStallLogged {
		b.perfMu.Unlock()
		return
	}
	since := b.motionLastAUAt
	age := now.Sub(since)
	// A scroll can begin on a non-scrollable page or against its boundary.
	// Until Chromium emits one encoded frame there is no active video
	// cadence to stall; input-to-first-frame is measured separately.
	if since.IsZero() || age < 75*time.Millisecond {
		b.perfMu.Unlock()
		return
	}
	b.motionStallLogged = true
	b.motionStalls++
	stalls := b.motionStalls
	b.perfMu.Unlock()
	telemetry.Emit("motion_au_stall", "capture", "webcodecs", map[string]any{
		"age_ms": float64(age.Microseconds()) / 1000.0,
	})
	log.Printf("capture: no encoded AU for %.1fms during touch motion (stalls=%d)",
		float64(age.Microseconds())/1000.0, stalls)
}

func (b *Controller) noteClientMessage(t string) {
	now := time.Now()
	b.perfMu.Lock()
	if b.perfSince.IsZero() {
		b.perfSince = now
	}
	b.perfCounts[t]++
	if now.Sub(b.perfSince) < 5*time.Second {
		b.perfMu.Unlock()
		return
	}
	dt := now.Sub(b.perfSince).Seconds()
	counts := b.perfCounts
	latN, latSumMS, latMaxMS := b.perfLatN, b.perfLatSumMS, b.perfLatMaxMS
	aLatN, aLatSumMS, aLatMaxMS := b.perfALatN, b.perfALatSumMS, b.perfALatMaxMS
	auN, auSumMS, auMaxMS := b.auLatN, b.auLatSumMS, b.auLatMaxMS
	motionN, motionSumMS, motionMaxMS := b.motionGapN, b.motionGapSumMS, b.motionGapMaxMS
	sourceN, sourceSumMS, sourceMaxMS := b.sourceGapN, b.sourceGapSumMS, b.sourceGapMaxMS
	duplicateAUs := b.duplicateAUs
	rawN, rawSumMS, rawMaxMS := b.rawGapN, b.rawGapSumMS, b.rawGapMaxMS
	submitN, submitSumMS, submitMaxMS := b.submitWaitN, b.submitWaitSumMS, b.submitWaitMaxMS
	encodeN, encodeSumMS, encodeMaxMS := b.encodeTimeN, b.encodeTimeSumMS, b.encodeTimeMaxMS
	b.perfCounts = map[string]int{}
	b.perfLatN, b.perfLatSumMS, b.perfLatMaxMS = 0, 0, 0
	b.perfALatN, b.perfALatSumMS, b.perfALatMaxMS = 0, 0, 0
	b.auLatN, b.auLatSumMS, b.auLatMaxMS = 0, 0, 0
	b.motionGapN, b.motionGapSumMS, b.motionGapMaxMS = 0, 0, 0
	b.sourceGapN, b.sourceGapSumMS, b.sourceGapMaxMS = 0, 0, 0
	b.duplicateAUs = 0
	b.rawGapN, b.rawGapSumMS, b.rawGapMaxMS = 0, 0, 0
	b.submitWaitN, b.submitWaitSumMS, b.submitWaitMaxMS = 0, 0, 0
	b.encodeTimeN, b.encodeTimeSumMS, b.encodeTimeMaxMS = 0, 0, 0
	b.perfSince = now
	b.perfMu.Unlock()

	latMeanMS := 0.0
	if latN > 0 {
		latMeanMS = latSumMS / float64(latN)
	}
	aLatMeanMS := 0.0
	if aLatN > 0 {
		aLatMeanMS = aLatSumMS / float64(aLatN)
	}
	auMeanMS := 0.0
	if auN > 0 {
		auMeanMS = auSumMS / float64(auN)
	}
	motionMeanMS := 0.0
	if motionN > 0 {
		motionMeanMS = motionSumMS / float64(motionN)
	}
	sourceMeanMS := 0.0
	if sourceN > 0 {
		sourceMeanMS = sourceSumMS / float64(sourceN)
	}
	mean := func(sum float64, n int) float64 {
		if n == 0 {
			return 0
		}
		return sum / float64(n)
	}
	logs.Info("performance", "Input and media performance sample", map[string]any{
		"interval_seconds":    dt,
		"touch_per_second":    float64(counts["touch"]) / dt,
		"key_per_second":      float64(counts["key"]) / dt,
		"navigation_commands": counts["nav"], "size_commands": counts["size"],
		"other_commands":         otherInputCount(counts),
		"input_to_image_mean_ms": auMeanMS, "input_to_image_max_ms": auMaxMS, "input_to_image_samples": auN,
		"motion_au_gap_mean_ms": motionMeanMS, "motion_au_gap_max_ms": motionMaxMS, "motion_au_gap_samples": motionN,
		"fresh_au_gap_mean_ms": sourceMeanMS, "fresh_au_gap_max_ms": sourceMaxMS, "fresh_au_gap_samples": sourceN,
		"duplicate_aus":           duplicateAUs,
		"capture_raw_gap_mean_ms": mean(rawSumMS, rawN), "capture_raw_gap_max_ms": rawMaxMS,
		"submit_wait_mean_ms": mean(submitSumMS, submitN), "submit_wait_max_ms": submitMaxMS,
		"encode_mean_ms": mean(encodeSumMS, encodeN), "encode_max_ms": encodeMaxMS,
		"video_queue_mean_ms": latMeanMS, "video_queue_max_ms": latMaxMS, "video_queue_samples": latN,
		"audio_latency_mean_ms": aLatMeanMS, "audio_latency_max_ms": aLatMaxMS, "audio_latency_samples": aLatN,
	})
}

func (b *Controller) noteCaptureStages(frame media.VideoFrame) {
	b.perfMu.Lock()
	record := func(d time.Duration, n *int, sum, max *float64) {
		if d <= 0 {
			return
		}
		ms := float64(d) / float64(time.Millisecond)
		*n++
		*sum += ms
		if ms > *max {
			*max = ms
		}
	}
	record(frame.RawGap, &b.rawGapN, &b.rawGapSumMS, &b.rawGapMaxMS)
	record(frame.SubmitWait, &b.submitWaitN, &b.submitWaitSumMS, &b.submitWaitMaxMS)
	record(frame.EncodeTime, &b.encodeTimeN, &b.encodeTimeSumMS, &b.encodeTimeMaxMS)
	b.perfMu.Unlock()
}

func (b *Controller) noteRenderInput() {
	b.perfMu.Lock()
	b.lastRenderInputAt = time.Now()
	b.perfMu.Unlock()
}

func (b *Controller) noteEncodedAU(fresh bool) {
	now := time.Now()
	b.perfMu.Lock()
	if b.motionActive {
		if !b.motionLastAUAt.IsZero() {
			gapMS := float64(now.Sub(b.motionLastAUAt).Microseconds()) / 1000.0
			b.motionGapN++
			b.motionGapSumMS += gapMS
			if gapMS > b.motionGapMaxMS {
				b.motionGapMaxMS = gapMS
			}
		}
		b.motionLastAUAt = now
		b.motionStallLogged = false
		if fresh {
			if !b.motionLastSourceAt.IsZero() {
				gapMS := float64(now.Sub(b.motionLastSourceAt).Microseconds()) / 1000.0
				b.sourceGapN++
				b.sourceGapSumMS += gapMS
				if gapMS > b.sourceGapMaxMS {
					b.sourceGapMaxMS = gapMS
				}
			}
			b.motionLastSourceAt = now
		}
	}
	if !fresh {
		b.duplicateAUs++
	}
	// A paced duplicate keeps presentation fluid but does not contain the
	// result of a new interaction. Only fresh compositor images may satisfy
	// input-to-image latency accounting.
	if fresh && !b.lastRenderInputAt.IsZero() {
		ms := float64(now.Sub(b.lastRenderInputAt).Microseconds()) / 1000.0
		b.auLatN++
		b.auLatSumMS += ms
		if ms > b.auLatMaxMS {
			b.auLatMaxMS = ms
		}
		b.lastRenderInputAt = time.Time{}
	}
	b.perfMu.Unlock()
}

// noteVideoLatency records one AU's per-subscriber queue-dwell time (time
// from splitter assembly to the SendBinary call) into the same rolling
// window noteClientMessage flushes, so one log line covers both input and
// video-pipeline health.
func (b *Controller) noteVideoLatency(d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)
	b.perfMu.Lock()
	if b.perfSince.IsZero() {
		b.perfSince = time.Now()
	}
	b.perfLatN++
	b.perfLatSumMS += ms
	if ms > b.perfLatMaxMS {
		b.perfLatMaxMS = ms
	}
	b.perfMu.Unlock()
}

// noteAudioLatency mirrors noteVideoLatency for the PCM lane.
func (b *Controller) noteAudioLatency(d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)
	b.perfMu.Lock()
	if b.perfSince.IsZero() {
		b.perfSince = time.Now()
	}
	b.perfALatN++
	b.perfALatSumMS += ms
	if ms > b.perfALatMaxMS {
		b.perfALatMaxMS = ms
	}
	b.perfMu.Unlock()
}

func otherInputCount(counts map[string]int) int {
	known := map[string]bool{
		"touch": true, "key": true, "compose": true, "nav": true, "size": true, "fullscreen": true,
	}
	n := 0
	for k, v := range counts {
		if !known[k] {
			n += v
		}
	}
	return n
}

func (b *Controller) handleSize(m *protocol.SizeCommand) {
	b.mu.Lock()
	w, h := normalizeViewportSize(m.W, m.H, b.viewW, b.viewH)
	// Chromium's H.264 WebCodecs implementation rejects odd dimensions.
	// Normalize the viewport itself so the page, compositor surface, raw
	// tab frame, encoder, and wire header all describe the same pixels.
	w &^= 1
	h &^= 1
	changed := w != b.viewW || h != b.viewH
	if changed {
		b.viewW, b.viewH = w, h
	}
	b.mu.Unlock()
	log.Printf("size: client asked %dx%d -> view %dx%d (changed=%t)", m.W, m.H, w, h, changed)
	if !changed {
		return
	}
	b.touch.cancel(true)
	b.scheduleViewportApply()
}

func normalizeViewportSize(width, height, defaultWidth, defaultHeight int) (int, int) {
	clampDim := func(value, fallback int) int {
		if value == 0 {
			value = fallback
		}
		// These are encoder/resource safety bounds, not device profiles. Keep
		// every client-derived size inside them exactly (modulo H.264's even
		// dimensions); a 320-point minimum once broke real phone landscape
		// surfaces by inventing pixels that were not in the native stream view.
		return min(maxViewportDimension, max(minViewportDimension, value)) &^ 1
	}
	return clampDim(width, defaultWidth), clampDim(height, defaultHeight)
}

// scheduleViewportApply coalesces the transient layouts emitted while native
// chrome rotates or enters fullscreen. Chromium's compositor and tabCapture
// are reconfigured only for the final dimensions, so the authenticated socket
// and its media subscription stay alive through the transition.
func (b *Controller) scheduleViewportApply() {
	b.resizeMu.Lock()
	if b.resizeClosed {
		b.resizeMu.Unlock()
		return
	}
	b.resizeGen++
	generation := b.resizeGen
	if b.resizeTimer != nil {
		b.resizeTimer.Stop()
	}
	b.resizeTimer = time.AfterFunc(viewportResizeSettle, func() {
		b.applySettledViewport(generation)
	})
	b.resizeMu.Unlock()
}

func (b *Controller) applySettledViewport(generation uint64) {
	b.resizeMu.Lock()
	if b.resizeClosed || generation != b.resizeGen {
		b.resizeMu.Unlock()
		return
	}
	b.resizeTimer = nil
	b.resizeMu.Unlock()

	b.mu.Lock()
	w, h := b.viewW, b.viewH
	profile := adaptiveProfiles[b.governor.profile]
	b.mu.Unlock()
	if tab := b.active(); tab != nil {
		b.applyView(tab)
	}
	maxW, maxH := adaptiveScale(profile, w, h)
	b.video.SetViewport(w, h, maxW, maxH)
}

func (b *Controller) handleTab(m *protocol.TabCommand) {
	switch m.Action {
	case "select":
		b.switchActive(m.ID)
	case "close":
		b.mu.Lock()
		t := b.tabs[m.ID]
		var target string
		if t != nil {
			target = t.TargetID
		}
		b.mu.Unlock()
		if target != "" {
			_, _ = b.cdp.Call("", "Target.closeTarget", map[string]any{"targetId": target})
		}
	case "new":
		// Attach and capture a stable blank surface first. Navigating only
		// after that first frame reaches the pipeline makes new-tab switching
		// immediate even while the network page initializes.
		_, _ = b.cdp.Call("", "Target.createTarget", b.newTargetParams("about:blank#surf-new"))
	}
}

func (b *Controller) navigateHistory(session string, back bool) bool {
	h, err := b.cdp.NavigationHistory(session)
	if err != nil {
		return false
	}
	i := h.CurrentIndex + 1
	if back {
		i = h.CurrentIndex - 1
	}
	if i >= 0 && i < len(h.Entries) {
		return b.cdp.NavigateToHistoryEntry(session, h.Entries[i].ID) == nil
	}
	return false
}

func (b *Controller) setTouchMode(session string) {
	_ = b.cdp.Dispatch(session, "Emulation.setTouchEmulationEnabled", map[string]any{
		"enabled": true, "maxTouchPoints": maxTouchContacts,
	})
}
