//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"testing"

	"surf-backend/internal/config"
)

func TestLinuxDoctorChecksChromium(t *testing.T) {
	checks := Doctor(&config.Config{
		ChromePath: "missing-chromium",
	})
	if len(checks) != 1 || checks[0].Name != "chromium" {
		t.Fatalf("checks=%v, want chromium only", checks)
	}
}

func TestLinuxPlatformPreparesDataDirectories(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Profile:      filepath.Join(root, "profile"),
		DownloadsDir: filepath.Join(root, "downloads"),
		UploadsDir:   filepath.Join(root, "uploads"),
	}
	handle, err := newPlatform().Prepare(cfg)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer handle.Shutdown()
	for _, dir := range []string{cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("data directory %q was not created", dir)
		}
	}
}
