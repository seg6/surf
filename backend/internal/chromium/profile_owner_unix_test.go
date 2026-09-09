//go:build !windows

package chromium

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeProfileOwnersAreNeverCleared(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{fmt.Sprintf("%s-%d", host, os.Getpid()), "other-computer-123", "uncertain"} {
		t.Run(owner, func(t *testing.T) {
			profile := t.TempDir()
			marker := filepath.Join(profile, "SingletonLock")
			if err := os.Symlink(owner, marker); err != nil {
				t.Fatal(err)
			}
			if lock, err := LockProfile(profile); err == nil {
				lock.Close()
				t.Fatal("accepted live/uncertain owner")
			}
			if actual, err := os.Readlink(marker); err != nil || actual != owner {
				t.Fatal("native owner marker changed")
			}
		})
	}
}

func TestProfileAliasSharesOwnership(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	lock, err := LockProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(profile, alias); err != nil {
		t.Fatal(err)
	}
	if second, err := LockProfile(alias); err == nil {
		second.Close()
		t.Fatal("symlink bypassed profile ownership")
	}
}
