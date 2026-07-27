//go:build windows || darwin

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

func TestStubPlatformHasNoCaptureLanes(t *testing.T) {
	h, err := newPlatform().Prepare(&config.Config{})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer h.Shutdown()
	if h.ChromeArgs() != nil {
		t.Fatalf("ChromeArgs=%v, want nil", h.ChromeArgs())
	}
	if h.VideoCaptureArgs("", 0, 0, 0) != nil {
		t.Fatal("VideoCaptureArgs should be nil (H.264 lane unsupported)")
	}
	if h.AudioCaptureArgs("") != nil {
		t.Fatal("AudioCaptureArgs should be nil (PCM lane unsupported)")
	}
	if err := h.ResizeSurface(1024, 768); err != nil {
		t.Fatalf("ResizeSurface: %v", err)
	}
}
