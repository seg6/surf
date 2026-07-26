// Package config reads all tunables from the environment.
package config

import (
	"fmt"
	"os"
	"os/exec"
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
	"lat",         // latency echo
	"dlprogress",  // download progress events
	"dldel",       // delete a download from the server
	"reqkeyframe", // on-demand IDR request (decode error / resync)
}

type Config struct {
	BindAddr      string
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
	SurfPassword  string
	AuthDays      int
	DownloadsDir  string
	UploadsDir    string

	Display         string
	XOutput         string
	PulseServer     string
	PulseSink       string
	AudioSource     string
	FFmpegPath      string
	XvfbPath        string
	XrandrPath      string
	PulseaudioPath  string
	PactlPath       string
	ManageDisplay   bool
	ManagePulse     bool
	EnsurePulseSink bool
	ChromeNoSandbox bool
	ChildEnv        []string

	// H.264 lane. The encoder only runs while a native video-mode client
	// is subscribed.
	StreamFPS      int    // STREAM_FPS
	StreamScale    string // STREAM_SCALE, "960x720" to shrink; empty = VWxVH
	StreamBitrateK int    // STREAM_BITRATE
	StreamMaxrateK int    // STREAM_MAXRATE
	StreamBufsizeK int    // STREAM_BUFSIZE
}

func (c *Config) RefreshChildEnv() {
	env := []string{
		"DISPLAY=" + c.Display,
		"WAYLAND_DISPLAY=",
		"XDG_SESSION_TYPE=x11",
		"GDK_BACKEND=x11",
		"QT_QPA_PLATFORM=xcb",
		"SDL_VIDEODRIVER=x11",
		"CLUTTER_BACKEND=x11",
	}
	if c.PulseServer != "" {
		env = append(env, "PULSE_SERVER="+c.PulseServer)
	}
	if c.PulseSink != "" {
		env = append(env, "PULSE_SINK="+c.PulseSink)
	}
	c.ChildEnv = env
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

var pulseServerAvailable = func(pactlPath string) bool {
	cmd := exec.Command(pactlPath, "info")
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}

func Load() (*Config, error) {
	return load(true)
}

func LoadForDiagnostics() (*Config, error) {
	return load(false)
}

func load(requireAuth bool) (*Config, error) {
	home := surfHome()
	viewW := envInt("VW", 1024)
	viewH := envInt("VH", 768)
	displayDefault := max(viewW, viewH)
	pulseSink := envStr("PULSE_SINK", "surf_output")
	pactlPath := envStr("PACTL", "pactl")
	managePulseDefault := true
	if os.Getenv("PULSE_SERVER") != "" || (os.Getenv("SURF_MANAGE_PULSE") == "" && pulseServerAvailable(pactlPath)) {
		managePulseDefault = false
	}
	managePulse := envBool("SURF_MANAGE_PULSE", managePulseDefault)
	pulseServerDefault := ""
	if managePulse {
		pulseServerDefault = "unix:/tmp/pulse/native"
	}
	cfg := &Config{
		BindAddr:        envStr("BIND_ADDR", "0.0.0.0"),
		Port:            envInt("PORT", 18080),
		ChromePath:      envStr("CHROME", "chromium"),
		StartURL:        envStr("START_URL", "https://www.google.com"),
		Profile:         envStr("PROFILE", filepath.Join(home, "profile")),
		ViewW:           viewW,
		ViewH:           viewH,
		DisplayW:        envInt("XFB_W", displayDefault),
		DisplayH:        envInt("XFB_H", displayDefault),
		Quality:         envInt("QUALITY", 100),
		MotionQuality:   envInt("MOTION_QUALITY", 85),
		SharpQuality:    envInt("SHARP_QUALITY", 82),
		SettleMS:        envInt("SETTLE_MS", 180),
		SurfPassword:    os.Getenv("SURF_PASSWORD"),
		AuthDays:        envInt("AUTH_DAYS", 180),
		DownloadsDir:    envStr("DOWNLOADS", filepath.Join(home, "downloads")),
		UploadsDir:      envStr("UPLOADS", filepath.Join(home, "uploads")),
		Display:         envStr("SURF_DISPLAY", envStr("DISPLAY", ":99")),
		XOutput:         envStr("X_OUTPUT", "screen"),
		PulseServer:     envStr("PULSE_SERVER", pulseServerDefault),
		PulseSink:       pulseSink,
		AudioSource:     envStr("AUDIO_SOURCE", pulseSink+".monitor"),
		FFmpegPath:      envStr("FFMPEG", "ffmpeg"),
		XvfbPath:        envStr("XVFB", "Xvfb"),
		XrandrPath:      envStr("XRANDR", "xrandr"),
		PulseaudioPath:  envStr("PULSEAUDIO", "pulseaudio"),
		PactlPath:       pactlPath,
		ManageDisplay:   envBool("SURF_MANAGE_DISPLAY", true),
		ManagePulse:     managePulse,
		EnsurePulseSink: envBool("SURF_ENSURE_PULSE_SINK", !managePulse),
		ChromeNoSandbox: envBool("CHROME_NO_SANDBOX", os.Geteuid() == 0),
		StreamFPS:       envInt("STREAM_FPS", 30),
		StreamScale:     envStr("STREAM_SCALE", "800x800"),
		StreamBitrateK:  envInt("STREAM_BITRATE", 2800),
		StreamMaxrateK:  envInt("STREAM_MAXRATE", 3600),
		StreamBufsizeK:  envInt("STREAM_BUFSIZE", 900),
	}
	if requireAuth && cfg.SurfPassword == "" {
		return nil, fmt.Errorf("SURF_PASSWORD is required")
	}
	cfg.RefreshChildEnv()
	return cfg, nil
}
