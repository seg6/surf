package runenv

import "surf-backend/internal/config"

// Platform abstracts everything about the host OS that surf-backend needs to
// launch and observe a headful Chromium: bringing up whatever renderable
// surface and audio sink Chromium/ffmpeg need, and describing how to grab
// video/audio for the H.264 and PCM lanes. Everything above this layer (CDP,
// browser tabs, the ws hub, the wire protocol, stream fan-out) never
// branches on GOOS; it only ever talks to a Handle. One implementation
// exists per supported GOOS, selected by newPlatform (build-tagged, one per
// runenv_<os>.go file — the same pattern internal/proc uses for process
// management).
type Platform interface {
	// Prepare brings up whatever host services this platform needs (a
	// virtual display + audio sink on Linux; nothing on Windows/macOS) and
	// may mutate cfg with whatever child processes need (Display,
	// PulseServer, ...). The returned Handle stays alive for the server's
	// lifetime; Shutdown tears down whatever Prepare started.
	Prepare(cfg *config.Config) (Handle, error)
	// Doctor lists the external tools this platform requires, including the
	// ones common to every platform (chromium, ffmpeg).
	Doctor(cfg *config.Config) []Check
}

// Handle is the live instance Prepare returned.
type Handle interface {
	// Shutdown tears down whatever Prepare started.
	Shutdown()
	// ChromeArgs are platform-specific Chromium launch flags (windowing
	// backend, positioning) appended after the common flag set.
	ChromeArgs() []string
	// VideoCaptureArgs builds the ffmpeg argument list — up to and including
	// "-i <surface>" — that grabs this platform's rendered surface for the
	// H.264 lane. Nil means the lane is unsupported here; callers fall back
	// to the JPEG/CDP screencast, which needs no OS-specific capture at all.
	VideoCaptureArgs(surface string, w, h, fps int) []string
	// AudioCaptureArgs builds the ffmpeg argument list — up to and including
	// "-i <source>" — for the PCM lane. Nil means unsupported.
	AudioCaptureArgs(source string) []string
	// ResizeSurface adapts the capture surface to a new viewport size
	// (xrandr's job on Linux); a no-op where there's no virtual display to
	// resize.
	ResizeSurface(w, h int) error
}
