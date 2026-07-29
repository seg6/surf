// Package runenv owns host-mode process orchestration for Surf: it
// dispatches to whichever Platform matches runtime.GOOS so everything above
// this layer (CDP, browser tabs, the ws hub, the wire protocol, stream
// fan-out) never branches on the host OS.
package runenv

import "surf-backend/internal/config"

// Runtime is the live host runtime returned by Start; call Shutdown when the
// server exits.
type Runtime struct {
	handle Handle
}

// Handle exposes the one remaining host-specific facility: system-audio
// capture. Chromium launch and the H.264 path are platform-independent.
func (r *Runtime) Handle() Handle { return r.handle }

// Shutdown tears down whatever the platform's Prepare started.
func (r *Runtime) Shutdown() {
	if r.handle != nil {
		r.handle.Shutdown()
	}
}

// Start prepares whatever audio service the current OS needs — a PulseAudio
// sink on Linux, a process-loopback handle on Windows, and nothing on macOS —
// via the matching implementation.
func Start(cfg *config.Config) (*Runtime, error) {
	handle, err := newPlatform().Prepare(cfg)
	if err != nil {
		return nil, err
	}
	return &Runtime{handle: handle}, nil
}
