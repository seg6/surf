// Package control publishes the private, per-process endpoint used by Surf's
// local CLI and desktop controller to manage a running server.
package control

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"surf-backend/internal/atomicfile"
)

const (
	Schema      = 1
	FileName    = "daemon.json"
	AdminHeader = "X-Surf-Admin"
)

var ErrNotRunning = errors.New("Surf server is not running")

// Descriptor is transient process state. AdminToken is deliberately kept in
// the permission-restricted descriptor rather than in command-line arguments
// or the environment, where it could be exposed by process inspection.
type Descriptor struct {
	Schema     int    `json:"schema"`
	PID        int    `json:"pid"`
	ControlURL string `json:"controlURL"`
	PublicPort int    `json:"publicPort"`
	ServerID   string `json:"serverID"`
	// Protocol is the legacy schema-1 JSON slot used to carry the ordered
	// compatibility generation. Keep its wire name so a new CLI can still
	// manage a backend started before this migration.
	Protocol   string    `json:"protocol"`
	StartedAt  time.Time `json:"startedAt"`
	AdminToken string    `json:"adminToken"`
}

// New creates a descriptor for one server lifetime.
func New(controlURL, serverID, compatibility string, publicPort int) (Descriptor, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Descriptor{}, fmt.Errorf("generate admin token: %w", err)
	}
	descriptor := Descriptor{
		Schema: Schema, PID: os.Getpid(), ControlURL: controlURL,
		PublicPort: publicPort, ServerID: serverID, Protocol: compatibility,
		StartedAt: time.Now().UTC(), AdminToken: base64.RawURLEncoding.EncodeToString(raw),
	}
	if err := descriptor.Validate(); err != nil {
		return Descriptor{}, err
	}
	return descriptor, nil
}

// Path returns the runtime descriptor path for a Surf home.
func Path(home string) string { return filepath.Join(home, FileName) }

// Validate rejects partial or untrusted descriptors before they are used to
// construct a TLS client.
func (d Descriptor) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("unsupported server control descriptor schema %d", d.Schema)
	}
	if d.PID <= 0 || d.PublicPort < 1 || d.PublicPort > 65535 {
		return errors.New("invalid server process or public port")
	}
	parsed, err := url.Parse(d.ControlURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return errors.New("invalid server control URL")
	}
	if ip := net.ParseIP(parsed.Hostname()); ip == nil || !ip.IsLoopback() {
		return errors.New("server control URL is not loopback")
	}
	serverID, err := hex.DecodeString(d.ServerID)
	if err != nil || len(serverID) != sha256.Size || strings.TrimSpace(d.Protocol) == "" {
		return errors.New("server identity or compatibility is missing")
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(d.AdminToken); err != nil || len(decoded) != 32 {
		return errors.New("invalid server admin token")
	}
	return nil
}

// Write atomically publishes a descriptor readable only by the Surf account.
func Write(home string, descriptor Descriptor) error {
	if strings.TrimSpace(home) == "" {
		return errors.New("SURF_HOME is empty")
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create Surf home: %w", err)
	}
	data, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := Path(home)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write server control descriptor: %w", err)
	}
	_ = os.Chmod(temporary, 0o600)
	if err := atomicfile.Replace(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish server control descriptor: %w", err)
	}
	return nil
}

// Load reads and validates the running server descriptor without creating any
// server identity or other persistent state.
func Load(home string) (Descriptor, error) {
	path := Path(home)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Descriptor{}, ErrNotRunning
	}
	if err != nil {
		return Descriptor{}, fmt.Errorf("read server control descriptor: %w", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			return Descriptor{}, fmt.Errorf("read server control descriptor: %w", err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return Descriptor{}, errors.New("read server control descriptor: permissions must be 0600")
		}
	}
	var descriptor Descriptor
	if err := json.Unmarshal(data, &descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("read server control descriptor: %w", err)
	}
	if err := descriptor.Validate(); err != nil {
		return Descriptor{}, fmt.Errorf("read server control descriptor: %w", err)
	}
	return descriptor, nil
}

// RemoveOwned removes only the descriptor from this server lifetime. This
// prevents an old process from deleting a descriptor published by a successor.
func RemoveOwned(home, adminToken string) error {
	descriptor, err := Load(home)
	if errors.Is(err, ErrNotRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	if descriptor.AdminToken != adminToken {
		return nil
	}
	if err := os.Remove(Path(home)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove server control descriptor: %w", err)
	}
	return nil
}
