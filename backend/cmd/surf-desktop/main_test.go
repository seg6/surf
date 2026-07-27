package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopConfigRoundTrip(t *testing.T) {
	home := t.TempDir()
	want := desktopConfig{Password: "test-secret", Port: 19090}
	if err := saveDesktopConfig(home, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadDesktopConfig(home)
	if err != nil || got != want {
		t.Fatalf("config=%+v err=%v", got, err)
	}
	info, err := os.Stat(filepath.Join(home, "desktop.json"))
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}

	want = desktopConfig{Password: "replacement-secret", Port: 18080}
	if err := saveDesktopConfig(home, want); err != nil {
		t.Fatal(err)
	}
	got, err = loadDesktopConfig(home)
	if err != nil || got != want {
		t.Fatalf("replacement config=%+v err=%v", got, err)
	}
}

func TestRandomPassword(t *testing.T) {
	first, err := randomPassword()
	if err != nil || len(first) < 20 {
		t.Fatalf("password length=%d err=%v", len(first), err)
	}
	second, err := randomPassword()
	if err != nil || first == second {
		t.Fatalf("password collision or error: %v", err)
	}
}

func TestTrayIconIsICOWithPNG(t *testing.T) {
	icon := trayIcon()
	if len(icon) < 30 || !bytes.Equal(icon[:4], []byte{0, 0, 1, 0}) {
		t.Fatalf("bad ICO header: %x", icon[:min(8, len(icon))])
	}
	offset := int(icon[18]) | int(icon[19])<<8 | int(icon[20])<<16 | int(icon[21])<<24
	if offset >= len(icon) || !bytes.Equal(icon[offset:offset+4], []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("ICO does not contain PNG at %d", offset)
	}
}
