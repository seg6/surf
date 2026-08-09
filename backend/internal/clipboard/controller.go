// Package clipboard owns Surf's ephemeral host clipboard state. Clipboard
// contents are never persisted; only the owner's sync-enabled preference is.
package clipboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"surf-backend/internal/atomicfile"
)

const MaxBytes = 64 << 10

type State struct {
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	System    string `json:"system"`
	Known     bool   `json:"known"`
	Text      string `json:"text"`
	Revision  uint64 `json:"revision"`
}

type Controller struct {
	mu         sync.Mutex
	settingsMu sync.Mutex
	systemMu   sync.Mutex
	home       string
	system     systemClipboard
	enabled    bool
	known      bool
	text       string
	revision   uint64
}

type settings struct {
	Enabled bool `json:"enabled"`
}

func operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 2*time.Second)
}

func New(home string) (*Controller, error) {
	return newController(home, newSystemClipboard())
}

func newController(home string, system systemClipboard) (*Controller, error) {
	if system == nil {
		system = unavailableSystem{}
	}
	c := &Controller{home: home, system: system}
	data, err := os.ReadFile(c.settingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read clipboard settings: %w", err)
	}
	var saved settings
	if err := json.Unmarshal(data, &saved); err != nil {
		return c, fmt.Errorf("read clipboard settings: %w", err)
	}
	c.enabled = saved.Enabled
	return c, nil
}

func Validate(text string) error {
	if !utf8.ValidString(text) || len([]byte(text)) > MaxBytes {
		return fmt.Errorf("clipboard text must contain at most %d bytes of UTF-8", MaxBytes)
	}
	if strings.IndexByte(text, 0) >= 0 {
		return errors.New("clipboard text cannot contain a NUL byte")
	}
	return nil
}

func (c *Controller) settingsPath() string {
	return filepath.Join(c.home, "clipboard.json")
}

func (c *Controller) snapshotLocked() State {
	return State{
		Enabled: c.enabled, Available: c.system.Name() != "unavailable",
		System: c.system.Name(), Known: c.known, Text: c.text, Revision: c.revision,
	}
}

func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

func (c *Controller) persist(enabled bool) error {
	if err := os.MkdirAll(c.home, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings{Enabled: enabled}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := c.settingsPath() + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := atomicfile.Replace(temporary, c.settingsPath()); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

// SetEnabled persists only the preference. Enabling samples the current host
// clipboard so it becomes the initial value sent to connected devices.
func (c *Controller) SetEnabled(ctx context.Context, enabled bool) (State, error) {
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	if err := c.persist(enabled); err != nil {
		return c.State(), fmt.Errorf("save clipboard settings: %w", err)
	}
	c.mu.Lock()
	c.enabled = enabled
	if !enabled {
		c.known, c.text = false, ""
	}
	state := c.snapshotLocked()
	c.mu.Unlock()
	if !enabled {
		return state, nil
	}
	state, _, err := c.Refresh(ctx)
	if errors.Is(err, ErrUnavailable) {
		err = nil // device-to-device and CLI state still work without an OS bridge
	}
	return state, err
}

func (c *Controller) apply(text string) (State, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.known && c.text == text {
		return c.snapshotLocked(), false
	}
	c.known, c.text = true, text
	c.revision++
	return c.snapshotLocked(), true
}

// Refresh samples the operating-system clipboard while sync is enabled.
func (c *Controller) Refresh(ctx context.Context) (State, bool, error) {
	state := c.State()
	if !state.Enabled {
		return state, false, nil
	}
	c.systemMu.Lock()
	defer c.systemMu.Unlock()
	operation, cancel := operationContext(ctx)
	defer cancel()
	text, err := c.system.Read(operation)
	if err != nil {
		return state, false, err
	}
	if err := Validate(text); err != nil {
		return state, false, err
	}
	if state = c.State(); !state.Enabled {
		return state, false, nil
	}
	state, changed := c.apply(text)
	return state, changed, nil
}

// SetHost updates Surf's ephemeral value and the operating-system clipboard.
// The state remains usable by headless CLI clients when no OS bridge exists.
func (c *Controller) SetHost(ctx context.Context, text string) (State, bool, error) {
	if err := Validate(text); err != nil {
		return c.State(), false, err
	}
	c.systemMu.Lock()
	defer c.systemMu.Unlock()
	operation, cancel := operationContext(ctx)
	defer cancel()
	writeErr := c.system.Write(operation, text)
	state, changed := c.apply(text)
	return state, changed, writeErr
}

// SetRemote accepts a device-originated value only while two-way sync is on.
func (c *Controller) SetRemote(ctx context.Context, text string) (State, bool, error) {
	if err := Validate(text); err != nil {
		return c.State(), false, err
	}
	if !c.State().Enabled {
		return c.State(), false, nil
	}
	c.systemMu.Lock()
	defer c.systemMu.Unlock()
	if !c.State().Enabled {
		return c.State(), false, nil
	}
	operation, cancel := operationContext(ctx)
	defer cancel()
	writeErr := c.system.Write(operation, text)
	if !c.State().Enabled {
		return c.State(), false, writeErr
	}
	state, changed := c.apply(text)
	return state, changed, writeErr
}

// Read returns the current OS clipboard, falling back to Surf's in-memory
// value on headless systems. It never reads or writes disk content.
func (c *Controller) Read(ctx context.Context) (State, error) {
	c.systemMu.Lock()
	defer c.systemMu.Unlock()
	operation, cancel := operationContext(ctx)
	defer cancel()
	text, err := c.system.Read(operation)
	if err == nil {
		if validateErr := Validate(text); validateErr != nil {
			return c.State(), validateErr
		}
		state, _ := c.apply(text)
		return state, nil
	}
	state := c.State()
	if state.Known {
		return state, nil
	}
	return state, err
}

func (c *Controller) Start(ctx context.Context, changed func(State)) {
	go func() {
		ticker := time.NewTicker(600 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state, didChange, err := c.Refresh(ctx)
				if err == nil && didChange && changed != nil {
					changed(state)
				}
			}
		}
	}()
}
