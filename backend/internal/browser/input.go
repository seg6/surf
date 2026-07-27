package browser

import (
	"encoding/json"
	"log"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"surf-backend/internal/protocol"
	"surf-backend/internal/telemetry"
	"surf-backend/internal/ws"
)

var schemeRe = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*:`)

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

// editableExpr reports whether focus sits in a text-entry element, plus the
// keyboard kind and the element's bounding box in viewport fractions (divided
// by innerWidth/Height in-page, so zoom math never leaks to the server).
const editableExpr = `(function(){
  var e = document.activeElement;
  var on = false, kind = 'text';
  if (e) {
    var t = e.tagName;
    if (t === 'TEXTAREA') { on = true; kind = 'textarea'; }
    else if (t === 'SELECT') { on = true; kind = 'select'; }
    else if (e.isContentEditable) { on = true; }
    else if (t === 'INPUT') {
      var ty = (e.type || 'text').toLowerCase();
      var skip = {button:1,checkbox:1,radio:1,submit:1,reset:1,file:1,image:1,range:1,color:1,hidden:1,date:1,time:1};
      if (!skip[ty]) {
        on = true;
        kind = ({password:'password',email:'email',number:'number',tel:'number',url:'url',search:'search'})[ty] || 'text';
      }
    }
  }
  if (!on) return {on:false};
  var r = e.getBoundingClientRect();
  var w = window.innerWidth || 1, h = window.innerHeight || 1;
  return {on:true, kind:kind, rect:[r.left/w, r.top/h, r.width/w, r.height/h]};
})()`

// ClientConnected implements ws.Handler: greet, sync state, and automatically
// subscribe the client to the only visual lane (H.264).
func (b *Controller) ClientConnected(c *ws.ClientTransport) {
	b.mu.Lock()
	w, h := b.viewW, b.viewH
	b.mu.Unlock()
	c.SendJSON(protocol.HelloEvent{Type: "hello", W: w, H: h})
	c.SendJSON(protocol.TabsEvent{Type: "tabs", Tabs: b.tabList()})
	// The native controller sends its laid-out viewport immediately after the
	// socket opens. Let that ordered message settle before spawning FFmpeg so
	// startup does not encode at the config default and then restart twice.
	time.AfterFunc(150*time.Millisecond, func() {
		select {
		case <-c.Closed():
			return
		default:
			b.subscribeVideo(c)
		}
	})
	if t := b.active(); t != nil {
		b.mu.Lock()
		u := t.URL
		b.mu.Unlock()
		c.SendJSON(b.urlMessage(u))
		b.pushNavState()
	}
}

// HandleMessage implements ws.Handler. It runs on the client's read goroutine,
// and only enqueues immutable typed commands. The controller goroutine is the
// sole ordered executor (press before release, key order, tab commands).
func (b *Controller) HandleMessage(c *ws.ClientTransport, command protocol.Command) {
	select {
	case b.commands <- controllerCommand{client: c, command: command}:
	case <-c.Closed():
	default:
		c.Close()
	}
}

func (b *Controller) handleCommand(c *ws.ClientTransport, command protocol.Command) {
	kind := command.Kind()
	telemetry.Emit("client_command", "input", "controller", map[string]any{"type": kind})
	iid, _ := command.Causal()
	b.noteClientMessage(kind)
	if iid != 0 && renderCommand(kind) {
		b.noteRenderInput()
		defer func() {
			b.perfMu.Lock()
			b.interactionID = iid
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
		}
		return
	case *protocol.DialogReplyCommand:
		b.handleDialogReply(m)
		return
	case *protocol.URLCommand:
		if kind == "opennew" {
			b.openInNewTab(m.URL)
			return
		}
	}
	switch kind {
	case "video-retry":
		b.stopVideo(c)
		b.video.ResetCrashBudget()
		b.subscribeVideo(c)
		return
	case "reqkeyframe":
		// Client lost decode sync (VT session error / bad SPS-PPS) and wants an
		// early IDR instead of waiting for the fixed 2s cadence.
		b.video.RequestKeyframe()
		return
	}

	t := b.active()
	if t == nil {
		return
	}
	b.mu.Lock()
	s := t.Session
	z := t.Zoom
	vw, vh := b.viewW, b.viewH
	b.mu.Unlock()
	if z < 1 {
		z = 1
	}
	// Input coordinates arrive as viewport fractions; the page works in CSS
	// pixels of the (possibly zoomed) emulated viewport.
	cssW, cssH := float64(vw)/z, float64(vh)/z

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
			b.mu.Lock()
			t.loading = true
			b.mu.Unlock()
			_ = b.cdp.Dispatch(s, "Page.navigate", map[string]any{"url": u})
		}
	case *protocol.EmptyCommand:
		switch kind {
		case "reload":
			b.mu.Lock()
			t.loading = true
			b.mu.Unlock()
			_ = b.cdp.Dispatch(s, "Page.reload", map[string]any{})
		case "stop":
			_, _ = b.cdp.Call(s, "Page.stopLoading", nil)
		case "back", "fwd":
			b.mu.Lock()
			t.loading = true
			b.mu.Unlock()
			b.navigateHistory(s, kind == "back")
		default:
			b.handleFeatureMessage(c, t, s, command)
		}
	case *protocol.PointCommand:
		switch kind {
		case "click":
			b.mouse(s, "mousePressed", m.X*cssW, m.Y*cssH, 1)
			b.mouse(s, "mouseReleased", m.X*cssW, m.Y*cssH, 1)
			b.checkEditable(c, s)
		case "lpdown":
			b.mouse(s, "mousePressed", m.X*cssW, m.Y*cssH, 1)
		case "lpmove":
			_ = b.cdp.Dispatch(s, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved", "x": math.Round(m.X * cssW), "y": math.Round(m.Y * cssH),
				"button": "left", "buttons": 1,
			})
		default:
			b.handleFeatureMessage(c, t, s, command)
		}
	case *protocol.WheelCommand:
		_ = b.cdp.Dispatch(s, "Input.dispatchMouseEvent", map[string]any{
			"type": "mouseWheel", "x": math.Round(m.X * cssW), "y": math.Round(m.Y * cssH),
			"deltaX": m.DX * cssW, "deltaY": m.DY * cssH,
		})
	case *protocol.LongPressUpCommand:
		b.mouse(s, "mouseReleased", m.X*cssW, m.Y*cssH, 1)
		b.finishLongpress(c, t, s, m.X*cssW, m.Y*cssH, m.Sel)
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
	default:
		b.handleFeatureMessage(c, t, s, command)
	}
}

func renderCommand(t string) bool {
	switch t {
	case "click", "wheel", "lpdown", "lpmove", "lpup", "key", "paste", "nav", "reload", "back", "fwd", "zoom":
		return true
	}
	return false
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
	sourceN, sourceSumMS, sourceMaxMS := b.sourceLatN, b.sourceLatSumMS, b.sourceLatMaxMS
	encodeN, encodeSumMS, encodeMaxMS := b.encodeLatN, b.encodeLatSumMS, b.encodeLatMaxMS
	b.perfCounts = map[string]int{}
	b.perfLatN, b.perfLatSumMS, b.perfLatMaxMS = 0, 0, 0
	b.perfALatN, b.perfALatSumMS, b.perfALatMaxMS = 0, 0, 0
	b.sourceLatN, b.sourceLatSumMS, b.sourceLatMaxMS = 0, 0, 0
	b.encodeLatN, b.encodeLatSumMS, b.encodeLatMaxMS = 0, 0, 0
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
	sourceMeanMS := 0.0
	if sourceN > 0 {
		sourceMeanMS = sourceSumMS / float64(sourceN)
	}
	encodeMeanMS := 0.0
	if encodeN > 0 {
		encodeMeanMS = encodeSumMS / float64(encodeN)
	}
	log.Printf("perf input %.1fs: click=%.1f/s wheel=%.1f/s key=%.1f/s nav=%d size=%d lp=%d other=%d | input->source mean=%.1fms max=%.1fms n=%d | source->encode mean=%.1fms max=%.1fms n=%d | video queue mean=%.1fms max=%.1fms n=%d | audio lat mean=%.1fms max=%.1fms n=%d",
		dt,
		float64(counts["click"])/dt,
		float64(counts["wheel"])/dt,
		float64(counts["key"])/dt,
		counts["nav"], counts["size"],
		counts["lpdown"]+counts["lpmove"]+counts["lpup"],
		otherInputCount(counts),
		sourceMeanMS, sourceMaxMS, sourceN,
		encodeMeanMS, encodeMaxMS, encodeN,
		latMeanMS, latMaxMS, latN,
		aLatMeanMS, aLatMaxMS, aLatN)
}

func (b *Controller) noteEncodeLatency(sourceNS, encodeNS uint64) {
	if sourceNS == 0 || encodeNS < sourceNS {
		return
	}
	ms := float64(encodeNS-sourceNS) / 1e6
	b.perfMu.Lock()
	b.encodeLatN++
	b.encodeLatSumMS += ms
	if ms > b.encodeLatMaxMS {
		b.encodeLatMaxMS = ms
	}
	b.perfMu.Unlock()
}

func (b *Controller) noteRenderInput() {
	b.perfMu.Lock()
	b.lastRenderInputAt = time.Now()
	b.perfMu.Unlock()
}

func (b *Controller) noteSourceFrame() {
	now := time.Now()
	b.perfMu.Lock()
	if !b.lastRenderInputAt.IsZero() {
		ms := float64(now.Sub(b.lastRenderInputAt).Microseconds()) / 1000.0
		b.sourceLatN++
		b.sourceLatSumMS += ms
		if ms > b.sourceLatMaxMS {
			b.sourceLatMaxMS = ms
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
		"click": true, "wheel": true, "key": true, "nav": true, "size": true,
		"lpdown": true, "lpmove": true, "lpup": true,
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
	clampDim := func(v, def int) int {
		if v == 0 {
			return def
		}
		return min(1600, max(320, v))
	}
	b.mu.Lock()
	w := clampDim(m.W, b.viewW)
	h := clampDim(m.H, b.viewH)
	changed := w != b.viewW || h != b.viewH
	if changed {
		b.viewW, b.viewH = w, h
	}
	b.mu.Unlock()
	log.Printf("size: client asked %dx%d -> view %dx%d (changed=%t)", m.W, m.H, w, h, changed)
	if !changed {
		return
	}
	t := b.active()
	if t == nil {
		return
	}
	b.stopCast(t)
	b.applyView(t)
	b.requestVideoResize(w, h, nil, "")
	b.ensureCast(t)
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

func (b *Controller) navigateHistory(session string, back bool) {
	h, err := b.cdp.NavigationHistory(session)
	if err != nil {
		return
	}
	i := h.CurrentIndex + 1
	if back {
		i = h.CurrentIndex - 1
	}
	if i >= 0 && i < len(h.Entries) {
		_ = b.cdp.NavigateToHistoryEntry(session, h.Entries[i].ID)
	}
}

func (b *Controller) mouse(session, typ string, x, y float64, clicks int) {
	_ = b.cdp.Dispatch(session, "Input.dispatchMouseEvent", map[string]any{
		"type": typ, "x": math.Round(x), "y": math.Round(y),
		"button": "left", "clickCount": clicks,
	})
}

// checkEditable tells the tapping client whether focus landed in a text field
// (so it can raise the iOS keyboard). Waits 180ms for focus to settle.
func (b *Controller) checkEditable(c *ws.ClientTransport, session string) {
	time.AfterFunc(180*time.Millisecond, func() {
		if !b.isActiveSession(session) {
			return
		}
		res, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
			"expression": editableExpr, "returnByValue": true,
		})
		if err != nil {
			return
		}
		var p struct {
			Result struct {
				Value struct {
					On   bool      `json:"on"`
					Kind string    `json:"kind"`
					Rect []float64 `json:"rect"`
				} `json:"value"`
			} `json:"result"`
		}
		if json.Unmarshal(res, &p) != nil {
			return
		}
		if !b.isActiveSession(session) {
			return
		}
		v := p.Result.Value
		msg := protocol.EditableEvent{Type: "editable", On: v.On}
		// kind selects the keyboard type; rect (viewport fractions) drives
		// keyboard avoidance.
		if v.On {
			msg.Kind = v.Kind
			if len(v.Rect) == 4 {
				msg.Rect = v.Rect
			}
		}
		c.SendJSON(msg)
	})
}
