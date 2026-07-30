package media

import (
	"bytes"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubBackpressureResyncsOnIDR(t *testing.T) {
	s := NewVideoPipeline(VideoPipelineConfig{W: 64, H: 64})
	sub := &VideoSubscription{C: make(chan AccessUnit, 2), s: s, fresh: true, dropped: true, gen: 1}

	sub.offer(AccessUnit{Seq: 1}, 1)
	if len(sub.C) != 0 {
		t.Fatal("fresh sub accepted a P-frame")
	}
	sub.offer(AccessUnit{Seq: 2, IDR: true}, 1)
	sub.offer(AccessUnit{Seq: 3}, 1)
	sub.offer(AccessUnit{Seq: 4}, 1)
	<-sub.C
	<-sub.C
	sub.offer(AccessUnit{Seq: 5}, 1)
	if len(sub.C) != 0 {
		t.Fatal("P-frame delivered while waiting for IDR")
	}
	sub.offer(AccessUnit{Seq: 6, IDR: true}, 1)
	if got := <-sub.C; got.Seq != 6 || !got.IDR {
		t.Fatalf("resync frame = %+v", got)
	}
}

func TestTabEncoderFeedsFanout(t *testing.T) {
	var starts, stops, keyframes atomic.Int64
	s := NewVideoPipeline(VideoPipelineConfig{
		W: 768, H: 950, BitrateK: 6000,
		Start: func(width, height, bitrateK int) error {
			if width != 768 || height != 950 || bitrateK != 6000 {
				t.Fatalf("start config = %dx%d %dk", width, height, bitrateK)
			}
			starts.Add(1)
			return nil
		},
		Stop:     func() { stops.Add(1) },
		Keyframe: func() { keyframes.Add(1) },
	})
	sub := s.Subscribe()
	data := []byte{0, 0, 0, 1, 0x65, 1, 2, 3}
	if !s.Push(data, true, 768, 950, 7, 9, FrameMetadata{Profile: 2}) {
		t.Fatal("access unit was rejected")
	}
	select {
	case au := <-sub.C:
		if !au.IDR || au.W != 768 || au.H != 950 || au.SourceSeq != 7 ||
			au.InteractionID != 9 || au.Profile != 2 || !bytes.Equal(au.Data, data) {
			t.Fatalf("access unit = %+v", au)
		}
	case <-time.After(time.Second):
		t.Fatal("access unit was not delivered")
	}
	s.mu.Lock()
	s.lastKeyframeReq = time.Time{}
	s.mu.Unlock()
	s.RequestKeyframe()
	if starts.Load() != 1 || keyframes.Load() != 1 {
		t.Fatalf("starts=%d keyframes=%d", starts.Load(), keyframes.Load())
	}
	s.Shutdown()
	if stops.Load() != 1 {
		t.Fatalf("stops=%d, want 1", stops.Load())
	}
}

func TestSubscribeFailsImmediatelyWhenEncoderCannotStart(t *testing.T) {
	s := NewVideoPipeline(VideoPipelineConfig{
		W: 64, H: 64,
		Start: func(int, int, int) error { return errors.New("unavailable") },
	})
	sub := s.Subscribe()
	select {
	case _, ok := <-sub.C:
		if ok {
			t.Fatal("expected closed channel")
		}
	default:
		t.Fatal("expected subscription to close immediately")
	}
}

func TestResizeRestartsAtClientDerivedSize(t *testing.T) {
	var starts [][2]int
	stops := 0
	s := NewVideoPipeline(VideoPipelineConfig{
		W: 768, H: 950,
		Start: func(width, height, bitrateK int) error {
			starts = append(starts, [2]int{width, height})
			return nil
		},
		Stop: func() { stops++ },
	})
	defer s.Shutdown()
	s.Subscribe()
	s.SetSize(1024, 768)
	if len(starts) != 2 || starts[1] != [2]int{1024, 768} {
		t.Fatalf("starts=%v", starts)
	}
	if stops != 1 {
		t.Fatalf("stops=%d, want 1 before shutdown", stops)
	}
}

func TestExplicitRestartPreservesSubscriberAndRequiresNewIDR(t *testing.T) {
	var starts, stops atomic.Int64
	s := NewVideoPipeline(VideoPipelineConfig{
		W: 64, H: 64,
		Start: func(int, int, int) error {
			starts.Add(1)
			return nil
		},
		Stop: func() { stops.Add(1) },
	})
	defer s.Shutdown()
	sub := s.Subscribe()
	if !s.Running() {
		t.Fatal("pipeline did not start")
	}
	if !s.Push([]byte{1}, true, 64, 64, 1, 0, FrameMetadata{}) {
		t.Fatal("initial IDR was rejected")
	}
	<-sub.C
	firstGeneration := s.Generation()

	s.Restart()
	if !s.Running() {
		t.Fatal("pipeline did not restart")
	}
	if starts.Load() != 2 || stops.Load() != 1 {
		t.Fatalf("starts=%d stops=%d, want 2/1", starts.Load(), stops.Load())
	}
	if s.Generation() == firstGeneration {
		t.Fatal("restart did not advance the encoder generation")
	}
	if !s.Push([]byte{2}, false, 64, 64, 2, 0, FrameMetadata{}) {
		t.Fatal("post-restart P-frame was rejected by the pipeline")
	}
	select {
	case au := <-sub.C:
		t.Fatalf("subscriber accepted P-frame before new IDR: %+v", au)
	default:
	}
	if !s.Push([]byte{3}, true, 64, 64, 3, 0, FrameMetadata{}) {
		t.Fatal("post-restart IDR was rejected")
	}
	if au := <-sub.C; !au.IDR || au.SourceSeq != 3 || au.Generation != s.Generation() {
		t.Fatalf("restart IDR = %+v", au)
	}
}

func TestScaleLimitPreservesAspectRatioAndEvenDimensions(t *testing.T) {
	s := NewVideoPipeline(VideoPipelineConfig{W: 1023, H: 767, CaptureW: 1023, CaptureH: 767, ScaleMaxW: 768, ScaleMaxH: 768})
	cfg := s.Config()
	if cfg.W != 768 || cfg.H != 576 {
		t.Fatalf("coded size=%dx%d, want 768x576", cfg.W, cfg.H)
	}
}
