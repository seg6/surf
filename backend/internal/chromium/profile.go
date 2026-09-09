package chromium

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"surf-backend/internal/process"
	"surf-backend/internal/statefile"
)

// PrepareProfile creates the persistent browser profile. Chromium owns its
// singleton files; Surf must never delete a live browser's ownership markers.
func PrepareProfile(profile string) error {
	if profile == "" {
		return nil
	}
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return fmt.Errorf("create browser profile %s: %w", profile, err)
	}
	return nil
}

// CheckProfileAvailable checks native ownership before each handoff launch.
// Chromium remains the final lock authority; no singleton file is removed.
func CheckProfileAvailable(profile string) error { return checkProfileOwner(profile) }

// LockProfile is shared by streaming and standalone setup, including homes
// configured to use the same profile. Keep the lock outside the profile so a
// recovery rename cannot release ownership of its replacement.
func LockProfile(profile string) (*process.InstanceLock, error) {
	if err := PrepareProfile(profile); err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(profile)
	if err != nil {
		return nil, err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, err
	}
	lock, acquired, err := process.AcquireInstanceLock(canonical + ".surf-lock")
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, fmt.Errorf("browser profile is in use by Surf or standalone browser setup; close that session first")
	}
	if err := checkProfileOwner(canonical); err != nil {
		_ = lock.Close()
		return nil, err
	}
	return lock, nil
}

// QuarantineProfile preserves a browser profile that repeatedly prevents
// Chromium startup, then restores Surf-owned library files into a fresh
// profile. Cookies and Chromium databases stay in the backup for diagnosis.
func QuarantineProfile(profile string) (string, error) {
	if profile == "" {
		return "", fmt.Errorf("browser profile is empty")
	}
	var backup string
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		backup, err = statefile.Quarantine(profile, "startup-failed")
		if err == nil {
			break
		}
		if os.IsNotExist(err) {
			return "", nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return "", fmt.Errorf("quarantine browser profile: %w", err)
	}
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return backup, fmt.Errorf("create replacement browser profile: %w", err)
	}
	for _, name := range []string{"bookmarks.json", "history.jsonl"} {
		source := filepath.Join(backup, name)
		if _, statErr := os.Stat(source); os.IsNotExist(statErr) {
			continue
		}
		if copyErr := copyProfileFile(source, filepath.Join(profile, name)); copyErr != nil {
			return backup, copyErr
		}
	}
	return backup, nil
}

func copyProfileFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
