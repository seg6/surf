package config

import (
	"path/filepath"
	"testing"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SURF_HOME", "BIND_ADDR", "PORT", "CHROME", "START_URL",
		"PROFILE", "VW", "VH", "SOURCE_JPEG_QUALITY",
		"AUTH_DAYS", "DOWNLOADS", "UPLOADS",
		"PULSE_SERVER", "PULSE_SINK",
		"AUDIO_SOURCE", "FFMPEG", "PULSEAUDIO", "PACTL",
		"SURF_MANAGE_PULSE", "SURF_ENSURE_PULSE_SINK",
		"CHROME_NO_SANDBOX", "SURF_PASSWORD",
		"STREAM_FPS", "STREAM_SCALE", "STREAM_ENCODER", "STREAM_BITRATE", "STREAM_MAXRATE",
		"STREAM_BUFSIZE",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("SURF_PASSWORD", "test-password")
}

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, item := range env {
		for i, r := range item {
			if r == '=' {
				m[item[:i]] = item[i+1:]
				break
			}
		}
	}
	return m
}

func requireChildEnv(t *testing.T, cfg *Config, want map[string]string) {
	t.Helper()
	got := envMap(cfg.ChildEnv)
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("ChildEnv[%s]=%q, want %q in %v", k, got[k], v, cfg.ChildEnv)
		}
	}
}

func setPulseServerAvailable(t *testing.T, available bool) {
	t.Helper()
	old := pulseServerAvailable
	pulseServerAvailable = func(string) bool { return available }
	t.Cleanup(func() { pulseServerAvailable = old })
}

func loadConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestLoadDefaultsStartPrivatePulseWhenNoServerIsAvailable(t *testing.T) {
	clearConfigEnv(t)
	setPulseServerAvailable(t, false)
	home := t.TempDir()
	t.Setenv("SURF_HOME", home)
	t.Setenv("CHROME_NO_SANDBOX", "0")
	cfg := loadConfig(t)
	if cfg.Port != 18080 {
		t.Fatalf("Port=%d, want 18080", cfg.Port)
	}
	if cfg.SourceJPEGQuality != 100 {
		t.Fatalf("SourceJPEGQuality=%d, want 100", cfg.SourceJPEGQuality)
	}
	if cfg.Profile != filepath.Join(home, "profile") || cfg.DownloadsDir != filepath.Join(home, "downloads") || cfg.UploadsDir != filepath.Join(home, "uploads") {
		t.Fatalf("data dirs = %q %q %q", cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir)
	}
	if cfg.ChromePath != "" {
		t.Fatalf("ChromePath=%q, want managed runtime selection", cfg.ChromePath)
	}
	if cfg.FFmpegPath != "" {
		t.Fatalf("FFmpegPath=%q, want managed runtime selection", cfg.FFmpegPath)
	}
	if cfg.ChromeNoSandbox || !cfg.ManagePulse || cfg.EnsurePulseSink {
		t.Fatalf("flags noSandbox=%t managePulse=%t ensurePulseSink=%t", cfg.ChromeNoSandbox, cfg.ManagePulse, cfg.EnsurePulseSink)
	}
	requireChildEnv(t, cfg, map[string]string{"PULSE_SERVER": "unix:/tmp/pulse/native", "PULSE_SINK": "surf_output"})
}

func TestLoadUsesExistingPulseServerByDefault(t *testing.T) {
	clearConfigEnv(t)
	setPulseServerAvailable(t, true)
	home := t.TempDir()
	t.Setenv("SURF_HOME", home)
	cfg := loadConfig(t)
	if cfg.PulseServer != "" {
		t.Fatalf("PulseServer=%q, want empty", cfg.PulseServer)
	}
	if cfg.ManagePulse || !cfg.EnsurePulseSink {
		t.Fatalf("pulse flags managePulse=%t ensurePulseSink=%t", cfg.ManagePulse, cfg.EnsurePulseSink)
	}
	requireChildEnv(t, cfg, map[string]string{"PULSE_SINK": "surf_output"})
	if cfg.Profile != filepath.Join(home, "profile") {
		t.Fatalf("Profile=%q", cfg.Profile)
	}
}

func TestManagedPulseCanBeForced(t *testing.T) {
	clearConfigEnv(t)
	setPulseServerAvailable(t, true)
	t.Setenv("SURF_MANAGE_PULSE", "1")
	cfg := loadConfig(t)
	if !cfg.ManagePulse || cfg.EnsurePulseSink {
		t.Fatalf("pulse flags managePulse=%t ensurePulseSink=%t", cfg.ManagePulse, cfg.EnsurePulseSink)
	}
	if cfg.PulseServer != "unix:/tmp/pulse/native" {
		t.Fatalf("PulseServer=%q", cfg.PulseServer)
	}
}

func TestPulseServerEnvUsesExistingPulseByDefault(t *testing.T) {
	clearConfigEnv(t)
	setPulseServerAvailable(t, false)
	t.Setenv("PULSE_SERVER", "unix:/tmp/custom-pulse")
	cfg := loadConfig(t)
	if cfg.ManagePulse || !cfg.EnsurePulseSink {
		t.Fatalf("pulse flags managePulse=%t ensurePulseSink=%t", cfg.ManagePulse, cfg.EnsurePulseSink)
	}
	if cfg.PulseServer != "unix:/tmp/custom-pulse" {
		t.Fatalf("PulseServer=%q", cfg.PulseServer)
	}
}

func TestExistingPulseCanSkipSinkCreation(t *testing.T) {
	clearConfigEnv(t)
	setPulseServerAvailable(t, true)
	t.Setenv("SURF_MANAGE_PULSE", "0")
	t.Setenv("SURF_ENSURE_PULSE_SINK", "0")
	cfg := loadConfig(t)
	if cfg.EnsurePulseSink {
		t.Fatal("EnsurePulseSink=true")
	}
}

func TestLoadRequiresSurfPassword(t *testing.T) {
	clearConfigEnv(t)
	setPulseServerAvailable(t, false)
	t.Setenv("SURF_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without SURF_PASSWORD")
	}
}

func TestLoadForDoctorDoesNotRequireSurfPassword(t *testing.T) {
	clearConfigEnv(t)
	setPulseServerAvailable(t, false)
	t.Setenv("SURF_PASSWORD", "")
	if _, err := LoadForDoctor(); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshChildEnv(t *testing.T) {
	cfg := &Config{PulseServer: "unix:/tmp/surf/native", PulseSink: "surf_output"}
	cfg.RefreshChildEnv()
	requireChildEnv(t, cfg, map[string]string{"PULSE_SERVER": "unix:/tmp/surf/native", "PULSE_SINK": "surf_output"})
}
