package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SURF_RUNTIME", "SURF_HOME", "BIND_ADDR", "PORT", "CHROME", "START_URL",
		"PROFILE", "VW", "VH", "XFB_W", "XFB_H", "QUALITY", "MOTION_QUALITY",
		"SHARP_QUALITY", "SETTLE_MS", "AUTH_DAYS", "DOWNLOADS", "UPLOADS",
		"SURF_DISPLAY", "DISPLAY", "X_OUTPUT", "PULSE_SERVER", "PULSE_SINK",
		"AUDIO_SOURCE", "FFMPEG", "XVFB", "XRANDR", "PULSEAUDIO", "PACTL",
		"SURF_MANAGE_DISPLAY", "SURF_MANAGE_PULSE", "CHROME_NO_SANDBOX",
		"STREAM_FPS", "STREAM_SCALE", "STREAM_BITRATE", "STREAM_MAXRATE",
		"STREAM_BUFSIZE", "STREAM_PRESET",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("AUTH_HASH", "test-hash")
}

func TestLoadDockerDefaults(t *testing.T) {
	clearConfigEnv(t)
	cfg := Load()
	if cfg.RuntimeMode != "docker" {
		t.Fatalf("RuntimeMode=%q, want docker", cfg.RuntimeMode)
	}
	if cfg.Profile != "/data/profile" || cfg.DownloadsDir != "/data/downloads" || cfg.UploadsDir != "/data/uploads" {
		t.Fatalf("docker data dirs = %q %q %q", cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir)
	}
	if cfg.ChromePath != "/usr/bin/chromium" {
		t.Fatalf("ChromePath=%q", cfg.ChromePath)
	}
	if !cfg.ChromeNoSandbox || cfg.ManageDisplay || cfg.ManagePulse {
		t.Fatalf("docker flags noSandbox=%t manageDisplay=%t managePulse=%t", cfg.ChromeNoSandbox, cfg.ManageDisplay, cfg.ManagePulse)
	}
	if !reflect.DeepEqual(cfg.ChildEnv, []string{"DISPLAY=:99", "PULSE_SERVER=unix:/tmp/pulse/native", "PULSE_SINK=surf_output"}) {
		t.Fatalf("ChildEnv=%v", cfg.ChildEnv)
	}
}

func TestLoadHostDefaults(t *testing.T) {
	clearConfigEnv(t)
	home := t.TempDir()
	t.Setenv("SURF_RUNTIME", "host")
	t.Setenv("SURF_HOME", home)
	cfg := Load()
	if cfg.RuntimeMode != "host" {
		t.Fatalf("RuntimeMode=%q, want host", cfg.RuntimeMode)
	}
	if cfg.Profile != filepath.Join(home, "profile") || cfg.DownloadsDir != filepath.Join(home, "downloads") || cfg.UploadsDir != filepath.Join(home, "uploads") {
		t.Fatalf("host data dirs = %q %q %q", cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir)
	}
	if cfg.ChromePath != "chromium" {
		t.Fatalf("ChromePath=%q", cfg.ChromePath)
	}
	if cfg.ChromeNoSandbox || !cfg.ManageDisplay || !cfg.ManagePulse {
		t.Fatalf("host flags noSandbox=%t manageDisplay=%t managePulse=%t", cfg.ChromeNoSandbox, cfg.ManageDisplay, cfg.ManagePulse)
	}
	if cfg.EnsurePulseSink {
		t.Fatal("managed PulseAudio host mode should not also ensure an external sink")
	}
}

func TestHostExistingPulseDoesNotForcePulseServer(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SURF_RUNTIME", "host")
	t.Setenv("SURF_MANAGE_PULSE", "0")
	cfg := Load()
	if cfg.PulseServer != "" {
		t.Fatalf("PulseServer=%q, want empty", cfg.PulseServer)
	}
	if !cfg.EnsurePulseSink {
		t.Fatal("expected Surf to create a null sink on the existing Pulse/PipeWire server")
	}
	if !reflect.DeepEqual(cfg.ChildEnv, []string{"DISPLAY=:99", "PULSE_SINK=surf_output"}) {
		t.Fatalf("ChildEnv=%v", cfg.ChildEnv)
	}
}

func TestHostExistingPulseCanSkipSinkCreation(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SURF_RUNTIME", "host")
	t.Setenv("SURF_MANAGE_PULSE", "0")
	t.Setenv("SURF_ENSURE_PULSE_SINK", "0")
	cfg := Load()
	if cfg.EnsurePulseSink {
		t.Fatal("EnsurePulseSink=true")
	}
}

func TestRefreshChildEnv(t *testing.T) {
	cfg := &Config{Display: ":234", PulseServer: "unix:/tmp/surf/native"}
	cfg.RefreshChildEnv()
	if !reflect.DeepEqual(cfg.ChildEnv, []string{"DISPLAY=:234", "PULSE_SERVER=unix:/tmp/surf/native"}) {
		t.Fatalf("ChildEnv=%v", cfg.ChildEnv)
	}
}
