package config

import (
	"path/filepath"
	"testing"
)

func TestCompatibilityGenerationAcceptsPublishedLegacyToken(t *testing.T) {
	original := CompatibilityVersion
	defer func() { CompatibilityVersion = original }()
	CompatibilityVersion = "1"
	if CompatibilityGeneration() != 1 || WireCompatibilityVersion() != "20260831-1" {
		t.Fatalf("generation=%d wire=%q", CompatibilityGeneration(), WireCompatibilityVersion())
	}
	for _, test := range []struct {
		value, legacy string
		want          int
		ok            bool
	}{
		{"1", "", 1, true},
		{"", "20260831-1", 1, true},
		{"2", "20260831-1", 2, true},
		{"", "unknown", 0, false},
	} {
		got, ok := ParseClientCompatibility(test.value, test.legacy)
		if got != test.want || ok != test.ok {
			t.Errorf("ParseClientCompatibility(%q, %q)=(%d,%t), want (%d,%t)",
				test.value, test.legacy, got, ok, test.want, test.ok)
		}
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SURF_HOME", "BIND_ADDR", "PORT", "CHROME", "START_URL",
		"PROFILE", "VW", "VH",
		"SURF_SERVER_NAME", "SURF_PUBLIC_ADDRESS", "DOWNLOADS", "UPLOADS",
		"SURF_TUNNEL_HOST",
		"CHROME_NO_SANDBOX",
		"SURF_ADAPTIVE_VIDEO",
		"STREAM_SCALE", "STREAM_BITRATE", "STREAM_QUANTIZER",
		"SURF_SEARCH_URL", "SURF_USER_AGENT", "SURF_VIEWPORT_SCALE", "SURF_AUDIO_QUALITY",
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

func TestLoadBrowserSettings(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SURF_HOME", t.TempDir())
	cfg := loadConfig(t)
	if cfg.SearchURL != "" || cfg.UserAgent != "" || cfg.ViewportScale != 1 {
		t.Fatalf("defaults: %q %q %v", cfg.SearchURL, cfg.UserAgent, cfg.ViewportScale)
	}
	t.Setenv("SURF_SEARCH_URL", " https://duckduckgo.com/?q=%s ")
	t.Setenv("SURF_USER_AGENT", " Custom/1.0 ")
	for value, want := range map[string]float64{"1.5": 1.5, "9": MaxViewportScale, "0.5": 1, "x": 1, "NaN": 1, "": 1} {
		t.Setenv("SURF_VIEWPORT_SCALE", value)
		cfg = loadConfig(t)
		if cfg.ViewportScale != want {
			t.Errorf("SURF_VIEWPORT_SCALE=%q -> %v, want %v", value, cfg.ViewportScale, want)
		}
	}
	if cfg.SearchURL != "https://duckduckgo.com/?q=%s" || cfg.UserAgent != "Custom/1.0" {
		t.Fatalf("trimmed: %q %q", cfg.SearchURL, cfg.UserAgent)
	}
}

func TestLoadAudioQuality(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SURF_HOME", t.TempDir())
	for value, want := range map[string]string{
		"": AudioQualityHigh, "high": AudioQualityHigh, "standard": AudioQualityStandard,
		" Standard ": AudioQualityStandard, "x": AudioQualityHigh,
	} {
		t.Setenv("SURF_AUDIO_QUALITY", value)
		if got := loadConfig(t).AudioQuality; got != want {
			t.Errorf("SURF_AUDIO_QUALITY=%q -> %q, want %q", value, got, want)
		}
	}
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
	if cfg.StreamBitrateK != 16000 {
		t.Fatalf("StreamBitrateK=%d, want 16000", cfg.StreamBitrateK)
	}
	if cfg.StreamQuantizer != 12 {
		t.Fatalf("StreamQuantizer=%d, want 12", cfg.StreamQuantizer)
	}
	if cfg.AdaptiveVideo {
		t.Fatal("AdaptiveVideo=true, want fixed native 60 FPS by default")
	}
}

func TestAdaptiveVideoCanBeEnabled(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SURF_ADAPTIVE_VIDEO", "1")
	if !loadConfig(t).AdaptiveVideo {
		t.Fatal("AdaptiveVideo=false after explicit enable")
	}
}

func TestLoadDoesNotRequirePassword(t *testing.T) {
	clearConfigEnv(t)
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
