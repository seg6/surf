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

func TestQuarantineProfilePreservesSurfLibrary(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"bookmarks.json": "bookmarks",
		"history.jsonl":  "history",
		"Preferences":    "poisoned browser state",
	}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(profile, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	backup, err := QuarantineProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("profile was not preserved")
	}
	for _, name := range []string{"bookmarks.json", "history.jsonl"} {
		data, err := os.ReadFile(filepath.Join(profile, name))
		if err != nil || string(data) != files[name] {
			t.Fatalf("restored %s=%q err=%v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(profile, "Preferences")); !os.IsNotExist(err) {
		t.Fatalf("poisoned browser state was restored: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(backup, "Preferences")); err != nil || string(data) != files["Preferences"] {
		t.Fatalf("backup Preferences=%q err=%v", data, err)
	}
}
