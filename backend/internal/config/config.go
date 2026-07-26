// Package config reads all tunables from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// NativeVersion gates the WS handshake: the Surf app and the server must agree,
// so a stale client can never talk a mismatched protocol. Release builds inject
// this from PROTOCOL_VERSION with Go linker flags.
var NativeVersion = "dev"

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
	"lat",         // latency echo
	"dlprogress",  // download progress events
	"dldel",       // delete a download from the server
	"reqkeyframe", // on-demand IDR request (decode error / resync)
}

type Config struct {
	Port          int
	ChromePath    string
	StartURL      string
	Profile       string
	ViewW         int
	ViewH         int
	DisplayW      int // X framebuffer / Chromium window size; must cover every viewport
	DisplayH      int
	Quality       int // steady-state screencast JPEG quality
	MotionQuality int // JPEG quality while scrolling/typing (cheap frames, low latency)
	SharpQuality  int // quality of the post-settle captureScreenshot
	SettleMS      int // how long after the last input before we consider motion over
	AuthHash      string
	AuthDays      int
	DownloadsDir  string
	UploadsDir    string

	// H.264 lane. The encoder only runs while a native video-mode client
	// is subscribed.
	StreamFPS      int    // STREAM_FPS
	StreamScale    string // STREAM_SCALE, "960x720" to shrink; empty = VWxVH
	StreamBitrateK int    // STREAM_BITRATE
	StreamMaxrateK int    // STREAM_MAXRATE
	StreamBufsizeK int    // STREAM_BUFSIZE
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

func authHash() string {
	return os.Getenv("AUTH_HASH")
}

func Load() (*Config, error) {
	viewW := envInt("VW", 1024)
	viewH := envInt("VH", 768)
	displayDefault := max(viewW, viewH)
	cfg := &Config{
		Port:           envInt("PORT", 8080),
		ChromePath:     envStr("CHROME", "/usr/bin/chromium"),
		StartURL:       envStr("START_URL", "https://www.google.com"),
		Profile:        envStr("PROFILE", "/data/profile"),
		ViewW:          viewW,
		ViewH:          viewH,
		DisplayW:       envInt("XFB_W", displayDefault),
		DisplayH:       envInt("XFB_H", displayDefault),
		Quality:        envInt("QUALITY", 100),
		MotionQuality:  envInt("MOTION_QUALITY", 85),
		SharpQuality:   envInt("SHARP_QUALITY", 82),
		SettleMS:       envInt("SETTLE_MS", 180),
		AuthHash:       authHash(),
		AuthDays:       envInt("AUTH_DAYS", 180),
		DownloadsDir:   envStr("DOWNLOADS", "/data/downloads"),
		UploadsDir:     envStr("UPLOADS", "/data/uploads"),
		StreamFPS:      envInt("STREAM_FPS", 30),
		StreamScale:    envStr("STREAM_SCALE", "800x800"),
		StreamBitrateK: envInt("STREAM_BITRATE", 2800),
		StreamMaxrateK: envInt("STREAM_MAXRATE", 3600),
		StreamBufsizeK: envInt("STREAM_BUFSIZE", 900),
	}
	if cfg.AuthHash == "" {
		return nil, fmt.Errorf("AUTH_HASH is required; generate one with surf-backend -hash-password")
	}
	return cfg, nil
}
