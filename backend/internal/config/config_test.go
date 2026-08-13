package config

import (
	"path/filepath"
	"testing"
	"time"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SURF_HOME", "BIND_ADDR", "PORT", "CHROME", "START_URL",
		"PROFILE", "VW", "VH",
		"SURF_SERVER_NAME", "SURF_PUBLIC_ADDRESS", "DOWNLOADS", "UPLOADS",
		"SURF_TUNNEL_HOST",
		"CHROME_NO_SANDBOX",
		"SURF_ADAPTIVE_VIDEO",
		"SURF_BROWSER_IDLE_TIMEOUT",
		"STREAM_SCALE", "STREAM_BITRATE", "STREAM_QUANTIZER",
	} {
		t.Setenv(key, "")
	}
}

func loadConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)
	home := t.TempDir()
	t.Setenv("SURF_HOME", home)
	t.Setenv("CHROME_NO_SANDBOX", "0")
	cfg := loadConfig(t)
	if cfg.Port != 18080 {
		t.Fatalf("Port=%d, want 18080", cfg.Port)
	}
	if cfg.Profile != filepath.Join(home, "profile") || cfg.DownloadsDir != filepath.Join(home, "downloads") || cfg.UploadsDir != filepath.Join(home, "uploads") {
		t.Fatalf("data dirs = %q %q %q", cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir)
	}
	if cfg.ChromePath != "" {
		t.Fatalf("ChromePath=%q, want managed runtime selection", cfg.ChromePath)
	}
	if cfg.ChromeNoSandbox {
		t.Fatal("ChromeNoSandbox=true")
	}
	if cfg.StreamBitrateK != 48000 {
		t.Fatalf("StreamBitrateK=%d, want 48000", cfg.StreamBitrateK)
	}
	if cfg.StreamQuantizer != 12 {
		t.Fatalf("StreamQuantizer=%d, want 12", cfg.StreamQuantizer)
	}
	if cfg.BrowserIdleTimeout != 2*time.Minute {
		t.Fatalf("BrowserIdleTimeout=%s, want 2m", cfg.BrowserIdleTimeout)
	}
}

func TestBrowserIdleTimeout(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SURF_BROWSER_IDLE_TIMEOUT", "45s")
	if got := loadConfig(t).BrowserIdleTimeout; got != 45*time.Second {
		t.Fatalf("BrowserIdleTimeout=%s, want 45s", got)
	}
	t.Setenv("SURF_BROWSER_IDLE_TIMEOUT", "0")
	if got := loadConfig(t).BrowserIdleTimeout; got != 0 {
		t.Fatalf("BrowserIdleTimeout=%s, want 0", got)
	}
	t.Setenv("SURF_BROWSER_IDLE_TIMEOUT", "nonsense")
	if got := loadConfig(t).BrowserIdleTimeout; got != 2*time.Minute {
		t.Fatalf("invalid BrowserIdleTimeout=%s, want default", got)
	}
}

func TestLoadDoesNotRequirePassword(t *testing.T) {
	clearConfigEnv(t)
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
