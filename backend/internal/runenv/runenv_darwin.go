//go:build darwin

// Package runenv's macOS platform needs nothing host-specific yet: Chromium
// runs headless (see internal/cdp) and the H.264 lane transcodes CDP's own
// screencast frames instead of grabbing the screen, so there's no
// display/desktop bring-up needed. The PCM lane is unsupported here for now
// (no capture source has been wired up).
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

// AudioCaptureArgs is unsupported for now: nil disables the PCM lane.
func (darwinHandle) AudioCaptureArgs(source string) []string { return nil }
