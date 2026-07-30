package media

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"surf-backend/internal/telemetry"
)

const (
	subQueueCap      = 4
	videoIdleDelay   = 5 * time.Second
	keyframeCooldown = 2 * time.Second
)

type VideoPipelineConfig struct {
	W, H                 int
	CaptureW, CaptureH   int
	ScaleMaxW, ScaleMaxH int
	BitrateK             int
	Start                func(width, height, bitrateK int) error
	Stop                 func()
	Keyframe             func()
}

type AccessUnit struct {
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
	Profile          uint8
}

type FrameMetadata struct {
	InputReceiveNS, CDPAcceptedNS uint64
	Profile                       uint8
}

type VideoSubscription struct {
	C chan AccessUnit

	s       *VideoPipeline
	mu      sync.Mutex
	dropped bool
	closed  bool
	fresh   bool
	gen     int
}

type VideoPipeline struct {
	cfg VideoPipelineConfig

	mu              sync.Mutex
	subs            map[*VideoSubscription]struct{}
	running         bool
	gen             int
	seq             uint32
	stopTimer       *time.Timer
	lastKeyframeReq time.Time

	accessUnits atomic.Uint64
	fanoutDrops atomic.Uint64
}

func NewVideoPipeline(cfg VideoPipelineConfig) *VideoPipeline {
	if cfg.CaptureW == 0 || cfg.CaptureH == 0 {
		cfg.CaptureW, cfg.CaptureH = cfg.W, cfg.H
	}
	cfg.W, cfg.H = cfg.codedSize(cfg.CaptureW, cfg.CaptureH)
	return &VideoPipeline{cfg: cfg, subs: map[*VideoSubscription]struct{}{}}
}

func (s *VideoPipeline) Config() VideoPipelineConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *VideoPipeline) Generation() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return uint32(s.gen)
}

func (s *VideoPipeline) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *VideoPipeline) Stats() map[string]uint64 {
	return map[string]uint64{
		"video_access_units": s.accessUnits.Load(),
		"video_fanout_drops": s.fanoutDrops.Load(),
	}
}

// Restart is the explicit recovery boundary used by the protocol. Existing
// subscribers stay attached but wait for an IDR from the new generation.
// With no subscribers it leaves the encoder stopped; the next Subscribe
// starts it normally.
func (s *VideoPipeline) Restart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopTimer != nil {
		s.stopTimer.Stop()
		s.stopTimer = nil
	}
	s.stopLocked()
	if len(s.subs) > 0 {
		s.startLocked()
	}
	s.resetSubsForGenLocked()
}

func (s *VideoPipeline) Subscribe() *VideoSubscription {
	sub := &VideoSubscription{C: make(chan AccessUnit, subQueueCap), s: s, fresh: true, dropped: true}
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

func (sub *VideoSubscription) Close() {
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
		s.stopTimer = time.AfterFunc(videoIdleDelay, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if len(s.subs) == 0 && s.running {
				log.Printf("stream: idle, stopping tab encoder")
				s.stopLocked()
			}
		})
	}
}

func (sub *VideoSubscription) ForceResync() {
	sub.mu.Lock()
	sub.dropped = true
	sub.mu.Unlock()
}

func (sub *VideoSubscription) resetForGen(gen int) {
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

func (sub *VideoSubscription) offer(au AccessUnit, gen int) {
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

func (s *VideoPipeline) Push(data []byte, idr bool, width, height int, sourceSeq uint32, interactionID uint64, metadata FrameMetadata) bool {
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
	au := AccessUnit{
		Data: data, IDR: idr, W: width, H: height, T: time.Now(),
		SourceSeq: sourceSeq, InteractionID: interactionID,
		SourceReceiveNS: nowNS, EncodeCompleteNS: nowNS,
		InputReceiveNS: metadata.InputReceiveNS, CDPAcceptedNS: metadata.CDPAcceptedNS,
		Profile: metadata.Profile,
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
	if err := s.cfg.Start(s.cfg.W, s.cfg.H, s.cfg.BitrateK); err != nil {
		s.running = false
		log.Printf("stream: tab encoder start failed: %v", err)
		s.failAllLocked()
		return
	}
	log.Printf("stream: tab encoder started generation=%d %dx%d source-paced", gen, s.cfg.W, s.cfg.H)
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

func (s *VideoPipeline) broadcast(au AccessUnit, gen int) {
	s.mu.Lock()
	if gen != s.gen || !s.running {
		s.mu.Unlock()
		return
	}
	s.seq++
	au.Seq = s.seq
	au.Generation = uint32(gen)
	targets := make([]*VideoSubscription, 0, len(s.subs))
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

func (cfg VideoPipelineConfig) codedSize(width, height int) (int, int) {
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
