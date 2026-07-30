package config

import (
	"path/filepath"
	"testing"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SURF_HOME", "BIND_ADDR", "PORT", "CHROME", "START_URL",
		"PROFILE", "VW", "VH",
		"AUTH_DAYS", "DOWNLOADS", "UPLOADS",
		"CHROME_NO_SANDBOX", "SURF_CHROME_GPU", "SURF_PASSWORD",
		"SURF_ADAPTIVE_VIDEO",
		"STREAM_SCALE", "STREAM_BITRATE",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("SURF_PASSWORD", "test-password")
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
	if cfg.StreamBitrateK != 12000 {
		t.Fatalf("StreamBitrateK=%d, want 12000", cfg.StreamBitrateK)
	}
}

func TestLoadRequiresSurfPassword(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SURF_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without SURF_PASSWORD")
	}
}

func TestLoadForDoctorDoesNotRequireSurfPassword(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SURF_PASSWORD", "")
	if _, err := LoadForDoctor(); err != nil {
		t.Fatal(err)
	}
}
