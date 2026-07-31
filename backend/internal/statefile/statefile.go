// Package statefile provides conservative recovery for replaceable state.
package statefile

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Quarantine moves path aside without deleting it. The returned path remains
// available for diagnostics or manual recovery.
func Quarantine(path, label string) (string, error) {
	if path == "" {
		return "", errors.New("state path is empty")
	}
	if label == "" {
		label = "invalid"
	}
	target := fmt.Sprintf("%s.%s-%s", path, label, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.Rename(path, target); err != nil {
		return "", err
	}
	return target, nil
}
