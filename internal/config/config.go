// Package config reads all tunables from the environment, mirroring the
// original server.js env contract so existing deployments keep working.
package config

import (
	"os"
	"strconv"
)

// ClientVersion gates the WS handshake: the served client and the server must
// agree, so a stale cached client can never talk a mismatched protocol.
const ClientVersion = "20260715-5"

// NativeVersion gates the Objective-C client handshake separately from the web
// client so native-only protocol additions do not force web cache churn.
const NativeVersion = "20260716-1"

type Config struct {
	Port                int
	ChromePath          string
	StartURL            string
	Profile             string
	ViewW               int
	ViewH               int
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
	return &Config{
		Port:                envInt("PORT", 8080),
		ChromePath:          envStr("CHROME", "/usr/bin/chromium"),
		StartURL:            envStr("START_URL", "https://duckduckgo.com"),
		Profile:             envStr("PROFILE", "/data/profile"),
		ViewW:               envInt("VW", 1024),
		ViewH:               envInt("VH", 768),
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
	}
}
