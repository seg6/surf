// Package stream owns the H.264 lane between Chromium's tabCapture/WebCodecs
// encoder and the native clients. It tracks encoder generations and provides
// per-subscriber, IDR-aware bounded fan-out.
package stream

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"surf-backend/internal/telemetry"
)

const (
	subQueueCap      = 4
	lingerStop       = 5 * time.Second
	keyframeCooldown = 2 * time.Second
)

type Config struct {
	W, H                 int
	CaptureW, CaptureH   int
	ScaleMaxW, ScaleMaxH int
	FPS                  int
	BitrateK             int
	Start                func(width, height, fps, bitrateK int) error
	Stop                 func()
	Keyframe             func()
}

type AU struct {
	Data             []byte
	IDR              bool
	Seq              uint32
	W, H             int
	T                time.Time
	Generation       uint32
	SourceSeq        uint32
	InteractionID    uint64
	SourceReceiveNS  uint64
	EncodeCompleteNS uint64
	InputReceiveNS   uint64
	CDPAcceptedNS    uint64
	ScrollX, ScrollY float64
	PageScale        float64
	Profile          uint8
}

type SourceMetadata struct {
	InputReceiveNS, CDPAcceptedNS uint64
	ScrollX, ScrollY, PageScale   float64
	Profile                       uint8
}

type Sub struct {
	C chan AU

	s       *VideoPipeline
	mu      sync.Mutex
	dropped bool
	closed  bool
	fresh   bool
	gen     int
}

type VideoPipeline struct {
	cfg Config

	mu              sync.Mutex
	subs            map[*Sub]struct{}
	running         bool
	gen             int
	seq             uint32
	stopTimer       *time.Timer
	lastKeyframeReq time.Time

	accessUnits atomic.Uint64
	fanoutDrops atomic.Uint64
}

func New(cfg Config) *VideoPipeline {
	if cfg.CaptureW == 0 || cfg.CaptureH == 0 {
		cfg.CaptureW, cfg.CaptureH = cfg.W, cfg.H
	}
	if cfg.FPS < 1 {
		cfg.FPS = 30
	}
	cfg.W, cfg.H = cfg.codedSize(cfg.CaptureW, cfg.CaptureH)
	return &VideoPipeline{cfg: cfg, subs: map[*Sub]struct{}{}}
}

func (s *VideoPipeline) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *VideoPipeline) Generation() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return uint32(s.gen)
}

func (s *VideoPipeline) Stats() map[string]uint64 {
	return map[string]uint64{
		"video_access_units": s.accessUnits.Load(),
		"video_fanout_drops": s.fanoutDrops.Load(),
	}
}

// ResetCrashBudget remains the explicit retry boundary used by the protocol.
// WebCodecs startup failures are retried by creating a new subscription, so
// there is no subprocess crash budget to clear.
func (s *VideoPipeline) ResetCrashBudget() {}

func (s *VideoPipeline) Subscribe() *Sub {
	sub := &Sub{C: make(chan AU, subQueueCap), s: s, fresh: true, dropped: true}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs[sub] = struct{}{}
	if s.stopTimer != nil {
		s.stopTimer.Stop()
		s.stopTimer = nil
	}
	if !s.running {
		s.startLocked()
	} else if time.Since(s.lastKeyframeReq) >= keyframeCooldown {
		s.lastKeyframeReq = time.Now()
		if s.cfg.Keyframe != nil {
			s.cfg.Keyframe()
		}
		s.resetSubsForGenLocked()
	}
	if _, exists := s.subs[sub]; exists {
		sub.resetForGen(s.gen)
	}
	return sub
}

func (sub *Sub) Close() {
	s := sub.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub]; !ok {
		return
	}
	delete(s.subs, sub)
	sub.mu.Lock()
	if !sub.closed {
		sub.closed = true
		close(sub.C)
	}
	sub.mu.Unlock()
	if len(s.subs) == 0 && s.running {
		s.stopTimer = time.AfterFunc(lingerStop, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if len(s.subs) == 0 && s.running {
				log.Printf("stream: idle, stopping tab encoder")
				s.stopLocked()
			}
		})
	}
}

func (sub *Sub) ForceResync() {
	sub.mu.Lock()
	sub.dropped = true
	sub.mu.Unlock()
}

func (sub *Sub) resetForGen(gen int) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	sub.gen = gen
	sub.dropped = true
	sub.fresh = true
	for {
		select {
		case <-sub.C:
		default:
			return
		}
	}
}

func (sub *Sub) offer(au AU, gen int) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed || gen != sub.gen {
		return
	}
	if sub.dropped && !au.IDR {
		return
	}
	select {
	case sub.C <- au:
		if sub.dropped && !sub.fresh {
			log.Printf("stream: sub resynced at seq=%d", au.Seq)
		}
		sub.dropped = false
		sub.fresh = false
	default:
		if !sub.dropped {
			log.Printf("stream: sub lagging, dropping to next IDR (seq=%d)", au.Seq)
		}
		sub.dropped = true
		sub.s.fanoutDrops.Add(1)
	}
}

func (s *VideoPipeline) PushEncodedMeta(data []byte, idr bool, width, height int, sourceSeq uint32, interactionID uint64, metadata SourceMetadata) bool {
	if len(data) == 0 || width < 2 || height < 2 {
		return false
	}
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return false
	}
	gen := s.gen
	s.mu.Unlock()

	nowNS := telemetry.MonoNS()
	s.accessUnits.Add(1)
	au := AU{
		Data: data, IDR: idr, W: width, H: height, T: time.Now(),
		SourceSeq: sourceSeq, InteractionID: interactionID,
		SourceReceiveNS: nowNS, EncodeCompleteNS: nowNS,
		InputReceiveNS: metadata.InputReceiveNS, CDPAcceptedNS: metadata.CDPAcceptedNS,
		ScrollX: metadata.ScrollX, ScrollY: metadata.ScrollY,
		PageScale: metadata.PageScale, Profile: metadata.Profile,
	}
	telemetry.Emit("encoded_au", "video", "webcodecs", map[string]any{"bytes": len(data), "idr": idr})
	s.broadcast(au, gen)
	return true
}

func (s *VideoPipeline) SetSize(width, height int) {
	if width < 64 || height < 64 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	codedW, codedH := s.cfg.codedSize(width, height)
	if width == s.cfg.CaptureW && height == s.cfg.CaptureH && codedW == s.cfg.W && codedH == s.cfg.H {
		return
	}
	s.cfg.CaptureW, s.cfg.CaptureH = width, height
	s.cfg.W, s.cfg.H = codedW, codedH
	if s.running {
		log.Printf("stream: resizing tab encoder capture=%dx%d coded=%dx%d", width, height, codedW, codedH)
		s.stopLocked()
		s.startLocked()
	}
	s.resetSubsForGenLocked()
}

func (s *VideoPipeline) SetScaleLimit(maxWidth, maxHeight int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.ScaleMaxW == maxWidth && s.cfg.ScaleMaxH == maxHeight {
		return
	}
	s.cfg.ScaleMaxW, s.cfg.ScaleMaxH = maxWidth, maxHeight
	width, height := s.cfg.codedSize(s.cfg.CaptureW, s.cfg.CaptureH)
	if width == s.cfg.W && height == s.cfg.H {
		return
	}
	s.cfg.W, s.cfg.H = width, height
	if s.running {
		log.Printf("stream: adaptive tab encoder size %dx%d", width, height)
		s.stopLocked()
		s.startLocked()
	}
	s.resetSubsForGenLocked()
}

func (s *VideoPipeline) RequestKeyframe() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	now := time.Now()
	if now.Sub(s.lastKeyframeReq) < keyframeCooldown {
		return
	}
	s.lastKeyframeReq = now
	log.Printf("stream: keyframe requested from tab encoder")
	if s.cfg.Keyframe != nil {
		s.cfg.Keyframe()
	}
	for sub := range s.subs {
		sub.mu.Lock()
		sub.dropped = true
		sub.mu.Unlock()
	}
}

func (s *VideoPipeline) startLocked() {
	if s.cfg.Start == nil {
		log.Printf("stream: tab encoder is unavailable")
		s.failAllLocked()
		return
	}
	s.gen++
	gen := s.gen
	s.running = true
	if err := s.cfg.Start(s.cfg.W, s.cfg.H, s.cfg.FPS, s.cfg.BitrateK); err != nil {
		s.running = false
		log.Printf("stream: tab encoder start failed: %v", err)
		s.failAllLocked()
		return
	}
	log.Printf("stream: tab encoder started generation=%d %dx%d@%d", gen, s.cfg.W, s.cfg.H, s.cfg.FPS)
}

func (s *VideoPipeline) stopLocked() {
	if !s.running {
		return
	}
	s.running = false
	s.gen++
	if s.cfg.Stop != nil {
		s.cfg.Stop()
	}
}

func (s *VideoPipeline) resetSubsForGenLocked() {
	for sub := range s.subs {
		sub.resetForGen(s.gen)
	}
}

func (s *VideoPipeline) failAllLocked() {
	for sub := range s.subs {
		delete(s.subs, sub)
		sub.mu.Lock()
		if !sub.closed {
			sub.closed = true
			close(sub.C)
		}
		sub.mu.Unlock()
	}
}

func (s *VideoPipeline) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopTimer != nil {
		s.stopTimer.Stop()
		s.stopTimer = nil
	}
	s.stopLocked()
	s.failAllLocked()
}

func (s *VideoPipeline) broadcast(au AU, gen int) {
	s.mu.Lock()
	if gen != s.gen || !s.running {
		s.mu.Unlock()
		return
	}
	s.seq++
	au.Seq = s.seq
	au.Generation = uint32(gen)
	targets := make([]*Sub, 0, len(s.subs))
	for sub := range s.subs {
		targets = append(targets, sub)
	}
	s.mu.Unlock()
	for _, sub := range targets {
		sub.offer(au, gen)
	}
}

func even(value int) int {
	if value < 2 {
		return 2
	}
	return value &^ 1
}

func (cfg Config) codedSize(width, height int) (int, int) {
	if cfg.ScaleMaxW < 64 || cfg.ScaleMaxH < 64 {
		return even(width), even(height)
	}
	scaleW := float64(cfg.ScaleMaxW) / float64(width)
	scaleH := float64(cfg.ScaleMaxH) / float64(height)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	if scale >= 1 {
		return even(width), even(height)
	}
	return even(int(float64(width)*scale + 0.5)), even(int(float64(height)*scale + 0.5))
}
