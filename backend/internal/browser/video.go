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

// firstAUWait bounds how long a fresh subscriber waits for its first
// (IDR) access unit: process spawn + one capture round-trip + margin.
const firstAUWait = 6 * time.Second

// hasVideoSubscribers reports whether any client currently has the H.264
// lane subscribed. The internal CDP screencast exists only to feed it.
func (b *Controller) hasVideoSubscribers() bool {
	b.mediaMu.Lock()
	defer b.mediaMu.Unlock()
	return len(b.videoSubs) > 0
}

func (b *Controller) subscribeVideo(c *ws.ClientTransport) {
	b.mu.Lock()
	viewW, viewH := b.viewW, b.viewH
	b.mu.Unlock()
	b.mediaMu.Lock()
	if _, exists := b.videoSubs[c]; exists {
		b.mediaMu.Unlock()
		return
	}
	// The client viewport is known before automatic subscription. Configure
	// the dormant encoder first so it never starts at the backend default and
	// immediately restarts at the iPad size.
	b.video.SetSize(viewW, viewH)
	sub := b.video.Subscribe()
	b.videoSubs[c] = sub
	b.mediaMu.Unlock()
	cfg := b.video.Config()
	c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "starting", FPS: cfg.FPS, Generation: b.video.Generation()})
	log.Printf("video: client subscribed")
	if t := b.active(); t != nil {
		b.ensureCast(t) // converge the shared source to stable video quality
	}
	go b.pumpVideo(c, sub)
	go b.bootstrapVideoFrame()
}

// bootstrapVideoFrame pushes one JPEG into the encoder right after a
// subscribe. CDP's screencast only emits frames when the compositor
// actually produces new content, so a subscriber landing on an already-
// static page would otherwise wait out firstAUWait for a frame that may
// never arrive on its own.
func (b *Controller) bootstrapVideoFrame() {
	t := b.active()
	if t == nil {
		return
	}
	b.mu.Lock()
	s := t.Session
	vw, vh := b.viewW, b.viewH
	b.mu.Unlock()
	buf, err := b.cdp.CaptureJPEG(s, clampQuality(b.cfg.SourceJPEGQuality, 100))
	if err != nil {
		return
	}
	b.mu.Lock()
	stillCurrent := t.ID == b.activeID && s == t.Session && vw == b.viewW && vh == b.viewH
	b.mu.Unlock()
	if !stillCurrent || !b.hasVideoSubscribers() {
		return
	}
	if w, h, ok := jpegSize(buf); ok {
		if w != vw || h != vh {
			// Page.captureScreenshot can race the device-metrics update and
			// return the previous viewport. Never let a bootstrap image
			// override the client-authoritative encoder size.
			return
		}
		current := b.video.Config()
		if current.CaptureW == w && current.CaptureH == h {
			b.video.PushBootstrap(buf)
			return
		}
		b.requestVideoResizeMode(w, h, buf, s, true)
		return
	}
	b.video.PushBootstrap(buf)
}

// pumpVideo gates on encoder health, then relays AUs until the sub dies or
// the client disconnects or explicitly retries.
func (b *Controller) pumpVideo(c *ws.ClientTransport, sub *stream.Sub) {
	cfg := b.video.Config()
	select {
	case au, ok := <-sub.C:
		if !ok {
			b.videoFailed(c)
			return
		}
		c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "ready", FPS: cfg.FPS, W: au.W, H: au.H, Generation: au.Generation})
		if t := b.active(); t != nil {
			b.ensureCast(t)
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
	// Channel closed: encoder gave up (or we unsubscribed).
	c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "unavailable", Reason: "encoder-stopped", FPS: cfg.FPS})
	b.stopVideo(c)
}

func (b *Controller) deliverAU(c *ws.ClientTransport, sub *stream.Sub, au stream.AU) {
	b.noteEncodeLatency(au.SourceReceiveNS, au.EncodeCompleteNS)
	if !au.T.IsZero() {
		b.noteVideoLatency(time.Since(au.T))
	}
	meta := protocol.VideoMeta{
		AUSeq: au.Seq, SourceSeq: au.SourceSeq, W: au.W, H: au.H,
		InteractionID:     au.InteractionID,
		SourceReceiveNS:   au.SourceReceiveNS,
		EncodeCompleteNS:  au.EncodeCompleteNS,
		EncoderGeneration: au.Generation,
	}
	if err := c.SendBinary(protocol.EncodeVideo(meta, au.IDR, au.Data)); err != nil {
		// Outbox dropped the AU — every P-frame after it is garbage, so make
		// the subscription resume at an immediate IDR. Waiting for the normal
		// two-second GOP boundary is the visible freeze this recovery path is
		// specifically meant to avoid; VideoPipeline applies its own cooldown.
		sub.ForceResync()
		go b.video.RequestKeyframe()
	}
}

func (b *Controller) videoFailed(c *ws.ClientTransport) {
	c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "unavailable", Reason: "encoder-start-timeout"})
	b.stopVideo(c)
}

// stopVideo tears down the client's subscription. The capture source parks
// automatically after the final video consumer leaves.
func (b *Controller) stopVideo(c *ws.ClientTransport) {
	b.mediaMu.Lock()
	sub := b.videoSubs[c]
	delete(b.videoSubs, c)
	b.mediaMu.Unlock()
	if sub == nil {
		return
	}
	sub.Close()
	log.Printf("video: client unsubscribed")
}

// ClientDisconnected implements ws.Handler: release the video subscription
// (idle-stop then kills the encoder) and let the cast converge.
func (b *Controller) ClientDisconnected(c *ws.ClientTransport) {
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
		FFmpegPath: cfg.FFmpegPath, Env: cfg.ChildEnv,
		W: cfg.ViewW, H: cfg.ViewH,
		CaptureW: cfg.ViewW, CaptureH: cfg.ViewH,
		ScaleMaxW: maxW, ScaleMaxH: maxH,
		FPS: cfg.StreamFPS, Encoder: cfg.StreamEncoder,
		BitrateK: cfg.StreamBitrateK, MaxrateK: cfg.StreamMaxrateK, BufsizeK: cfg.StreamBufsizeK,
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
