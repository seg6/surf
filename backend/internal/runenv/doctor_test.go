package runenv

import (
	"testing"

	"surf-backend/internal/config"
)

func TestDoctorMarksXrandrOptional(t *testing.T) {
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
			if check.Required {
				t.Fatal("xrandr should be optional")
			}
		}
	}
	if !found {
		t.Fatal("doctor did not include xrandr")
	}
}
