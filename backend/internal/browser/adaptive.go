package browser

import (
	"fmt"
	"log"
	"math"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
	"surf-backend/internal/transport"
)

const (
	governorSampleTTL        = 7 * time.Second
	governorDownshiftDelay   = 1500 * time.Millisecond
	governorProfileSettle    = 4 * time.Second
	governorHealthyHold      = 12 * time.Second
	governorStaticHold       = 4 * time.Second
	governorPressureAlpha    = 0.45
	governorActiveAURate     = 5.0
	governorUnhealthySamples = 2
)

// Profiles are deliberately few and ordered. Preserve native resolution so
// text and images stay sharp; cadence is the primary pressure lever because
// it directly bounds how much work legacy VideoToolbox must finish per second.
type adaptiveProfile struct {
	name                   string
	scaleNum, scaleDen     int
	frameRate              int
	quantizerDelta         int
	bitrateNum, bitrateDen int
}

var adaptiveProfiles = [...]adaptiveProfile{
	{name: "crisp", scaleNum: 1, scaleDen: 1, frameRate: 60, bitrateNum: 1, bitrateDen: 1},
	{name: "motion", scaleNum: 1, scaleDen: 1, frameRate: 50, quantizerDelta: 1, bitrateNum: 1, bitrateDen: 1},
	{name: "balanced", scaleNum: 1, scaleDen: 1, frameRate: 40, quantizerDelta: 2, bitrateNum: 1, bitrateDen: 1},
	{name: "recovery", scaleNum: 1, scaleDen: 1, frameRate: 30, quantizerDelta: 3, bitrateNum: 1, bitrateDen: 1},
}

type governorClient struct {
	lastSeen     time.Time
	healthySince time.Time
	staticSince  time.Time
	pressure     float64
	hasPressure  bool
	unhealthy    int
	active       bool
	severe       bool
	reason       string
	settling     int
}

type streamGovernor struct {
	profile     int
	lastChange  time.Time
	settleUntil time.Time
	clients     map[string]*governorClient
}

type governorDecision struct {
	changed  bool
	from, to int
	reason   string
	pressure float64
}

func newStreamGovernor() streamGovernor {
	return streamGovernor{clients: map[string]*governorClient{}}
}

func (g *streamGovernor) remove(clientID string) {
	delete(g.clients, clientID)
}

func (g *streamGovernor) settleAfterDiscontinuity(now time.Time) {
	g.settleUntil = now.Add(governorProfileSettle)
	for _, state := range g.clients {
		state.active = false
		state.severe = false
		state.reason = "source warm-up"
		state.unhealthy = 0
		state.healthySince = time.Time{}
		state.staticSince = time.Time{}
		state.pressure = 0
		state.hasPressure = false
		// The first report after the timer may still begin inside the source
		// transition. Require one wholly subsequent active sample.
		state.settling = 1
	}
}

func (g *streamGovernor) observe(clientID string, now time.Time, stats protocol.MediaStatsCommand) governorDecision {
	if g.clients == nil {
		g.clients = map[string]*governorClient{}
	}
	for id, client := range g.clients {
		if now.Sub(client.lastSeen) > governorSampleTTL {
			delete(g.clients, id)
		}
	}
	client := g.clients[clientID]
	if client == nil {
		// The first report often spans VideoToolbox creation and the first IDR.
		// It is useful telemetry but not a steady-state control sample.
		client = &governorClient{settling: 1}
		g.clients[clientID] = client
	}
	client.lastSeen = now
	if now.Before(g.settleUntil) {
		// A native report can straddle an encoder-generation boundary. Never
		// carry that mixed evidence through the settle interval: it used to
		// become an instant downshift as soon as the timer expired.
		client.active = false
		client.severe = false
		client.reason = "profile warm-up"
		client.unhealthy = 0
		client.healthySince = time.Time{}
		client.staticSince = time.Time{}
		client.pressure = 0
		client.hasPressure = false
		return governorDecision{from: g.profile, to: g.profile}
	}

	profile := adaptiveProfiles[g.profile]
	pressure, active, severe, reason := governorPressure(stats, profile)
	client.active = active
	client.severe = severe
	client.reason = reason
	if active && client.settling > 0 {
		client.settling--
		client.unhealthy = 0
		client.healthySince = time.Time{}
		client.staticSince = time.Time{}
		client.pressure = 0
		client.hasPressure = false
		client.severe = false
		client.reason = "decoder warm-up"
	} else if !active {
		client.unhealthy = 0
		client.healthySince = time.Time{}
		if client.staticSince.IsZero() {
			client.staticSince = now
		}
		client.pressure *= 0.5
		client.hasPressure = false
	} else {
		client.staticSince = time.Time{}
		if client.hasPressure {
			client.pressure = client.pressure*(1-governorPressureAlpha) + pressure*governorPressureAlpha
		} else {
			client.pressure = pressure
			client.hasPressure = true
		}
		// EWMA describes the trend, but hysteresis must be consecutive current
		// samples. Otherwise one gap leaves the smoothed value above one and a
		// healthy following sample incorrectly completes the downshift pair.
		if severe || pressure >= 1.0 {
			client.unhealthy++
			client.healthySince = time.Time{}
		} else {
			client.unhealthy = 0
			if client.pressure <= 0.72 {
				if client.healthySince.IsZero() {
					client.healthySince = now
				}
			} else {
				client.healthySince = time.Time{}
			}
		}
	}

	worstPressure := 0.0
	worstReason := "decoder pressure"
	severeAny, unhealthyAny := false, false
	allStatic, allHealthy := len(g.clients) > 0, len(g.clients) > 0
	for _, state := range g.clients {
		if now.Sub(state.lastSeen) > governorSampleTTL {
			continue
		}
		if state.pressure > worstPressure {
			worstPressure, worstReason = state.pressure, state.reason
		}
		if state.severe {
			severeAny = true
			worstReason = state.reason
		}
		if state.unhealthy >= governorUnhealthySamples {
			unhealthyAny = true
		}
		if state.staticSince.IsZero() || now.Sub(state.staticSince) < governorStaticHold {
			allStatic = false
		}
		if state.healthySince.IsZero() || now.Sub(state.healthySince) < governorHealthyHold {
			allHealthy = false
		}
	}

	decision := governorDecision{from: g.profile, to: g.profile, pressure: worstPressure}
	if now.Before(g.settleUntil) {
		return decision
	}
	canDownshift := g.lastChange.IsZero() || now.Sub(g.lastChange) >= governorDownshiftDelay
	if canDownshift && severeAny && g.profile < len(adaptiveProfiles)-1 {
		decision.to = min(len(adaptiveProfiles)-1, g.profile+2)
		decision.reason = worstReason
	} else if canDownshift && unhealthyAny && g.profile < len(adaptiveProfiles)-1 {
		decision.to = g.profile + 1
		decision.reason = worstReason
	} else if g.profile > 0 && allStatic && now.Sub(g.lastChange) >= governorStaticHold {
		decision.to = 0
		decision.reason = "motion stopped"
	} else if g.profile > 0 && allHealthy && now.Sub(g.lastChange) >= governorHealthyHold {
		decision.to = g.profile - 1
		decision.reason = "sustained decoder headroom"
	}
	if decision.to == g.profile {
		return decision
	}

	decision.changed = true
	g.profile = decision.to
	g.lastChange = now
	g.settleUntil = now.Add(governorProfileSettle)
	// The new profile has a different frame budget. Require fresh evidence at
	// that budget instead of carrying counters across the reconfiguration.
	for _, state := range g.clients {
		state.unhealthy = 0
		state.healthySince = time.Time{}
		state.hasPressure = false
		state.severe = false
	}
	return decision
}

func governorPressure(stats protocol.MediaStatsCommand, profile adaptiveProfile) (float64, bool, bool, string) {
	if stats.MemoryWarn {
		return 4, true, true, "memory warning"
	}
	if stats.AURate < governorActiveAURate {
		return 0, false, false, "static page"
	}
	frameBudget := 1000.0 / float64(profile.frameRate)
	pressure, reason := 0.0, "decoder pressure"
	consider := func(value float64, label string) {
		if value > pressure {
			pressure, reason = value, label
		}
	}
	consider(stats.DropPercent/1.5, "decoder drops")
	if stats.Renderer == "system" {
		consider(float64(stats.QueueDepth)/2.0, "renderer queue")
		consider(float64(stats.RendererBackpressure)/3.0, "renderer backpressure")
		consider(float64(stats.RendererRecoveries)*2.0, "renderer recovery")
		consider(float64(stats.RendererFailures)*4.0, "renderer failures")
	} else {
		consider(float64(stats.QueueDepth)/2.0, "decoder queue")
		consider(stats.CallbackMS/(frameBudget*0.9), "decode callback time")
		consider(float64(stats.DecodeErrors)*2.0, "decoder errors")
	}

	decodeRatio, presentRatio := 1.0, 1.0
	if stats.WindowMS > 0 && stats.AURate > 0 {
		if stats.Renderer == "system" {
			decodeRatio = stats.RendererFPS / stats.AURate
			presentRatio = decodeRatio
			consider(math.Max(0, 0.92-decodeRatio)/0.20, "renderer throughput")
		} else {
			decodeRatio = stats.DecodeFPS / stats.AURate
			presentRatio = stats.PresentedFPS / stats.AURate
			consider(math.Max(0, 0.92-decodeRatio)/0.20, "decoder throughput")
			consider(math.Max(0, 0.88-presentRatio)/0.25, "presentation throughput")
		}
		// A large maximum gap with matching AU/decode/presentation rates is a
		// compositor pause or a page becoming static, not decoder overload.
		if presentRatio < 0.85 {
			consider(stats.GapMS/(frameBudget*3.0), "presentation gaps")
		}
	}
	severe := stats.DecodeErrors > 0 || stats.RendererFailures > 0 ||
		stats.RendererRecoveries > 0 || stats.QueueDepth >= 3 || stats.DropPercent >= 8 ||
		stats.WindowMS > 0 && (decodeRatio < 0.50 || presentRatio < 0.35)
	return pressure, true, severe, reason
}

func (b *Controller) profileIndex() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.governor.profile
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

func adaptiveEncoderSettings(profile adaptiveProfile, cfg *config.Config, w, h int) (maxW, maxH, bitrateK, quantizer, frameRate int) {
	maxW, maxH = adaptiveScale(profile, w, h)
	bitrateK = cfg.StreamBitrateK * profile.bitrateNum / profile.bitrateDen
	if bitrateK < 1000 {
		bitrateK = 1000
	}
	quantizer = max(0, min(51, cfg.StreamQuantizer+profile.quantizerDelta))
	return maxW, maxH, bitrateK, quantizer, profile.frameRate
}

func governorClientID(c *transport.Client) string {
	if id := c.DeviceID(); id != "" {
		return id
	}
	return fmt.Sprintf("socket:%p", c)
}

func (b *Controller) handleMediaStats(c *transport.Client, stats *protocol.MediaStatsCommand) {
	if !b.cfg.AdaptiveVideo {
		return
	}
	now := time.Now()
	b.mu.Lock()
	decision := b.governor.observe(governorClientID(c), now, *stats)
	profile := adaptiveProfiles[b.governor.profile]
	b.mu.Unlock()
	if !decision.changed {
		return
	}
	go b.applyAdaptiveProfile(decision, profile)
}

func (b *Controller) applyAdaptiveProfile(decision governorDecision, profile adaptiveProfile) {
	b.adaptiveApplyMu.Lock()
	defer b.adaptiveApplyMu.Unlock()
	b.mu.Lock()
	if b.governor.profile != decision.to {
		b.mu.Unlock()
		return
	}
	w, h := b.viewW, b.viewH
	b.mu.Unlock()
	maxW, maxH, bitrateK, quantizer, frameRate := adaptiveEncoderSettings(profile, b.cfg, w, h)
	log.Printf("video: governor %s -> %s pressure=%.2f reason=%s coded-max=%dx%d fps=%d qp=%d fallback=%dkbit/s",
		adaptiveProfiles[decision.from].name, profile.name, decision.pressure,
		decision.reason, maxW, maxH, frameRate, quantizer, bitrateK)
	b.video.SetProfile(maxW, maxH, bitrateK, quantizer, frameRate)
}
