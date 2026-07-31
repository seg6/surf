package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"surf-backend/internal/atomicfile"
)

const (
	browserFailureWindow    = 10 * time.Minute
	browserRecoveryCooldown = 24 * time.Hour
)

type browserRecoveryState struct {
	Profile      string    `json:"profile"`
	Failures     int       `json:"failures"`
	LastFailure  time.Time `json:"lastFailure"`
	LastRecovery time.Time `json:"lastRecovery,omitempty"`
}

func browserRecoveryPath(home string) string {
	return filepath.Join(home, "runtime", "browser-startup.json")
}

func noteBrowserStartupFailure(home, profile string, now time.Time) (bool, error) {
	path := browserRecoveryPath(home)
	var state browserRecoveryState
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &state)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if state.Profile != profile {
		state = browserRecoveryState{Profile: profile}
	} else if state.LastFailure.IsZero() || now.Sub(state.LastFailure) > browserFailureWindow {
		state.Failures = 0
	}
	state.Failures++
	state.LastFailure = now.UTC()
	recover := state.Failures >= 2 && (state.LastRecovery.IsZero() || now.Sub(state.LastRecovery) >= browserRecoveryCooldown)
	if recover {
		state.Failures = 0
		state.LastRecovery = now.UTC()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return false, err
	}
	if err := atomicfile.Replace(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return recover, nil
}

func clearBrowserStartupFailures(home string) {
	_ = os.Remove(browserRecoveryPath(home))
}
