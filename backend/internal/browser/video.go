package browser

import (
	"log"
	"strings"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
	"surf-backend/internal/stream"
	"surf-backend/internal/ws"
)

type streamProfile struct {
	fps                      int
	maxW, maxH               int
	bitrateK, maxrateK, bufK int
	preset                   string
	label                    string
}

var streamProfiles = map[string]streamProfile{
	"sharp":    {fps: 30, maxW: 1024, maxH: 1024, bitrateK: 6000, maxrateK: 8000, bufK: 1800, preset: "ultrafast", label: "Sharp 30"},
	"smooth":   {fps: 60, maxW: 800, maxH: 800, bitrateK: 6000, maxrateK: 8000, bufK: 1200, preset: "ultrafast", label: "Smooth 60"},
	"balanced": {fps: 30, maxW: 800, maxH: 800, bitrateK: 3200, maxrateK: 4500, bufK: 900, preset: "ultrafast", label: "Balanced 30"},
	"fast":     {fps: 45, maxW: 720, maxH: 720, bitrateK: 4500, maxrateK: 6500, bufK: 900, preset: "ultrafast", label: "Fast 45"},
	"potato":   {fps: 20, maxW: 640, maxH: 640, bitrateK: 1200, maxrateK: 1800, bufK: 500, preset: "ultrafast", label: "Low Data 20"},
	"max":      {fps: 60, maxW: 1024, maxH: 1024, bitrateK: 8000, maxrateK: 10000, bufK: 1800, preset: "ultrafast", label: "Max 60"},
}

func (b *Browser) handleStreamProfile(c *ws.Client, profile string) {
	if profile == "" {
		profile = "balanced"
	}
	p, ok := streamProfiles[profile]
	if !ok {
		c.SendJSON(map[string]any{"t": "toast", "text": "unknown stream profile"})
		return
	}
	b.streamer.Configure(p.fps, p.maxW, p.maxH, p.bitrateK, p.maxrateK, p.bufK, p.preset)
	c.SendJSON(map[string]any{"t": "toast", "text": "stream: " + p.label})
}

// firstAUWait bounds how long a fresh subscriber waits for its first
// (IDR) access unit: process spawn + one 2s IDR interval + margin.
const firstAUWait = 6 * time.Second

// handleVideo services {"t":"video","on":...}. Runs on the client's read
// goroutine; everything slow happens in the pump goroutine.
func (b *Browser) handleVideo(c *ws.Client, on bool) {
	if !on {
		b.stopVideo(c, true)
		return
	}
	b.mediaMu.Lock()
	if _, exists := b.videoSubs[c]; exists {
		b.mediaMu.Unlock()
		return
	}
	sub := b.streamer.Subscribe()
	b.videoSubs[c] = sub
	b.mediaMu.Unlock()
	log.Printf("video: client subscribed")
	go b.pumpVideo(c, sub)
}

// pumpVideo gates on encoder health, then relays AUs until the sub dies or
// the client leaves video mode.
func (b *Browser) pumpVideo(c *ws.Client, sub *stream.Sub) {
	cfg := b.streamer.Config()
	select {
	case au, ok := <-sub.C:
		if !ok {
			b.videoFailed(c)
			return
		}
		c.SetVideoMode(true)
		c.SendJSON(map[string]any{"t": "video-config", "ok": true, "fps": cfg.FPS, "w": au.W, "h": au.H})
		if t := b.active(); t != nil {
			b.ensureCast(t) // may stop the cast if nobody consumes JPEG now
		}
		b.deliverAU(c, sub, au)
	case <-time.After(firstAUWait):
		log.Printf("video: no AU within %s, lane unavailable", firstAUWait)
		b.videoFailed(c)
		return
	}

	for au := range sub.C {
		b.deliverAU(c, sub, au)
	}
	// Channel closed: encoder gave up (or we unsubscribed). Tell the client
	// to fall back; stopVideo is idempotent.
	c.SendJSON(map[string]any{"t": "video-config", "ok": false})
	b.stopVideo(c, true)
}

func (b *Browser) deliverAU(c *ws.Client, sub *stream.Sub, au stream.AU) {
	if !au.T.IsZero() {
		b.noteVideoLatency(time.Since(au.T))
	}
	if err := c.SendBinary(protocol.EncodeVideoAU(au.Seq, au.IDR, au.W, au.H, au.Data)); err != nil {
		// Outbox dropped the AU — every P-frame after it is garbage, so make
		// the subscription resume at the next IDR.
		sub.ForceResync()
	}
}

func (b *Browser) videoFailed(c *ws.Client) {
	c.SendJSON(map[string]any{"t": "video-config", "ok": false})
	b.stopVideo(c, false)
}

// stopVideo tears down the client's subscription. restoreCast also brings the
// JPEG lane back and pushes a fresh frame so the screen never goes stale.
func (b *Browser) stopVideo(c *ws.Client, restoreCast bool) {
	b.mediaMu.Lock()
	sub := b.videoSubs[c]
	delete(b.videoSubs, c)
	b.mediaMu.Unlock()
	if sub == nil {
		return
	}
	sub.Close()
	c.SetVideoMode(false)
	log.Printf("video: client unsubscribed")
	if restoreCast {
		if t := b.active(); t != nil {
			b.ensureCast(t)
		}
		go b.sendFreshFrame(c, b.cfg.SharpQuality)
	}
}

// ClientDisconnected implements ws.Handler: release the video subscription
// (idle-stop then kills the encoder) and let the cast converge.
func (b *Browser) ClientDisconnected(c *ws.Client) {
	b.mediaMu.Lock()
	sub := b.videoSubs[c]
	delete(b.videoSubs, c)
	audioSub := b.audioSubs[c]
	delete(b.audioSubs, c)
	b.mediaMu.Unlock()
	if sub != nil {
		sub.Close()
		log.Printf("video: client disconnected, unsubscribed")
	}
	if audioSub != nil {
		audioSub.Close()
		log.Printf("audio: client disconnected, unsubscribed")
	}
	if t := b.active(); t != nil {
		b.ensureCast(t)
	}
}

// streamConfig derives the encoder config from the server config.
func streamConfig(cfg *config.Config) stream.Config {
	maxW, maxH := 0, 0
	if s := cfg.StreamScale; s != "" {
		var sw, sh int
		if i := strings.IndexByte(s, 'x'); i > 0 {
			sw = atoiSafe(s[:i])
			sh = atoiSafe(s[i+1:])
		}
		if sw >= 64 && sh >= 64 {
			maxW, maxH = sw, sh
		} else {
			log.Printf("stream: ignoring bad STREAM_SCALE %q", s)
		}
	}
	return stream.Config{
		W: cfg.ViewW, H: cfg.ViewH,
		CaptureW: cfg.ViewW, CaptureH: cfg.ViewH,
		ScaleMaxW: maxW, ScaleMaxH: maxH,
		FPS:      cfg.StreamFPS,
		BitrateK: cfg.StreamBitrateK, MaxrateK: cfg.StreamMaxrateK, BufsizeK: cfg.StreamBufsizeK,
		Preset: cfg.StreamPreset,
	}
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
