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

func TestWindowsHandleUsesNativeAudioLane(t *testing.T) {
	h, err := newPlatform().Prepare(&config.Config{ChromePath: `C:\custom\chrome.exe`})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer h.Shutdown()
	if _, ok := h.(NativeAudioCapturer); !ok {
		t.Fatal("Windows handle does not implement NativeAudioCapturer")
	}
	aware, ok := h.(BrowserProcessAware)
	if !ok {
		t.Fatal("Windows handle does not implement BrowserProcessAware")
	}
	if _, err := h.(NativeAudioCapturer).OpenAudioCapture(); err == nil {
		t.Fatal("OpenAudioCapture succeeded without a Chromium PID")
	}
	aware.BrowserStarted(1234)
	if h.AudioCaptureArgs("") != nil {
		t.Fatal("Windows should not use FFmpeg capture arguments")
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
