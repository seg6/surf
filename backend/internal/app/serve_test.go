package app

import (
	"os"
	"path/filepath"
	"testing"

	"surf-backend/internal/config"
)

func TestPrepareCreatesRuntimeStorage(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		SurfHome:       filepath.Join(root, "surf"),
		Profile:        filepath.Join(root, "profile"),
		DownloadsDir:   filepath.Join(root, "downloads"),
		UploadsDir:     filepath.Join(root, "uploads"),
		ChromePath:     filepath.Join(root, "explicit-chromium"),
		ContentBlocker: false,
	}
	if err := Prepare(cfg); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	for _, dir := range []string{cfg.SurfHome, cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("runtime directory %q was not created", dir)
		}
	}
}
