package browser

import (
	"testing"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
	"surf-backend/internal/stream"
)

func TestAdaptiveProfileHysteresis(t *testing.T) {
	cfg := &config.Config{SourceJPEGQuality: 100, AdaptiveVideo: true}
	b := &Controller{
		cfg: cfg,
		video: stream.New(stream.Config{
			W: 768, H: 934, CaptureW: 768, CaptureH: 934, FPS: 30,
		}),
		tabs: map[int]*Tab{},
	}
	b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 27, AURate: 30})
	if b.adaptiveProfile != 0 {
		t.Fatal("profile degraded after only one unhealthy report")
	}
	b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 27, AURate: 30})
	if b.adaptiveProfile != 1 || cfg.SourceJPEGQuality != 85 {
		t.Fatalf("unhealthy report did not degrade: profile=%d quality=%d", b.adaptiveProfile, cfg.SourceJPEGQuality)
	}
	b.adaptiveLastChange = time.Now().Add(-11 * time.Second)
	for i := 0; i < 5; i++ {
		b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 30, AURate: 30, CallbackMS: 12, GapMS: 34})
	}
	if b.adaptiveProfile != 1 {
		t.Fatal("profile promoted before six healthy reports")
	}
	b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 30, AURate: 30, CallbackMS: 12, GapMS: 34})
	if b.adaptiveProfile != 0 || cfg.SourceJPEGQuality != 100 {
		t.Fatalf("healthy reports did not promote: profile=%d quality=%d", b.adaptiveProfile, cfg.SourceJPEGQuality)
	}
}

func TestAdaptiveIgnoresStaticPageCadence(t *testing.T) {
	cfg := &config.Config{SourceJPEGQuality: 100, AdaptiveVideo: true}
	b := &Controller{
		cfg: cfg,
		video: stream.New(stream.Config{
			W: 768, H: 934, CaptureW: 768, CaptureH: 934, FPS: 30,
		}),
		tabs: map[int]*Tab{},
	}
	b.handleMediaStats(&protocol.MediaStatsCommand{
		PresentedFPS: 2, AURate: 2, CallbackMS: 40, GapMS: 5000,
	})
	if b.adaptiveProfile != 0 {
		t.Fatalf("static page degraded to profile %d", b.adaptiveProfile)
	}
}

func TestAdaptiveNeverRaisesConfiguredSourceQuality(t *testing.T) {
	cfg := &config.Config{SourceJPEGQuality: 70, AdaptiveVideo: true}
	b := &Controller{
		cfg:                 cfg,
		adaptiveBaseQuality: 70,
		video: stream.New(stream.Config{
			W: 768, H: 934, CaptureW: 768, CaptureH: 934, FPS: 30,
		}),
		tabs: map[int]*Tab{},
	}
	b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 20, AURate: 30})
	b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 20, AURate: 30})
	if cfg.SourceJPEGQuality != 70 {
		t.Fatalf("adaptive mode raised explicit source quality to %d", cfg.SourceJPEGQuality)
	}
}
