package chromium

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"surf-backend/internal/statefile"
)

// PrepareProfile creates the persistent browser profile and clears stale
// Chromium singleton files where the host requires it.
func PrepareProfile(profile string) error {
	if profile == "" {
		return nil
	}
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return fmt.Errorf("create browser profile %s: %w", profile, err)
	}
	return cleanupProfileLocks(profile)
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
