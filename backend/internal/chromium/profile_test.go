package chromium

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareProfileCreatesProfileDirectory(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	if err := PrepareProfile(profile); err != nil {
		t.Fatalf("PrepareProfile: %v", err)
	}
	if info, err := os.Stat(profile); err != nil || !info.IsDir() {
		t.Fatalf("profile directory %q was not created", profile)
	}
}
