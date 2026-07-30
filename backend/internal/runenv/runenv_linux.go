//go:build linux

// Package runenv's Linux platform prepares Surf's data directories and
// removes stale Chromium profile locks. Audio is captured directly from the
// active Chromium tab above this platform layer.
package runenv

import (
	"os"
	"path/filepath"
	"strings"

	"surf-backend/internal/config"
)

func newPlatform() Platform { return linuxPlatform{} }

type linuxPlatform struct{}

func (linuxPlatform) Doctor(cfg *config.Config) []Check {
	return []Check{
		checkTool("chromium", cfg.ChromePath, true),
	}
}

func (linuxPlatform) Prepare(cfg *config.Config) (Handle, error) {
	for _, dir := range []string{cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	_ = cleanupChromeSingletons(cfg.Profile)
	return linuxHandle{}, nil
}

type linuxHandle struct{}

func (linuxHandle) Shutdown() {}

func cleanupChromeSingletons(profile string) error {
	entries, err := os.ReadDir(profile)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "Singleton") {
			_ = os.RemoveAll(filepath.Join(profile, entry.Name()))
		}
	}
	return nil
}
