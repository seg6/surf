package chromium

import (
	"fmt"
	"os"
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
