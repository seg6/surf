//go:build darwin

// The macOS platform needs nothing host-specific yet: Chromium
// runs headless (see internal/cdp), and Chromium tab capture owns media above
// this platform layer.
package platform

import "surf-backend/internal/config"

func newPlatform() platform { return darwinPlatform{} }

type darwinPlatform struct{}

func (darwinPlatform) Doctor(cfg *config.Config) []Check {
	return []Check{
		checkTool("chromium", cfg.ChromePath, true),
	}
}

func (darwinPlatform) Prepare(cfg *config.Config) (handle, error) {
	return darwinHandle{}, nil
}

// darwinHandle needs no teardown: Prepare didn't start anything.
type darwinHandle struct{}

func (darwinHandle) Shutdown() {}
