package browser

import (
	"testing"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
)

func healthyMediaStats(rate float64) protocol.MediaStatsCommand {
	return protocol.MediaStatsCommand{
		AURate: rate, DecodeFPS: rate * 0.99, PresentedFPS: rate * 0.98,
		ImageFPS: rate * 0.98, CallbackMS: 8, GapMS: 20,
		WindowMS: 2000, QueueDepth: 1,
	}
}

func healthySystemRendererStats(rate float64) protocol.MediaStatsCommand {
	return protocol.MediaStatsCommand{
		AURate: rate, Renderer: "system", RendererFPS: rate * 0.99,
		ImageFPS: rate * 0.99, PresentedFPS: rate * 0.99, DecodeFPS: rate * 0.99,
		RendererMS: 0.2, WindowMS: 2000, QueueDepth: 0,
	}
}

func TestGovernorEmergencyDownshiftFollowsDecoderCollapse(t *testing.T) {
	g := newStreamGovernor()
	now := time.Unix(100, 0)
	collapse := protocol.MediaStatsCommand{
		AURate: 60, DecodeFPS: 5, PresentedFPS: 4, ImageFPS: 4,
		CallbackMS: 24, GapMS: 180, FrameAgeMS: 1100,
		WindowMS: 2000, DropPercent: 6, QueueDepth: 3,
	}
	decision := g.observe("ipad", now, collapse)
	if decision.changed || g.profile != 0 {
		t.Fatalf("warm-up collapse decision = %+v profile=%d", decision, g.profile)
	}
	decision = g.observe("ipad", now.Add(2*time.Second), collapse)
	if !decision.changed || decision.from != 0 || decision.to != 2 || g.profile != 2 {
		t.Fatalf("steady collapse decision = %+v profile=%d", decision, g.profile)
	}
	if decision = g.observe("ipad", now.Add(4*time.Second), collapse); decision.changed {
		t.Fatalf("changed during profile settle: %+v", decision)
	}
	decision = g.observe("ipad", now.Add(6*time.Second), collapse)
	if !decision.changed || decision.from != 2 || decision.to != 3 || g.profile != 3 {
		t.Fatalf("continued collapse decision = %+v profile=%d", decision, g.profile)
	}
}

func TestGovernorModeratePressureNeedsTwoSamples(t *testing.T) {
	g := newStreamGovernor()
	now := time.Unix(200, 0)
	pressure := healthyMediaStats(60)
	pressure.DropPercent = 2
	if decision := g.observe("ipad", now, pressure); decision.changed {
		t.Fatalf("warm-up sample changed profile: %+v", decision)
	}
	if decision := g.observe("ipad", now.Add(2*time.Second), pressure); decision.changed {
		t.Fatalf("one moderate control sample changed profile: %+v", decision)
	}
	decision := g.observe("ipad", now.Add(4*time.Second), pressure)
	if !decision.changed || decision.to != 1 {
		t.Fatalf("two moderate samples decision = %+v", decision)
	}
}

func TestGovernorRecoversSlowlyAndRestoresCrispWhenStatic(t *testing.T) {
	g := newStreamGovernor()
	g.profile = 2
	now := time.Unix(300, 0)
	g.lastChange = now
	if decision := g.observe("ipad", now.Add(time.Second), healthyMediaStats(30)); decision.changed {
		t.Fatalf("promoted without healthy hold: %+v", decision)
	}
	var decision governorDecision
	for second := 3; second <= 15; second += 2 {
		decision = g.observe("ipad", now.Add(time.Duration(second)*time.Second), healthyMediaStats(30))
		if decision.changed {
			break
		}
	}
	if !decision.changed || decision.to != 1 {
		t.Fatalf("healthy hold decision = %+v", decision)
	}
	static := protocol.MediaStatsCommand{AURate: 1, WindowMS: 2000}
	if decision = g.observe("ipad", now.Add(17*time.Second), static); decision.changed {
		t.Fatalf("settling static sample changed profile: %+v", decision)
	}
	g.observe("ipad", now.Add(19*time.Second), static)
	g.observe("ipad", now.Add(21*time.Second), static)
	decision = g.observe("ipad", now.Add(23*time.Second), static)
	if !decision.changed || decision.to != 0 {
		t.Fatalf("static recovery decision = %+v", decision)
	}
}

func TestGovernorDiscardsEvidenceDuringProfileSettle(t *testing.T) {
	g := newStreamGovernor()
	now := time.Unix(350, 0)
	g.observe("ipad", now, healthyMediaStats(60)) // initial warm-up
	collapse := protocol.MediaStatsCommand{
		AURate: 60, DecodeFPS: 4, PresentedFPS: 3, WindowMS: 2000,
		CallbackMS: 25, QueueDepth: 3,
	}
	decision := g.observe("ipad", now.Add(2*time.Second), collapse)
	if !decision.changed || decision.to != 2 {
		t.Fatalf("initial recovery decision = %+v", decision)
	}
	for _, offset := range []time.Duration{3 * time.Second, 5 * time.Second} {
		if decision = g.observe("ipad", now.Add(offset), collapse); decision.changed {
			t.Fatalf("mixed generation sample changed profile: %+v", decision)
		}
		state := g.clients["ipad"]
		if state.unhealthy != 0 || state.severe || state.hasPressure {
			t.Fatalf("settling evidence leaked into control state: %+v", state)
		}
	}
}

func TestGovernorDiscardsTabSwitchTransition(t *testing.T) {
	g := newStreamGovernor()
	now := time.Unix(375, 0)
	g.observe("ipad", now, healthyMediaStats(60))
	g.observe("ipad", now.Add(2*time.Second), healthyMediaStats(60))
	g.settleAfterDiscontinuity(now.Add(3 * time.Second))
	collapse := protocol.MediaStatsCommand{
		AURate: 60, DecodeFPS: 12, PresentedFPS: 11, WindowMS: 2000,
		CallbackMS: 24, DropPercent: 10, QueueDepth: 3,
	}
	for _, offset := range []time.Duration{4 * time.Second, 6 * time.Second} {
		if decision := g.observe("ipad", now.Add(offset), collapse); decision.changed {
			t.Fatalf("tab transition changed profile during settle: %+v", decision)
		}
	}
	// The first active sample after the timer can straddle the boundary.
	if decision := g.observe("ipad", now.Add(8*time.Second), collapse); decision.changed {
		t.Fatalf("first post-switch sample changed profile: %+v", decision)
	}
	if decision := g.observe("ipad", now.Add(10*time.Second), healthyMediaStats(60)); decision.changed {
		t.Fatalf("healthy post-switch sample changed profile: %+v", decision)
	}
}

func TestGovernorUsesWorstRecentClient(t *testing.T) {
	g := newStreamGovernor()
	now := time.Unix(400, 0)
	g.observe("modern", now, healthyMediaStats(60))
	collapse := protocol.MediaStatsCommand{
		AURate: 60, DecodeFPS: 4, PresentedFPS: 3, WindowMS: 2000,
		CallbackMS: 25, FrameAgeMS: 900, QueueDepth: 3,
	}
	g.observe("legacy", now, collapse)
	decision := g.observe("legacy", now.Add(2*time.Second), collapse)
	if !decision.changed || decision.to != 2 {
		t.Fatalf("worst-client decision = %+v", decision)
	}
}

func TestGovernorTreatsDecoderErrorAsEmergency(t *testing.T) {
	g := newStreamGovernor()
	stats := healthyMediaStats(60)
	stats.DecodeErrors = 1
	g.observe("ipad", time.Unix(450, 0), stats)
	decision := g.observe("ipad", time.Unix(452, 0), stats)
	if !decision.changed || decision.to != 2 || decision.reason != "decoder errors" {
		t.Fatalf("decoder-error decision = %+v", decision)
	}
}

func TestGovernorUsesSystemRendererSignalsWithoutDecoderCallbacks(t *testing.T) {
	g := newStreamGovernor()
	now := time.Unix(475, 0)
	healthy := healthySystemRendererStats(60)
	g.observe("ipad", now, healthy)
	if decision := g.observe("ipad", now.Add(2*time.Second), healthy); decision.changed {
		t.Fatalf("healthy system renderer changed profile: %+v", decision)
	}
	collapse := healthy
	collapse.RendererFPS = 18
	collapse.RendererRecoveries = 1
	decision := g.observe("ipad", now.Add(4*time.Second), collapse)
	if !decision.changed || decision.to != 2 || decision.reason != "renderer throughput" {
		t.Fatalf("system-renderer collapse decision = %+v", decision)
	}
}

func TestGovernorIgnoresStaticPageCadence(t *testing.T) {
	g := newStreamGovernor()
	for i := 0; i < 4; i++ {
		decision := g.observe("ipad", time.Unix(500+int64(i*2), 0), protocol.MediaStatsCommand{
			AURate: 2, PresentedFPS: 1, DecodeFPS: 1, CallbackMS: 40,
			GapMS: 5000, FrameAgeMS: 5000, WindowMS: 2000,
		})
		if decision.changed || g.profile != 0 {
			t.Fatalf("static sample degraded: decision=%+v profile=%d", decision, g.profile)
		}
	}
}

func TestGovernorDoesNotCarryOneBadSampleIntoHealthyDownshift(t *testing.T) {
	g := newStreamGovernor()
	now := time.Unix(600, 0)
	g.observe("ipad", now, healthyMediaStats(60)) // warm-up
	bad := healthyMediaStats(60)
	bad.GapMS = 700
	bad.PresentedFPS = 35
	if decision := g.observe("ipad", now.Add(2*time.Second), bad); decision.changed {
		t.Fatalf("one bad sample changed profile: %+v", decision)
	}
	if decision := g.observe("ipad", now.Add(4*time.Second), healthyMediaStats(60)); decision.changed {
		t.Fatalf("healthy sample completed stale EWMA downshift: %+v", decision)
	}
}

func TestAdaptiveEncoderSettingsCoordinateEveryLever(t *testing.T) {
	cfg := &config.Config{StreamBitrateK: 16000, StreamQuantizer: 12}
	maxW, maxH, bitrate, quantizer, fps := adaptiveEncoderSettings(adaptiveProfiles[2], cfg, 768, 974)
	if maxW != 0 || maxH != 0 || bitrate != 16000 || quantizer != 14 || fps != 40 {
		t.Fatalf("balanced settings = %dx%d %dk qp=%d fps=%d", maxW, maxH, bitrate, quantizer, fps)
	}
}
