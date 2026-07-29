//go:build windows

// Package runenv's Windows platform resolves a real Chromium/Edge path
// (Windows has no "chromium" PATH convention) and sets up a Job Object so
// the whole Chromium/ffmpeg process tree is killed automatically if
// Surf itself is force-killed and never gets to run its own
// cleanup.
//
// Chromium runs headless (see internal/cdp) and the H.264 lane transcodes
// CDP's own screencast frames instead of grabbing the screen, so there's no
// display/desktop bring-up needed here at all — an earlier version of this
// file created a hidden desktop and drove Chromium onto it via
// STARTUPINFO.lpDesktop specifically to keep a headful Chromium invisible;
// headless mode makes that unnecessary (confirmed live: no window is ever
// created at all, not merely hidden) and removes a real, demonstrated risk
// (SwitchDesktop-adjacent Win32 desktop APIs) along with it.
//
// The PCM lane uses Windows process-loopback WASAPI capture. Unlike endpoint
// loopback it follows Chromium's root PID and descendants across output
// devices without recording unrelated host audio.
package runenv

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"surf-backend/internal/config"
	"surf-backend/internal/winaudio"
)

func newPlatform() Platform { return windowsPlatform{} }

type windowsPlatform struct{}

func (windowsPlatform) Doctor(cfg *config.Config) []Check {
	return []Check{
		checkTool("chromium", resolveChromePath(cfg.ChromePath), true),
		checkTool("ffmpeg", cfg.FFmpegPath, true),
	}
}

func (windowsPlatform) Prepare(cfg *config.Config) (Handle, error) {
	cfg.ChromePath = resolveChromePath(cfg.ChromePath)
	return &windowsHandle{job: createKillOnCloseJob()}, nil
}

// createKillOnCloseJob puts our own process into a Job Object configured to
// kill every member process the instant the job's last handle closes — an
// OS-level safety net for when Surf itself is force-killed and
// never gets to run its own cleanup at all. Confirmed live: a forced kill of
// Surf's own process left a dozen orphaned Chromium helper
// processes running even with proc.Kill's taskkill /T fix, because that fix
// never ran — nothing in Go ever executes if the process is torn down from
// outside. Every child process this program spawns inherits membership in
// the job by default (Windows only excludes children launched with
// CREATE_BREAKAWAY_FROM_JOB, which nothing here ever passes), so no
// per-child bookkeeping is needed once this runs once at startup.
//
// Best effort: if any step fails (e.g. we're already nested in a job that
// forbids it) this logs and returns 0 — proc.Kill's taskkill /T still
// covers every explicit shutdown path either way.
func createKillOnCloseJob() windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		log.Printf("runtime: CreateJobObject failed, no kill-on-crash safety net: %v", err)
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		log.Printf("runtime: SetInformationJobObject failed, no kill-on-crash safety net: %v", err)
		_ = windows.CloseHandle(job)
		return 0
	}
	self, err := windows.GetCurrentProcess()
	if err != nil {
		log.Printf("runtime: GetCurrentProcess failed, no kill-on-crash safety net: %v", err)
		_ = windows.CloseHandle(job)
		return 0
	}
	if err := windows.AssignProcessToJobObject(job, self); err != nil {
		log.Printf("runtime: AssignProcessToJobObject failed, no kill-on-crash safety net: %v", err)
		_ = windows.CloseHandle(job)
		return 0
	}
	log.Printf("runtime: chromium/ffmpeg will be killed automatically if Surf exits unexpectedly")
	return job
}

// resolveChromePath fills in a real browser path when the caller left
// ChromePath at its generic "chromium" default and nothing by that name is
// on PATH: Windows has no standard "chromium" command, but most boxes have
// a real Chromium/Chrome install or, failing that, Edge (Chromium-based,
// ships with every Windows 10/11 install, speaks CDP identically).
func resolveChromePath(path string) string {
	if path != "" && path != "chromium" {
		return path
	}
	if _, err := exec.LookPath(path); err == nil {
		return path
	}
	if found := findChromium(); found != "" {
		return found
	}
	return path
}

func findChromium() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	candidates := []string{
		filepath.Join(localAppData, "Chromium", "Application", "chrome.exe"),
		`C:\Program Files\Chromium\Application\chrome.exe`,
		`C:\Program Files (x86)\Chromium\Application\chrome.exe`,
		filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"),
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// windowsHandle holds the kill-on-close Job Object (if it could be set up).
type windowsHandle struct {
	job        windows.Handle
	browserPID atomic.Uint32
}

func (wh *windowsHandle) Shutdown() {
	if wh.job != 0 {
		_ = windows.CloseHandle(wh.job)
	}
}

func (wh *windowsHandle) BrowserStarted(pid int) {
	if pid > 0 {
		wh.browserPID.Store(uint32(pid))
	}
}

func (wh *windowsHandle) OpenAudioCapture() (io.ReadCloser, error) {
	pid := wh.browserPID.Load()
	if pid == 0 {
		return nil, fmt.Errorf("Chromium has not started")
	}
	return winaudio.OpenProcessLoopback(pid)
}

// Windows captures natively through WASAPI rather than an FFmpeg device.
func (wh *windowsHandle) AudioCaptureArgs(source string) []string { return nil }
