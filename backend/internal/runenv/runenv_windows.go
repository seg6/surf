//go:build windows

// Package runenv's Windows platform runs Chromium directly on the desktop
// session: no virtual display or audio sink to manage, and no H.264/PCM
// capture lane yet — the JPEG/CDP screencast lane covers browsing until a
// gdigrab/WASAPI capture backend lands here.
package runenv

import "surf-backend/internal/config"

func newPlatform() Platform { return windowsPlatform{} }

type windowsPlatform struct{}

func (windowsPlatform) Doctor(cfg *config.Config) []Check {
	return []Check{
		checkTool("chromium", cfg.ChromePath, true),
		checkTool("ffmpeg", cfg.FFmpegPath, true),
	}
}

func (windowsPlatform) Prepare(cfg *config.Config) (Handle, error) {
	return windowsHandle{}, nil
}

// windowsHandle needs no teardown: Prepare didn't start anything.
type windowsHandle struct{}

func (windowsHandle) Shutdown() {}

// ChromeArgs is empty: Chromium uses its normal Windows windowing backend,
// no ozone/X11 flag needed.
func (windowsHandle) ChromeArgs() []string { return nil }

// VideoCaptureArgs is unsupported for now: nil disables the H.264 lane and
// callers fall back to the JPEG/CDP screencast.
func (windowsHandle) VideoCaptureArgs(surface string, w, h, fps int) []string { return nil }

// AudioCaptureArgs is unsupported for now: nil disables the PCM lane.
func (windowsHandle) AudioCaptureArgs(source string) []string { return nil }

// ResizeSurface is a no-op: there's no virtual display to resize.
func (windowsHandle) ResizeSurface(w, h int) error { return nil }
