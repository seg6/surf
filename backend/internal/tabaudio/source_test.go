package tabaudio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewExtractsLoopbackOnlyExtension(t *testing.T) {
	source, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer source.Close()

	manifest, err := os.ReadFile(filepath.Join(source.ExtensionPath(), "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifest), `"tabCapture"`) ||
		!strings.Contains(string(manifest), `"offscreen"`) {
		t.Fatalf("manifest does not declare audio permissions: %s", manifest)
	}
	config, err := os.ReadFile(filepath.Join(source.ExtensionPath(), "config.js"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(config), `ws://127.0.0.1:`) ||
		!strings.Contains(string(config), `/audio/`) {
		t.Fatalf("bridge is not loopback-scoped: %s", config)
	}
}

func TestOpenBeforeAttachFails(t *testing.T) {
	source, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer source.Close()
	if _, err := source.Open(); err == nil {
		t.Fatal("Open succeeded before Attach")
	}
}
