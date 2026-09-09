//go:build !windows

package chromium

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"surf-backend/internal/process"
)

func checkProfileOwner(profile string) error {
	path := filepath.Join(profile, "SingletonLock")
	owner, err := os.Readlink(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot verify browser profile ownership: %w", err)
	}
	split := strings.LastIndexByte(owner, '-')
	host, hostErr := os.Hostname()
	if split < 1 || hostErr != nil || owner[:split] != host {
		return fmt.Errorf("browser profile has an unverified owner; close the browser using it before starting Surf")
	}
	pid, err := strconv.Atoi(owner[split+1:])
	if err != nil || pid <= 0 || process.Running(pid) {
		return fmt.Errorf("browser profile is in use by another browser; close it before starting Surf")
	}
	// Proven dead local owner. Leave cleanup to Chromium itself.
	return nil
}
