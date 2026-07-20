// Package config reads all tunables from the environment, mirroring the
// original server.js env contract so existing deployments keep working.
package config

import (
	"os"
	"strconv"
)

// ClientVersion gates the WS handshake: the served client and the server must
// agree, so a stale cached client can never talk a mismatched protocol.
// 20260716-1: editable gained kind+rect (additive; bumped per docs/02 §4).
const ClientVersion = "20260716-1"

// NativeVersion gates the Objective-C client handshake separately from the web
// client so native-only protocol additions do not force web cache churn.
// 20260716-2: H.264 video lane (type-3 frames, video/video-config messages).
const NativeVersion = "20260716-2"

type Config struct {
	Port                int
	ChromePath          string
	StartURL            string
	Profile             string
	ViewW               int
	ViewH               int
	DisplayW            int // X framebuffer / Chromium window size; must cover every viewport
	DisplayH            int
	Quality             int     // steady-state screencast JPEG quality
	NativeQuality       int     // native-client screencast JPEG quality
	NativeMotionQuality int     // native-client JPEG quality during motion
	MotionQuality       int     // quality while scrolling/typing (cheap frames, low latency)
	MotionScale         float64 // resolution factor during motion; smaller = faster decode
	SharpQuality        int     // quality of the post-settle captureScreenshot
	SettleMS            int     // how long after the last input before we consider motion over
	AuthHash            string
	AuthDays            int
	Adblock             bool
	DownloadsDir        string

	// H.264 lane (docs/03 §2, §6). The encoder only runs while a native
	// video-mode client is subscribed.
	StreamFPS      int    // STREAM_FPS
	StreamScale    string // STREAM_SCALE, "960x720" to shrink; empty = VWxVH
	StreamBitrateK int    // STREAM_BITRATE
	StreamMaxrateK int    // STREAM_MAXRATE
	StreamBufsizeK int    // STREAM_BUFSIZE
	StreamPreset   string // STREAM_PRESET
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

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil && v > 0.2 && v <= 1 {
		return v
	}
	return def
}

func Load() *Config {
	viewW := envInt("VW", 1024)
	viewH := envInt("VH", 768)
	displayDefault := max(viewW, viewH)
	return &Config{
		Port:                envInt("PORT", 8080),
		ChromePath:          envStr("CHROME", "/usr/bin/chromium"),
		StartURL:            envStr("START_URL", "https://duckduckgo.com"),
		Profile:             envStr("PROFILE", "/data/profile"),
		ViewW:               viewW,
		ViewH:               viewH,
		DisplayW:            envInt("XFB_W", displayDefault),
		DisplayH:            envInt("XFB_H", displayDefault),
		Quality:             envInt("QUALITY", 60),
		NativeQuality:       envInt("NATIVE_QUALITY", 100),
		NativeMotionQuality: envInt("NATIVE_MOTION_QUALITY", 85),
		MotionQuality:       envInt("MOTION_QUALITY", 40),
		MotionScale:         envFloat("MOTION_SCALE", 0.5),
		SharpQuality:        envInt("SHARP_QUALITY", 82),
		SettleMS:            envInt("SETTLE_MS", 180),
		AuthHash:            envStr("AUTH_HASH", "$2a$14$U0M6mD3WVZ.GAYHuIKepPepFBSmm5zh.0JGhumvs6Fk6w8maxJcea"),
		AuthDays:            envInt("AUTH_DAYS", 180),
		Adblock:             os.Getenv("ADBLOCK") != "0",
		DownloadsDir:        envStr("DOWNLOADS", "/data/downloads"),
		StreamFPS:           envInt("STREAM_FPS", 24),
		StreamScale:         envStr("STREAM_SCALE", ""),
		StreamBitrateK:      envInt("STREAM_BITRATE", 2200),
		StreamMaxrateK:      envInt("STREAM_MAXRATE", 3000),
		StreamBufsizeK:      envInt("STREAM_BUFSIZE", 800),
		StreamPreset:        envStr("STREAM_PRESET", "superfast"),
	}
}
