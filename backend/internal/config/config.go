// Package config reads all tunables from the environment.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// NativeVersion gates the WS handshake: the Surf app and the server must agree,
// so a stale client can never talk a mismatched protocol. Release builds inject
// this from PROTOCOL_VERSION with Go linker flags.
var NativeVersion = "dev"

// AppVersion is injected from VERSION in release builds.
var AppVersion = "dev"

// Caps enumerates optional server capabilities for /native-config. The client
// feature-gates on these instead of parsing version strings, so server and app
// can ship independently once both understand a capability.
var Caps = []string{
	"dialog",      // JS dialog forwarding + dialogreply
	"filechooser", // upload intercept + POST /upload
	"linkinfo",    // hit message -> linkinfo reply
	"history2",    // paginated history query + histdel/clear
	"reader",      // reader-mode extraction
	"security",    // TLS state on url messages
	"pageerror",   // native error surface messages
	"clock",       // NTP-style monotonic clock synchronization
	"dlprogress",  // download progress events
	"dldel",       // delete a download from the server
	"reqkeyframe", // on-demand IDR request (decode error / resync)
	"video-retry", // explicit retry after an unavailable encoder state
	"media-stats", // client decode/presentation health for adaptive profiles
}

type Config struct {
	SurfHome     string
	BindAddr     string
	Port         int
	ChromePath   string
	StartURL     string
	Profile      string
	ViewW        int
	ViewH        int
	SurfPassword string
	AuthDays     int
	DownloadsDir string
	UploadsDir   string

	ChromeNoSandbox    bool
	ChromeGPU          bool
	ContentBlocker     bool
	ContentBlockerPath string
	AdaptiveVideo      bool

	// H.264 lane. The encoder only runs while a native client is subscribed.
	StreamScale    string // STREAM_SCALE, optional maximum; empty = client size
	StreamBitrateK int    // STREAM_BITRATE
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func surfHome() string {
	if v := os.Getenv("SURF_HOME"); v != "" {
		return v
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".surf")
	}
	return ".surf"
}

func Load() (*Config, error) {
	return load(true)
}

func LoadForDoctor() (*Config, error) {
	return load(false)
}

func load(requireAuth bool) (*Config, error) {
	home := surfHome()
	viewW := envInt("VW", 768)
	viewH := envInt("VH", 934)
	cfg := &Config{
		SurfHome:        home,
		BindAddr:        envStr("BIND_ADDR", "0.0.0.0"),
		Port:            envInt("PORT", 18080),
		ChromePath:      os.Getenv("CHROME"),
		StartURL:        envStr("START_URL", "https://www.google.com"),
		Profile:         envStr("PROFILE", filepath.Join(home, "profile")),
		ViewW:           viewW,
		ViewH:           viewH,
		SurfPassword:    os.Getenv("SURF_PASSWORD"),
		AuthDays:        envInt("AUTH_DAYS", 180),
		DownloadsDir:    envStr("DOWNLOADS", filepath.Join(home, "downloads")),
		UploadsDir:      envStr("UPLOADS", filepath.Join(home, "uploads")),
		ChromeNoSandbox: envBool("CHROME_NO_SANDBOX", os.Geteuid() == 0),
		ChromeGPU:       envBool("SURF_CHROME_GPU", true),
		ContentBlocker:  envBool("SURF_CONTENT_BLOCKER", true),
		AdaptiveVideo:   envBool("SURF_ADAPTIVE_VIDEO", false),
		StreamScale:     envStr("STREAM_SCALE", ""),
		StreamBitrateK:  envInt("STREAM_BITRATE", 6000),
	}
	if requireAuth && cfg.SurfPassword == "" {
		return nil, fmt.Errorf("SURF_PASSWORD is required")
	}
	return cfg, nil
}
