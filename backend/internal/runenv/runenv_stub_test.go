//go:build darwin

package runenv

import (
	"testing"

	"surf-backend/internal/config"
)

func TestStubPlatformDoctorChecksCommonToolsOnly(t *testing.T) {
	checks := Doctor(&config.Config{ChromePath: "missing-chromium", FFmpegPath: "missing-ffmpeg"})
	if len(checks) != 2 {
		t.Fatalf("checks=%v, want chromium+ffmpeg only", checks)
	}
}

func TestStubPlatformHasNoAudioLane(t *testing.T) {
	h, err := newPlatform().Prepare(&config.Config{})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer h.Shutdown()
	if h.AudioCaptureArgs("") != nil {
		t.Fatal("AudioCaptureArgs should be nil (PCM lane unsupported)")
	}
}
