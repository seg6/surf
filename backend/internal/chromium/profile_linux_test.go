//go:build linux

package chromium

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareProfilePreservesChromiumSingletonFiles(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SingletonLock", "SingletonCookie", "bookmarks.json"} {
		if err := os.WriteFile(filepath.Join(profile, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := PrepareProfile(profile); err != nil {
		t.Fatalf("PrepareProfile: %v", err)
	}
	for _, name := range []string{"SingletonLock", "SingletonCookie"} {
		if _, err := os.Stat(filepath.Join(profile, name)); err != nil {
			t.Fatalf("%s was removed: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(profile, "bookmarks.json")); err != nil {
		t.Fatalf("unrelated profile data was removed: %v", err)
	}
}
