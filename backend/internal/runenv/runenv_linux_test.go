//go:build linux

package runenv

import (
	"testing"

	"surf-backend/internal/config"
)

func TestDisplaySocket(t *testing.T) {
	if got := displaySocket(":123.0"); got != "/tmp/.X11-unix/X123" {
		t.Fatalf("displaySocket=%q", got)
	}
}

func TestDoctorRequiresXrandr(t *testing.T) {
	checks := Doctor(&config.Config{
		ChromePath:     "missing-chromium",
		FFmpegPath:     "missing-ffmpeg",
		XvfbPath:       "missing-xvfb",
		XrandrPath:     "missing-xrandr",
		PulseaudioPath: "missing-pulseaudio",
		PactlPath:      "missing-pactl",
		ManageDisplay:  true,
		ManagePulse:    true,
	})
	found := false
	for _, check := range checks {
		if check.Name == "xrandr" {
			found = true
			if !check.Required {
				t.Fatal("xrandr should be required")
			}
		}
	}
	if !found {
		t.Fatal("doctor did not include xrandr")
	}
}

func TestDoctorRequiresPactlWhenEnsuringExternalSink(t *testing.T) {
	checks := Doctor(&config.Config{
		ChromePath:      "missing-chromium",
		FFmpegPath:      "missing-ffmpeg",
		XrandrPath:      "missing-xrandr",
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
