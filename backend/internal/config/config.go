// Package config reads all tunables from the environment.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CompatibilityVersion is an ordered generation shared by the Surf backend
// and native client. Release versions may differ while this value remains the
// same; increment it only when the older side must not connect.
//
// Release builds inject this from COMPATIBILITY_VERSION with Go linker flags.
var CompatibilityVersion = "1"

// AppVersion is injected from VERSION in release builds.
var AppVersion = "dev"

// The published 0.15.4 client predates the ordered compatibility field and
// identifies generation 1 with this opaque native-protocol token. Keep the
// alias on the wire while generation 1 is supported so old and new builds can
// connect in both directions.
const legacyCompatibilityOne = "20260831-1"

func CompatibilityGeneration() int {
	value, err := strconv.Atoi(CompatibilityVersion)
	if err != nil || value < 1 {
		return 0
	}
	return value
}

func WireCompatibilityVersion() string {
	if CompatibilityGeneration() == 1 {
		return legacyCompatibilityOne
	}
	return CompatibilityVersion
}

// ParseClientCompatibility accepts the ordered generation used by new clients
// and the one legacy token shipped in Surf 0.15.4.
func ParseClientCompatibility(value, legacy string) (int, bool) {
	if value != "" {
		generation, err := strconv.Atoi(value)
		return generation, err == nil && generation > 0
	}
	if legacy == legacyCompatibilityOne {
		return 1, true
	}
	if legacy != "" {
		generation, err := strconv.Atoi(legacy)
		return generation, err == nil && generation > 0
	}
	return 0, false
}

// Caps enumerates optional server capabilities for /api/v1/config. The client
// feature-gates on these instead of parsing version strings, so server and app
// can ship independently once both understand a capability.
var Caps = []string{
	"dialog",         // JS dialog forwarding + dialogreply
	"filechooser",    // upload intercept + POST /api/v1/uploads
	"history2",       // paginated history query + histdel/clear
	"reader",         // reader-mode extraction
	"security",       // TLS state on url messages
	"pageerror",      // native error surface messages
	"clock",          // NTP-style monotonic clock synchronization
	"dlprogress",     // download progress events
	"dldel",          // delete a download from the server
	"reqkeyframe",    // on-demand IDR request (decode error / resync)
	"video-retry",    // explicit retry after an unavailable encoder state
	"media-stats",    // client decode/presentation health for adaptive profiles
	"live-logs",      // authenticated native log records mirrored live to the host
	"clipboard-sync", // owner-controlled bidirectional system clipboard bridge
}

type Config struct {
	SurfHome      string
	BindAddr      string
	Port          int
	ChromePath    string
	StartURL      string
	Profile       string
	ViewW         int
	ViewH         int
	ServerName    string
	PublicAddress string
	TunnelHost    string
	DownloadsDir  string
	UploadsDir    string

	ChromeNoSandbox    bool
	ContentBlocker     bool
	ContentBlockerPath string
	AdaptiveVideo      bool

	// H.264 lane. The encoder only runs while a native client is subscribed.
	StreamScale     string // STREAM_SCALE, optional maximum; empty = client size
	StreamBitrateK  int    // STREAM_BITRATE, primary variable-rate H.264 target
	StreamQuantizer int    // STREAM_QUANTIZER, fallback H.264 QP 0..51; lower is sharper
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
	return load()
}

func LoadForDoctor() (*Config, error) {
	return load()
}

func load() (*Config, error) {
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
		ServerName:      envStr("SURF_SERVER_NAME", "Surf"),
		PublicAddress:   os.Getenv("SURF_PUBLIC_ADDRESS"),
		TunnelHost:      strings.ToLower(strings.TrimSpace(os.Getenv("SURF_TUNNEL_HOST"))),
		DownloadsDir:    envStr("DOWNLOADS", filepath.Join(home, "downloads")),
		UploadsDir:      envStr("UPLOADS", filepath.Join(home, "uploads")),
		ChromeNoSandbox: envBool("CHROME_NO_SANDBOX", os.Geteuid() == 0),
		ContentBlocker:  envBool("SURF_CONTENT_BLOCKER", true),
		AdaptiveVideo:   envBool("SURF_ADAPTIVE_VIDEO", false),
		StreamScale:     envStr("STREAM_SCALE", ""),
		StreamBitrateK:  envInt("STREAM_BITRATE", 16000),
		StreamQuantizer: envInt("STREAM_QUANTIZER", 12),
	}
	return cfg, nil
}
