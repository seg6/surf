package browser

import (
	"log"
	"time"

	"surf-backend/internal/protocol"
)

type adaptiveProfile struct {
	name               string
	scaleNum, scaleDen int
}

var adaptiveProfiles = [...]adaptiveProfile{
	{name: "high", scaleNum: 1, scaleDen: 1},
	{name: "balanced", scaleNum: 7, scaleDen: 8},
	{name: "recovery", scaleNum: 25, scaleDen: 32},
}

func (b *Controller) profileIndex() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.adaptiveProfile
}

func (b *Controller) profileName() string {
	return adaptiveProfiles[b.profileIndex()].name
}

func adaptiveScale(profile adaptiveProfile, w, h int) (int, int) {
	if profile.scaleNum >= profile.scaleDen {
		return 0, 0
	}
	return w * profile.scaleNum / profile.scaleDen, h * profile.scaleNum / profile.scaleDen
}

func (b *Controller) handleMediaStats(stats *protocol.MediaStatsCommand) {
	if !b.cfg.AdaptiveVideo {
		return
	}
	// Static tabs naturally produce few access units. During motion, react to
	// real decode/presentation pressure instead of comparing against a fixed
	// source FPS: Chromium now drives capture at the source's available rate,
	// which may exceed the client's display refresh rate.
	activeMotion := stats.AURate >= 5.0
	unhealthy := stats.MemoryWarn || (activeMotion &&
		(stats.DropPercent > 1.0 ||
			stats.GapMS > 50.0 ||
			stats.CallbackMS > 24.0))
	now := time.Now()
	b.mu.Lock()
	current := b.adaptiveProfile
	if now.Sub(b.adaptiveLastChange) < 10*time.Second {
		b.mu.Unlock()
		return
	}
	next := current
	if unhealthy {
		b.adaptiveHealthy = 0
		b.adaptiveUnhealthy++
		// One report can straddle the transition from a static page into
		// motion. Require a second consecutive bad window before restarting
		// the WebCodecs encoder at a lower profile.
		if b.adaptiveUnhealthy >= 2 && next < len(adaptiveProfiles)-1 {
			next++
			b.adaptiveUnhealthy = 0
		}
	} else {
		b.adaptiveUnhealthy = 0
		b.adaptiveHealthy++
		if b.adaptiveHealthy >= 6 && next > 0 { // six 5s reports
			next--
			b.adaptiveHealthy = 0
		}
	}
	if next == current {
		b.mu.Unlock()
		return
	}
	b.adaptiveProfile = next
	b.adaptiveLastChange = now
	profile := adaptiveProfiles[next]
	viewW, viewH := b.viewW, b.viewH
	b.mu.Unlock()

	maxW, maxH := adaptiveScale(profile, viewW, viewH)
	log.Printf("video: adaptive profile %s max=%dx%d", profile.name, maxW, maxH)
	b.video.SetScaleLimit(maxW, maxH)
}
