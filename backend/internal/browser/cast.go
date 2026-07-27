package browser

import (
	"log"
	"strconv"
	"strings"
	"time"

	"surf-backend/internal/protocol"
	"surf-backend/internal/ws"
)

func clampQuality(q, def int) int {
	if q <= 0 {
		q = def
	}
	if q < 1 {
		return 1
	}
	if q > 100 {
		return 100
	}
	return q
}

// desiredCastLocked derives the wanted screencast parameters from the tab's
// motion state: cheaper frames while the user interacts, full quality when
// still. Always full resolution — the native decode path handles it.
//
// While a video subscriber is attached, motion is ignored and quality stays
// fixed: the H.264 lane transcodes these same screencast frames now, so
// every motion/settle transition's Page.stopScreencast+startScreencast
// cycle (below, in ensureCast) would interrupt its frame supply too — and
// real touch input (discrete swipes with pauses between them, unlike a
// perfectly continuous synthetic scroll) hits that transition constantly
// (confirmed live: this was why the video feed barely updated during
// interaction). x264's own rate control already adapts bitrate to motion;
// the JPEG-bandwidth-saving reason for this quality switch doesn't apply
// when nobody's actually consuming the JPEG lane's bytes over the wire.
// b.mu held by caller.
func (b *Browser) desiredCastLocked(t *Tab) (q, maxW, maxH int) {
	if b.hasVideoSubscribers() {
		// H.264 is already the wire-quality boundary in video mode. Feeding
		// its encoder q=100 JPEGs made Chromium produce and FFmpeg decode
		// roughly half-megabyte frames 30 times per second, consuming CPU
		// needed for browser/tab responsiveness without visible benefit.
		// Keep one stable q=85 cast for the whole subscription: no expensive
		// motion/settle restarts, and far less intermediate JPEG work.
		return clampQuality(b.cfg.MotionQuality, 85), b.viewW, b.viewH
	}
	if t.motion {
		return clampQuality(b.cfg.MotionQuality, 85), b.viewW, b.viewH
	}
	return clampQuality(b.cfg.Quality, 100), b.viewW, b.viewH
}

// ensureCast converges the running screencast onto the desired parameters.
// One worker per tab; safe to call from anywhere, never blocks the caller.
func (b *Browser) ensureCast(t *Tab) {
	b.mu.Lock()
	if t.recasting {
		b.mu.Unlock()
		return
	}
	t.recasting = true
	b.mu.Unlock()

	go func() {
		defer func() {
			b.mu.Lock()
			t.recasting = false
			b.mu.Unlock()
		}()
		for i := 0; i < 6; i++ { // bounded: motion state flapping can't spin us forever
			// Nobody consuming JPEG or H.264 (the latter is transcoded from
			// these same screencast frames, so it depends on the cast too):
			// park it and stop spending CPU on it. ClientConnected /
			// video-off call ensureCast again to bring it back.
			if !b.hub.HasCastClient() && !b.hasVideoSubscribers() {
				b.mu.Lock()
				casting := t.casting
				b.mu.Unlock()
				if casting {
					b.stopCast(t)
					log.Printf("cast tab %d parked: no consumers", t.ID)
				}
				return
			}
			b.mu.Lock()
			q, maxW, maxH := b.desiredCastLocked(t)
			done := t.casting && t.castQuality == q && t.castMaxW == maxW
			active := t.ID == b.activeID
			b.mu.Unlock()
			if done || !active {
				return
			}
			b.stopCast(t)
			if !b.startCast(t, q, maxW, maxH) {
				return
			}
		}
	}()
}

func (b *Browser) startCast(t *Tab, q, maxW, maxH int) bool {
	b.mu.Lock()
	s := t.Session
	b.mu.Unlock()
	opts := map[string]any{
		"format": "jpeg", "quality": q,
		"maxWidth": maxW, "maxHeight": maxH, "everyNthFrame": 1,
	}
	var lastErr error
	for i := 0; i < 15; i++ {
		if _, err := b.cdp.Call(s, "Page.startScreencast", opts); err == nil {
			log.Printf("cast tab %d started q=%d max=%dx%d motion=%t", t.ID, q, maxW, maxH, t.motion)
			b.mu.Lock()
			t.casting = true
			t.castQuality = q
			t.castMaxW = maxW
			t.castMaxH = maxH
			b.mu.Unlock()
			return true
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	log.Printf("startCast tab %d failed after retries: %v", t.ID, lastErr)
	return false
}

func (b *Browser) stopCast(t *Tab) {
	b.mu.Lock()
	s := t.Session
	b.mu.Unlock()
	_, _ = b.cdp.Call(s, "Page.stopScreencast", nil)
	b.mu.Lock()
	t.casting = false
	b.mu.Unlock()
}

// beginMotion drops the cast to cheap low-res frames while the user is
// interacting; once input stops for SettleMS we go back to full resolution
// and push one sharp screenshot so the final view is crisp.
func (b *Browser) beginMotion(t *Tab) {
	b.mu.Lock()
	wasMotion := t.motion
	t.motion = true
	t.settleSeq++
	seq := t.settleSeq
	if t.settleTimer != nil {
		t.settleTimer.Stop()
	}
	t.settleTimer = time.AfterFunc(time.Duration(b.cfg.SettleMS)*time.Millisecond, func() {
		b.mu.Lock()
		if t.settleSeq != seq {
			b.mu.Unlock()
			return
		}
		t.motion = false
		b.mu.Unlock()
		b.ensureCast(t)
		b.sendSharpFrame(nil, t)
	})
	b.mu.Unlock()
	if !wasMotion {
		b.ensureCast(t)
	}
}

func (b *Browser) sendSharpFrame(only *ws.Client, t *Tab) {
	b.mu.Lock()
	if t == nil || t.ID != b.activeID {
		b.mu.Unlock()
		return
	}
	if time.Since(t.lastSharpAt) < 750*time.Millisecond {
		b.mu.Unlock()
		return
	}
	t.lastSharpAt = time.Now()
	b.mu.Unlock()
	b.sendFreshFrame(only, b.cfg.SharpQuality)
}

// sendFreshFrame captures a full-res screenshot of the active tab and queues
// it to one client (only != nil) or everyone, stamped with the current scroll
// offset. Guarantees the latest state is visible even when the page is static
// and the screencast is idle.
func (b *Browser) sendFreshFrame(only *ws.Client, quality int) {
	t := b.active()
	if t == nil {
		return
	}
	q := clampQuality(quality, b.cfg.Quality)
	b.mu.Lock()
	s := t.Session
	w, h := b.viewW, b.viewH
	z := t.Zoom
	b.mu.Unlock()
	if z < 1 {
		z = 1
	}

	// Scroll offset first (cheap), then the pixels; a tiny race between the
	// two is invisible next to frame latency.
	var sx, sy uint32
	if v, err := b.cdp.EvaluateString(s, "''+window.pageXOffset+','+window.pageYOffset"); err == nil {
		if i := strings.IndexByte(v, ','); i > 0 {
			fx, err1 := strconv.ParseFloat(v[:i], 64)
			fy, err2 := strconv.ParseFloat(v[i+1:], 64)
			if err1 == nil && err2 == nil {
				sx, sy = clampScroll(fx), clampScroll(fy)
			}
		}
	}

	started := time.Now()
	buf, err := b.cdp.CaptureJPEG(s, q)
	if err != nil {
		log.Printf("captureScreenshot tab %d failed after %.1fs: %v", t.ID, time.Since(started).Seconds(), err)
		return
	}
	if d := time.Since(started); d > 2*time.Second {
		log.Printf("captureScreenshot tab %d slow: %.1fs", t.ID, d.Seconds())
	}
	if !b.isActiveSession(s) {
		return
	}
	// Sharp marks this as the settle frame — the one JPEG that still reaches
	// video-mode clients (their crisp-text overlay).
	b.hub.QueueFrame(&protocol.Frame{W: w, H: h, ScrollX: sx, ScrollY: sy, Data: buf, Sharp: true}, only)
}

func clampScroll(v float64) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v + 0.5)
}
