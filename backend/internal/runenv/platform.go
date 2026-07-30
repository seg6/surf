package runenv

import "surf-backend/internal/config"

// Platform abstracts what's left that's genuinely host-OS-specific once
// Chromium runs headless and the H.264 lane transcodes CDP's own screencast
// frames instead of grabbing the OS display: maintaining the optional Linux
// PulseAudio fallback and host process-lifetime behavior. Everything else
// (CDP, browser tabs, the ws hub,
// the wire protocol, stream fan-out, and even Chromium's own launch flags)
// is identical on every OS and lives above this layer. One implementation
// exists per supported GOOS, selected by newPlatform (build-tagged, one per
// runenv_<os>.go file — the same pattern internal/proc uses for process
// management).
type Platform interface {
	// Prepare brings up whatever host services this platform needs (an
	// fallback audio sink on Linux; no separate service on Windows/macOS) and may mutate cfg
	// with whatever child processes need (PulseServer, ...). The returned
	// Handle stays alive for the server's lifetime; Shutdown tears down
	// whatever Prepare started.
	Prepare(cfg *config.Config) (Handle, error)
	// Doctor lists the external tools this platform requires, including the
	// ones common to every platform (chromium, ffmpeg).
	Doctor(cfg *config.Config) []Check
}

// Handle is the live instance Prepare returned.
type Handle interface {
	// Shutdown tears down whatever Prepare started.
	Shutdown()
	// AudioCaptureArgs builds the optional FFmpeg fallback argument list up to
	// and including "-i <source>". Nil means no platform fallback.
	AudioCaptureArgs(source string) []string
}
