// Package runenv owns host-mode process orchestration for surf-backend: it
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

// Handle exposes the active platform's capture/launch hooks to the browser
// package: Chromium launch flags, ffmpeg capture argument builders for the
// H.264/PCM lanes, and capture-surface resize.
func (r *Runtime) Handle() Handle { return r.handle }

// Shutdown tears down whatever the platform's Prepare started.
func (r *Runtime) Shutdown() {
	if r.handle != nil {
		r.handle.Shutdown()
	}
}

// Start prepares whatever host services the current OS needs — a virtual
// display and audio sink on Linux; nothing on Windows/macOS — via the
// platform matching runtime.GOOS.
func Start(cfg *config.Config) (*Runtime, error) {
	handle, err := newPlatform().Prepare(cfg)
	if err != nil {
		return nil, err
	}
	return &Runtime{handle: handle}, nil
}
