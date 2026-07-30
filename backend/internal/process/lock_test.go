package process

import (
	"path/filepath"
	"testing"
)

func TestInstanceLockExcludesSecondOwnerAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.lock")
	first, acquired, err := AcquireInstanceLock(path)
	if err != nil || !acquired {
		t.Fatalf("first lock: acquired=%v err=%v", acquired, err)
	}
	defer first.Close()

	second, acquired, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatalf("second lock: %v", err)
	}
	if acquired {
		second.Close()
		t.Fatal("second owner acquired the same lock")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}
	third, acquired, err := AcquireInstanceLock(path)
	if err != nil || !acquired {
		t.Fatalf("lock after release: acquired=%v err=%v", acquired, err)
	}
	_ = third.Close()
}
