package chromium

import (
	"path/filepath"
	"testing"
)

func TestProfileOwnership(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	first, err := LockProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := LockProfile(profile); err == nil {
		second.Close()
		t.Fatal("shared profile acquired twice")
	}
	first.Close()
	next, err := LockProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}
