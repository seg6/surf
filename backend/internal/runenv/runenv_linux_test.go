//go:build linux

package runenv

import (
	"testing"

	"surf-backend/internal/config"
)

func TestDoctorChecksCommonToolsWhenPulseUnmanaged(t *testing.T) {
	checks := Doctor(&config.Config{
		ChromePath: "missing-chromium",
		FFmpegPath: "missing-ffmpeg",
	})
	if len(checks) != 2 {
		t.Fatalf("checks=%v, want chromium+ffmpeg only", checks)
	}
}

func TestDoctorRequiresPulseaudioAndPactlWhenManagingPulse(t *testing.T) {
	checks := Doctor(&config.Config{
		ChromePath:     "missing-chromium",
		FFmpegPath:     "missing-ffmpeg",
		PulseaudioPath: "missing-pulseaudio",
		PactlPath:      "missing-pactl",
		ManagePulse:    true,
	})
	for _, name := range []string{"pulseaudio", "pactl"} {
		found := false
		for _, check := range checks {
			if check.Name == name {
				found = true
				if !check.Required {
					t.Fatalf("%s should be required", name)
				}
			}
		}
		if !found {
			t.Fatalf("doctor did not include %s", name)
		}
	}
}

func TestDoctorRequiresPactlWhenEnsuringExternalSink(t *testing.T) {
	checks := Doctor(&config.Config{
		ChromePath:      "missing-chromium",
		FFmpegPath:      "missing-ffmpeg",
		PactlPath:       "missing-pactl",
		EnsurePulseSink: true,
	})
	found := false
	for _, check := range checks {
		if check.Name == "pactl" {
			found = true
			if !check.Required {
				t.Fatal("pactl should be required when creating an external sink")
			}
		}
	}
	if !found {
		t.Fatal("doctor did not include pactl")
	}
}
