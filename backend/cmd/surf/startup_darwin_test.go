//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartAtLoginDarwin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := setStartAtLogin(true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "space.seg6.surf.desktop.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<key>Label</key><string>space.seg6.surf.desktop</string>",
		"<key>ProgramArguments</key>",
		"<key>RunAtLoad</key><true/>",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("launch agent is missing %q: %s", want, data)
		}
	}
	if err := setStartAtLogin(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("launch agent still exists: %v", err)
	}
}
