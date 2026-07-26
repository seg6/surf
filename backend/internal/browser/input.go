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

// ClientConnected implements ws.Handler: greet, sync tabs/url, push a frame.
func (b *Browser) ClientConnected(c *ws.Client) {
	b.mu.Lock()
	w, h := b.viewW, b.viewH
	b.mu.Unlock()
	c.SendJSON(map[string]any{"t": "hello", "vw": w, "vh": h})
	c.SendJSON(map[string]any{"t": "tabs", "tabs": b.tabList()})
	if t := b.active(); t != nil {
		b.mu.Lock()
		u := t.URL
		b.mu.Unlock()
		c.SendJSON(b.urlMessage(u))
		b.pushNavState()
		// The cast may be parked (all previous clients were video-mode or
		// gone); a new JPEG consumer brings it back.
		b.ensureCast(t)
		go b.sendFreshFrame(c, 0)
	}
}

// HandleMessage implements ws.Handler. It runs on the client's read goroutine,
// so per-client message order is preserved (press before release, etc.).
func (b *Browser) HandleMessage(c *ws.Client, m *protocol.ClientMessage) {
	b.noteClientMessage(m.T)
	switch m.T {
	case "ready":
		c.Ready()
		return
	case "poke":
		c.PokeReset()
		go b.sendFreshFrame(c, 0)
		return
	case "size":
		b.handleSize(c, m)
		return
	case "tab":
		b.handleTab(m)
		return
	case "video":
		b.handleVideo(c, m.On)
		return
	case "stream":
		b.handleStreamProfile(c, m.Profile)
		return
	case "audio":
		b.handleAudio(c, m.On)
		return
	case "reqkeyframe":
		// Client lost decode sync (VT session error / bad SPS-PPS) and wants an
		// early IDR instead of waiting for the fixed 2s cadence.
		b.streamer.RequestKeyframe()
		return
	case "lat":
		// Latency echo (M1.1): immediate bounce, client computes RTT.
		c.SendJSON(map[string]any{"t": "lat", "id": m.ID})
		return
	case "dialogreply":
		b.handleDialogReply(m)
		return
	case "opennew":
		b.openInNewTab(m.URL)
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

	switch m.T {
	case "nav":
		// Fire-and-forget: Page.navigate doesn't answer until the navigation
		// commits, and the WS reader must stay responsive meanwhile.
		if u := NormalizeNavURL(m.URL); u != "" {
			log.Printf("nav -> %s", u)
			b.cdp.Send(s, "Page.navigate", map[string]any{"url": u})
		}
	case "reload":
		b.cdp.Send(s, "Page.reload", map[string]any{})
	case "stop":
		_, _ = b.cdp.Call(s, "Page.stopLoading", nil)
	case "back", "fwd":
		b.navigateHistory(s, m.T == "back")
	case "click":
		b.beginMotion(t)
		b.mouse(s, "mousePressed", m.X*cssW, m.Y*cssH, 1)
		b.mouse(s, "mouseReleased", m.X*cssW, m.Y*cssH, 1)
		b.checkEditable(c, s)
	case "wheel":
		b.beginMotion(t)
		b.cdp.Send(s, "Input.dispatchMouseEvent", map[string]any{
			"type": "mouseWheel", "x": math.Round(m.X * cssW), "y": math.Round(m.Y * cssH),
			"deltaX": m.DX * cssW, "deltaY": m.DY * cssH,
		})
	case "lpdown":
		// Long-press: a real mouse-down that stays down. Moving drags
		// (sliders, maps, text selection); releasing in place selects the
		// word underneath (handled on lpup).
		b.beginMotion(t)
		b.mouse(s, "mousePressed", m.X*cssW, m.Y*cssH, 1)
	case "lpmove":
		b.beginMotion(t)
		b.cdp.Send(s, "Input.dispatchMouseEvent", map[string]any{
			"type": "mouseMoved", "x": math.Round(m.X * cssW), "y": math.Round(m.Y * cssH),
			"button": "left", "buttons": 1,
		})
	case "lpup":
		b.mouse(s, "mouseReleased", m.X*cssW, m.Y*cssH, 1)
		b.finishLongpress(c, t, s, m.X*cssW, m.Y*cssH, m.Sel)
	case "key":
		b.beginMotion(t)
		if m.Text != "" {
			b.cdp.Send(s, "Input.dispatchKeyEvent", map[string]any{"type": "char", "text": m.Text})
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
			b.cdp.Send(s, "Input.dispatchKeyEvent", params)
		}
	default:
		b.handleFeatureMessage(c, t, s, m)
	}
}

func (b *Browser) noteClientMessage(t string) {
	if t == "ready" {
		return
	}
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
	b.perfCounts = map[string]int{}
	b.perfLatN, b.perfLatSumMS, b.perfLatMaxMS = 0, 0, 0
	b.perfALatN, b.perfALatSumMS, b.perfALatMaxMS = 0, 0, 0
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
	log.Printf("perf input %.1fs: click=%.1f/s wheel=%.1f/s key=%.1f/s nav=%d size=%d video=%d poke=%d lp=%d other=%d | video lat mean=%.1fms max=%.1fms n=%d | audio lat mean=%.1fms max=%.1fms n=%d",
		dt,
		float64(counts["click"])/dt,
		float64(counts["wheel"])/dt,
		float64(counts["key"])/dt,
		counts["nav"], counts["size"], counts["video"], counts["poke"],
		counts["lpdown"]+counts["lpmove"]+counts["lpup"],
		otherInputCount(counts),
		latMeanMS, latMaxMS, latN,
		aLatMeanMS, aLatMaxMS, aLatN)
}

// noteVideoLatency records one AU's per-subscriber queue-dwell time (time
// from splitter assembly to the SendBinary call) into the same rolling
// window noteClientMessage flushes, so one log line covers both input and
// video-pipeline health.
func (b *Browser) noteVideoLatency(d time.Duration) {
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
func (b *Browser) noteAudioLatency(d time.Duration) {
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
		"video": true, "poke": true, "lpdown": true, "lpmove": true, "lpup": true,
	}
	n := 0
	for k, v := range counts {
		if !known[k] {
			n += v
		}
	}
	return n
}

func (b *Browser) handleSize(c *ws.Client, m *protocol.ClientMessage) {
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
		go b.sendFreshFrame(c, 0)
		return
	}
	t := b.active()
	if t == nil {
		return
	}
	b.stopCast(t)
	b.applyView(t)
	go b.syncVideoSurface(w, h)
	b.mu.Lock()
	t.motion = false
	b.mu.Unlock()
	b.ensureCast(t)
	go b.sendSharpFrame(nil, t)
}

func (b *Browser) handleTab(m *protocol.ClientMessage) {
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
		_, _ = b.cdp.Call("", "Target.createTarget", map[string]any{"url": b.cfg.StartURL})
	}
}

func (b *Browser) navigateHistory(session string, back bool) {
	res, err := b.cdp.Call(session, "Page.getNavigationHistory", nil)
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
	i := h.CurrentIndex + 1
	if back {
		i = h.CurrentIndex - 1
	}
	if i >= 0 && i < len(h.Entries) {
		_, _ = b.cdp.Call(session, "Page.navigateToHistoryEntry", map[string]any{"entryId": h.Entries[i].ID})
	}
}

func (b *Browser) mouse(session, typ string, x, y float64, clicks int) {
	_, _ = b.cdp.Call(session, "Input.dispatchMouseEvent", map[string]any{
		"type": typ, "x": math.Round(x), "y": math.Round(y),
		"button": "left", "clickCount": clicks,
	})
}

// checkEditable tells the tapping client whether focus landed in a text field
// (so it can raise the iOS keyboard). Waits 180ms for focus to settle.
func (b *Browser) checkEditable(c *ws.Client, session string) {
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
		msg := map[string]any{"t": "editable", "on": v.On}
		// kind selects the keyboard type; rect (viewport fractions) drives
		// keyboard avoidance.
		if v.On {
			msg["kind"] = v.Kind
			if len(v.Rect) == 4 {
				msg["rect"] = v.Rect
			}
		}
		c.SendJSON(msg)
	})
}
