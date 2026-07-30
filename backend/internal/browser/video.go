package browser

import (
	"log"
	"strings"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
	"surf-backend/internal/stream"
	"surf-backend/internal/tabcapture"
	"surf-backend/internal/telemetry"
	"surf-backend/internal/ws"
)

// firstAUWait bounds how long a fresh subscriber waits for its first
// (IDR) access unit: process spawn + one capture round-trip + margin.
const firstAUWait = 6 * time.Second

// hasVideoSubscribers reports whether any client currently has the H.264
// lane subscribed.
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
	b.mediaMu.Unlock()
	// The client viewport is known before automatic subscription. Configure
	// the dormant encoder first so it never starts at the backend default and
	// immediately restarts at the iPad size.
	b.video.SetSize(viewW, viewH)
	cfg := b.video.Config()
	c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "starting", FPS: cfg.FPS, Generation: b.video.Generation(), Profile: b.profileName()})
	sub := b.video.Subscribe()
	select {
	case <-c.Closed():
		sub.Close()
		return
	default:
	}
	b.mediaMu.Lock()
	if _, exists := b.videoSubs[c]; exists {
		b.mediaMu.Unlock()
		sub.Close()
		return
	}
	b.videoSubs[c] = sub
	b.mediaMu.Unlock()
	log.Printf("video: client subscribed")
	if t := b.active(); t != nil {
		b.ensureCast(t) // converge the shared source to stable video quality
	}
	go b.pumpVideo(c, sub)
}

// onTabCaptureFrame accepts an already encoded H.264 access unit from
// tabCapture/WebCodecs. Browser control and interaction metadata still come
// from CDP.
func (b *Controller) onTabCaptureFrame(frame tabcapture.VideoFrame) {
	t := b.active()
	if t == nil || b.hub.ClientCount() == 0 || !b.hasVideoSubscribers() {
		return
	}
	b.noteSourceFrame()
	telemetry.Emit("source_frame", "capture", "tab", nil)

	b.perfMu.Lock()
	b.sourceSeq++
	sourceSeq, interactionID := b.sourceSeq, b.interactionID
	inputNS, cdpNS := b.interactionInputNS, b.interactionCDPNS
	b.perfMu.Unlock()
	if !b.video.PushEncodedMeta(frame.Data, frame.Key, frame.Width, frame.Height,
		sourceSeq, interactionID, stream.SourceMetadata{
			InputReceiveNS: inputNS, CDPAcceptedNS: cdpNS,
			Profile: uint8(b.profileIndex()),
		}) {
		return
	}

	b.mu.Lock()
	pageReady := t.awaitingPageFrame && b.tabs[t.ID] == t && b.activeID == t.ID
	if pageReady {
		t.awaitingPageFrame = false
	}
	b.mu.Unlock()
	if pageReady {
		b.hub.BroadcastJSON(protocol.PageFrameEvent{Type: "pageframe", SourceSeq: sourceSeq})
	}
}

func tabCaptureCodec(width, height, fps int) string {
	if fps > 30 || ((width+15)/16)*((height+15)/16)*fps > 108000 {
		return "avc1.42E029" // constrained baseline, level 4.1
	}
	return "avc1.42E01F" // constrained baseline, level 3.1
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
		c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "ready", FPS: cfg.FPS, W: au.W, H: au.H, Generation: au.Generation, Profile: b.profileName()})
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
		InputReceiveNS:    au.InputReceiveNS,
		CDPAcceptedNS:     au.CDPAcceptedNS,
		ScrollX:           au.ScrollX, ScrollY: au.ScrollY, PageScale: au.PageScale,
		Profile: au.Profile,
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
		W: cfg.ViewW, H: cfg.ViewH,
		CaptureW: cfg.ViewW, CaptureH: cfg.ViewH,
		ScaleMaxW: maxW, ScaleMaxH: maxH,
		FPS:      cfg.StreamFPS,
		BitrateK: cfg.StreamBitrateK,
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
