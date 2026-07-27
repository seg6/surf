//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartAtLoginLinux(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	if err := setStartAtLogin(true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config, "autostart", "surf.desktop")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "X-GNOME-Autostart-enabled=true") {
		t.Fatalf("unexpected desktop entry: %s", data)
	}
	if err := setStartAtLogin(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("autostart file still exists: %v", err)
	}
}
