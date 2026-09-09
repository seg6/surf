//go:build !windows

package process

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

// TrackBrowser records the real browser, not a short-lived launcher. On Linux
// os.FindProcess uses a pidfd; keeping the handle avoids PID reuse on signals.
func (s *Started) TrackBrowser(pid int) error {
	group, err := syscall.Getpgid(pid)
	if err != nil {
		return err
	}
	if group != s.Pid {
		return fmt.Errorf("browser escaped its owned process group; cannot safely manage this launcher")
	}
	handle, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	released := false
	alive := func() (bool, error) {
		if released {
			return false, nil
		}
		err := handle.Signal(syscall.Signal(0))
		if err == nil {
			return true, nil
		}
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		return false, err
	}
	s.killOwned = func() error {
		mu.Lock()
		defer mu.Unlock()
		running, err := alive()
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		// No fallback to a PID. An extant member pins this original process group.
		return syscall.Kill(-group, syscall.SIGKILL)
	}
	s.waitOwned = func(timeout time.Duration) error {
		deadline := time.Now().Add(timeout)
		for {
			mu.Lock()
			running, err := alive()
			mu.Unlock()
			if err != nil {
				return err
			}
			if !running {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("browser process is still closing")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	s.releaseOwned = func() {
		mu.Lock()
		defer mu.Unlock()
		if !released {
			handle.Release()
			released = true
		}
	}
	return nil
}
