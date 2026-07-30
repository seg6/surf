package browser

import (
	"testing"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/media"
	"surf-backend/internal/protocol"
)

func TestAdaptiveProfileHysteresis(t *testing.T) {
	cfg := &config.Config{AdaptiveVideo: true}
	b := &Controller{
		cfg: cfg,
		video: media.NewVideoPipeline(media.VideoPipelineConfig{
			W: 768, H: 934, CaptureW: 768, CaptureH: 934, FPS: 30,
		}),
		tabs: map[int]*Tab{},
	}
	b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 27, AURate: 30})
	if b.adaptiveProfile != 0 {
		t.Fatal("profile degraded after only one unhealthy report")
	}
	b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 27, AURate: 30})
	if b.adaptiveProfile != 1 {
		t.Fatalf("unhealthy report did not degrade: profile=%d", b.adaptiveProfile)
	}
	b.adaptiveLastChange = time.Now().Add(-11 * time.Second)
	for i := 0; i < 5; i++ {
		b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 30, AURate: 30, CallbackMS: 12, GapMS: 34})
	}
	if b.adaptiveProfile != 1 {
		t.Fatal("profile promoted before six healthy reports")
	}
	b.handleMediaStats(&protocol.MediaStatsCommand{PresentedFPS: 30, AURate: 30, CallbackMS: 12, GapMS: 34})
	if b.adaptiveProfile != 0 {
		t.Fatalf("healthy reports did not promote: profile=%d", b.adaptiveProfile)
	}
}

func TestAdaptiveIgnoresStaticPageCadence(t *testing.T) {
	cfg := &config.Config{AdaptiveVideo: true}
	b := &Controller{
		cfg: cfg,
		video: media.NewVideoPipeline(media.VideoPipelineConfig{
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
