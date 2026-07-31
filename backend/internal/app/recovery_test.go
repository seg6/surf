package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrowserRecoveryRequiresRepeatedFailureAndHasCooldown(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	recover, err := noteBrowserStartupFailure(home, filepath.Join(home, "profile"), now)
	if err != nil || recover {
		t.Fatalf("first failure recover=%v err=%v", recover, err)
	}
	recover, err = noteBrowserStartupFailure(home, filepath.Join(home, "profile"), now.Add(time.Second))
	if err != nil || !recover {
		t.Fatalf("second failure recover=%v err=%v", recover, err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		recover, err = noteBrowserStartupFailure(home, filepath.Join(home, "profile"), now.Add(time.Duration(attempt+2)*time.Second))
		if err != nil || recover {
			t.Fatalf("cooldown attempt %d recover=%v err=%v", attempt, recover, err)
		}
	}
	clearBrowserStartupFailures(home)
	if _, err := os.Stat(browserRecoveryPath(home)); !os.IsNotExist(err) {
		t.Fatalf("recovery marker remains: %v", err)
	}
}

func TestBrowserRecoveryFailureWindowResetsCount(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile")
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if recover, err := noteBrowserStartupFailure(home, profile, now); err != nil || recover {
		t.Fatalf("first failure recover=%v err=%v", recover, err)
	}
	if recover, err := noteBrowserStartupFailure(home, profile, now.Add(browserFailureWindow+time.Second)); err != nil || recover {
		t.Fatalf("stale second failure recover=%v err=%v", recover, err)
	}
}
