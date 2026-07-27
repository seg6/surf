//go:build darwin

// Package runenv's macOS platform runs Chromium directly on the desktop
// session: no virtual display or audio sink to manage, and no H.264/PCM
// capture lane yet — the JPEG/CDP screencast lane covers browsing until an
// avfoundation capture backend lands here.
package runenv

import "surf-backend/internal/config"

func newPlatform() Platform { return darwinPlatform{} }

type darwinPlatform struct{}

func (darwinPlatform) Doctor(cfg *config.Config) []Check {
	return []Check{
		checkTool("chromium", cfg.ChromePath, true),
		checkTool("ffmpeg", cfg.FFmpegPath, true),
	}
}

func (darwinPlatform) Prepare(cfg *config.Config) (Handle, error) {
	return darwinHandle{}, nil
}

// darwinHandle needs no teardown: Prepare didn't start anything.
type darwinHandle struct{}

func (darwinHandle) Shutdown() {}

// ChromeArgs is empty: Chromium uses its normal macOS windowing backend.
func (darwinHandle) ChromeArgs() []string { return nil }

// VideoCaptureArgs is unsupported for now: nil disables the H.264 lane and
// callers fall back to the JPEG/CDP screencast.
func (darwinHandle) VideoCaptureArgs(surface string, w, h, fps int) []string { return nil }

// AudioCaptureArgs is unsupported for now: nil disables the PCM lane.
func (darwinHandle) AudioCaptureArgs(source string) []string { return nil }

// ResizeSurface is a no-op: there's no virtual display to resize.
func (darwinHandle) ResizeSurface(w, h int) error { return nil }
