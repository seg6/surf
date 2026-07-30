//go:build linux

package chromium

import (
	"os"
	"path/filepath"
	"strings"
)

func cleanupProfileLocks(profile string) error {
	entries, err := os.ReadDir(profile)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "Singleton") {
			_ = os.RemoveAll(filepath.Join(profile, entry.Name()))
		}
	}
	return nil
}
