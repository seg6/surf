//go:build windows

package runenv

import (
	"testing"

	"golang.org/x/sys/windows"

	"surf-backend/internal/config"
)

func TestResolveChromePathPassesThroughExplicitPath(t *testing.T) {
	if got := resolveChromePath(`C:\custom\chrome.exe`); got != `C:\custom\chrome.exe` {
		t.Fatalf("resolveChromePath=%q, want passthrough", got)
	}
}

func TestWindowsDoctorChecksCommonToolsOnly(t *testing.T) {
	checks := Doctor(&config.Config{ChromePath: `C:\missing\chrome.exe`, FFmpegPath: "missing-ffmpeg"})
	if len(checks) != 2 {
		t.Fatalf("checks=%v, want chromium+ffmpeg only", checks)
	}
}

func TestWindowsHandleHasNoOzoneFlagsOrAudioLane(t *testing.T) {
	h, err := newPlatform().Prepare(&config.Config{ChromePath: `C:\custom\chrome.exe`})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer h.Shutdown()
	if h.ChromeArgs() != nil {
		t.Fatalf("ChromeArgs=%v, want nil", h.ChromeArgs())
	}
	if h.AudioCaptureArgs("") != nil {
		t.Fatal("AudioCaptureArgs should be nil (PCM lane unsupported)")
	}
	if err := h.ResizeSurface(1024, 768); err != nil {
		t.Fatalf("ResizeSurface: %v", err)
	}
}

func TestWindowsVideoCaptureArgsUsesGdigrabAtOrigin(t *testing.T) {
	h := &windowsHandle{}
	args := h.VideoCaptureArgs("", 1024, 768, 30)
	want := []string{
		"-loglevel", "warning",
		"-f", "gdigrab",
		"-framerate", "30",
		"-offset_x", "0", "-offset_y", "0",
		"-video_size", "1024x768",
		"-i", "desktop",
	}
	if len(args) != len(want) {
		t.Fatalf("args=%v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d]=%q, want %q (full: %v)", i, args[i], want[i], args)
		}
	}
}

// TestCreateKillOnCloseJobBestEffort is intentionally soft: whether a Job
// Object can be created/assigned depends on the host's own process/job
// nesting (a sandbox or CI runner may already restrict this), and
// createKillOnCloseJob is documented as best-effort for exactly that reason.
func TestCreateKillOnCloseJobBestEffort(t *testing.T) {
	job := createKillOnCloseJob()
	if job == 0 {
		t.Log("createKillOnCloseJob returned 0 in this environment (best-effort, not a failure)")
		return
	}
	defer windows.CloseHandle(job)
}
