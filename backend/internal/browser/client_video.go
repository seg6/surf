package browser

import (
	"log"
	"strings"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/media"
	"surf-backend/internal/protocol"
	"surf-backend/internal/telemetry"
	"surf-backend/internal/transport"
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

func (b *Controller) subscribeVideo(c *transport.Client) {
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
	c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "starting", Generation: b.video.Generation(), Profile: b.profileName()})
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
	go b.pumpVideo(c, sub)
}

// onVideoFrame accepts an already encoded H.264 access unit from
// tabCapture/WebCodecs. Browser control and interaction metadata still come
// from CDP.
func (b *Controller) onVideoFrame(frame media.VideoFrame) {
	t := b.active()
	if t == nil || b.hub.ClientCount() == 0 || !b.hasVideoSubscribers() {
		return
	}
	b.noteEncodedAU(frame.Fresh)
	b.noteCaptureStages(frame)
	telemetry.Emit("encoded_au_received", "capture", "webcodecs", map[string]any{
		"fresh": frame.Fresh, "capture_sequence": frame.SourceSeq,
	})

	b.perfMu.Lock()
	if frame.Fresh || b.sourceSeq == 0 {
		b.sourceSeq++
		b.sourceInteraction = b.interactionID
		b.sourceInputNS = b.interactionInputNS
		b.sourceCDPNS = b.interactionCDPNS
	}
	sourceSeq, interactionID := b.sourceSeq, b.sourceInteraction
	inputNS, cdpNS := b.sourceInputNS, b.sourceCDPNS
	b.perfMu.Unlock()
	if !b.video.Push(frame.Data, frame.Key, frame.Width, frame.Height,
		sourceSeq, interactionID, media.FrameMetadata{
			InputReceiveNS: inputNS, CDPAcceptedNS: cdpNS,
			Profile: uint8(b.profileIndex()),
		}) {
		return
	}

	b.mu.Lock()
	pageReady := frame.Fresh && t.awaitingPageFrame &&
		b.tabs[t.ID] == t && b.activeID == t.ID
	if pageReady {
		t.awaitingPageFrame = false
	}
	b.mu.Unlock()
	if pageReady {
		b.hub.BroadcastJSON(protocol.PageFrameEvent{Type: "pageframe", SourceSeq: sourceSeq})
	}
}

func encoderCodec(width, height int) string {
	// Level selection is decoder compatibility metadata, not a frame-rate
	// limit. Prefer Level 3.1 for the original iPad whenever the coded size at
	// Surf's stable 30 FPS target fits its macroblock rate.
	const assumedSourceFPS = 30
	if ((width+15)/16)*((height+15)/16)*assumedSourceFPS > 108000 {
		return "avc1.42E029" // constrained baseline, level 4.1
	}
	return "avc1.42E01F" // constrained baseline, level 3.1
}

// pumpVideo gates on encoder health, then relays AUs until the sub dies or
// the client disconnects or explicitly retries.
func (b *Controller) pumpVideo(c *transport.Client, sub *media.VideoSubscription) {
	var generation uint32
	select {
	case au, ok := <-sub.C:
		if !ok {
			b.videoFailed(c)
			return
		}
		generation = au.Generation
		c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "ready", W: au.W, H: au.H, Generation: au.Generation, Profile: b.profileName()})
		b.deliverAU(c, sub, au)
	case <-time.After(firstAUWait):
		log.Printf("video: no AU within %s, lane unavailable", firstAUWait)
		b.videoFailed(c)
		return
	}

	for au := range sub.C {
		if au.Generation != generation {
			generation = au.Generation
			// A viewport/fullscreen change restarts WebCodecs without
			// replacing the subscription. Reconfigure VideoToolbox before the
			// new generation's first IDR, exactly like initial subscription.
			c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "ready", W: au.W, H: au.H, Generation: au.Generation, Profile: b.profileName()})
		}
		b.deliverAU(c, sub, au)
	}
	// Channel closed: encoder gave up (or we unsubscribed).
	c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "unavailable", Reason: "encoder-stopped"})
	b.stopVideo(c)
}

func (b *Controller) deliverAU(c *transport.Client, sub *media.VideoSubscription, au media.AccessUnit) {
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
		Profile:           au.Profile,
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

func (b *Controller) videoFailed(c *transport.Client) {
	c.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "unavailable", Reason: "encoder-start-timeout"})
	b.stopVideo(c)
}

// stopVideo tears down the client's subscription. The WebCodecs encoder parks
// automatically after the final video consumer leaves.
func (b *Controller) stopVideo(c *transport.Client) {
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

// ClientDisconnected implements transport.Handler and releases media subscriptions.
func (b *Controller) ClientDisconnected(c *transport.Client) {
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
}

// videoPipelineConfig derives the encoder config from the server config.
func videoPipelineConfig(cfg *config.Config) media.VideoPipelineConfig {
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
	return media.VideoPipelineConfig{
		W: cfg.ViewW, H: cfg.ViewH,
		CaptureW: cfg.ViewW, CaptureH: cfg.ViewH,
		ScaleMaxW: maxW, ScaleMaxH: maxH,
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
