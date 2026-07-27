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

// TestWindowsVideoCaptureArgsDisabledOnHiddenDesktop guards a real, live-
// confirmed finding: gdigrab cannot capture a desktop that was never made
// the foreground one (ERROR_ACCESS_DENIED, independent of the desktop's
// DACL — confirmed by switching it to foreground, which made capture work
// immediately). So the H.264 lane must stay off whenever a hidden desktop
// is active; only the JPEG/CDP screencast lane works there.
func TestWindowsVideoCaptureArgsDisabledOnHiddenDesktop(t *testing.T) {
	h := &windowsHandle{desktopName: `WinSta0\SurfHidden`}
	if args := h.VideoCaptureArgs("", 1024, 768, 30); args != nil {
		t.Fatalf("VideoCaptureArgs=%v, want nil when a hidden desktop is active", args)
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

// TestCreateHiddenDesktop is intentionally soft for the same reason: it's a
// real Win32 call whose success can depend on the host environment. This
// never calls SwitchDesktop or anything else that could affect what's
// actually displayed — CreateDesktopW alone has no visible effect at all.
func TestCreateHiddenDesktop(t *testing.T) {
	name, handle, err := createHiddenDesktop("SurfRunenvTest")
	if err != nil {
		t.Logf("createHiddenDesktop failed in this environment (best-effort, not a failure): %v", err)
		return
	}
	defer closeDesktop(handle)
	if name != `WinSta0\SurfRunenvTest` {
		t.Fatalf("name=%q, want WinSta0-qualified", name)
	}
}

func TestPrepareSetsHiddenDesktopWhenEnabled(t *testing.T) {
	h, err := newPlatform().Prepare(&config.Config{ChromePath: `C:\custom\chrome.exe`, HiddenDesktop: true})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer h.Shutdown()
	// Soft: only assert consistency, since desktop creation itself is
	// best-effort and may fail in a restricted environment.
	if h.HiddenDesktop() != "" && h.VideoCaptureArgs("", 1024, 768, 30) != nil {
		t.Fatal("H.264 lane should be disabled whenever a hidden desktop is active")
	}
}

func TestPrepareSkipsHiddenDesktopWhenDisabled(t *testing.T) {
	h, err := newPlatform().Prepare(&config.Config{ChromePath: `C:\custom\chrome.exe`, HiddenDesktop: false})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer h.Shutdown()
	if h.HiddenDesktop() != "" {
		t.Fatalf("HiddenDesktop=%q, want empty when config.HiddenDesktop is false", h.HiddenDesktop())
	}
	if h.VideoCaptureArgs("", 1024, 768, 30) == nil {
		t.Fatal("H.264 lane should still work when hidden-desktop mode is off")
	}
}
