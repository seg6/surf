//go:build windows

// Package runenv's Windows platform runs Chromium directly on the desktop
// session: no virtual display or audio sink to manage. Chromium launches
// visible on the real desktop, pinned at the same --window-position=0,0/
// --window-size flags cdp.LaunchConfig always passes, so gdigrab can grab
// that fixed screen region for the H.264 lane.
//
// This is the "visible window" version, not the hidden-desktop one: a true
// stealth capture (CreateDesktop + a custom SetThreadDesktop/BitBlt or DXGI
// duplication capturer feeding raw frames into ffmpeg's stdin) needs Win32
// syscalls neither the standard library nor golang.org/x/sys/windows expose
// today (no Desktop field on syscall.SysProcAttr, no CreateDesktopW wrapper,
// no window-enumeration helpers). That's a real follow-up, not this pass.
//
// The PCM lane is still unsupported here: dshow only exposes physical input
// devices (microphones) on a stock Windows box, not a loopback/monitor
// source. Capturing Chromium's own audio without also capturing the host's
// speakers needs either a virtual audio cable or the Windows 10 2004+
// per-process loopback API (ActivateAudioInterfaceAsync with
// AUDIOCLIENT_ACTIVATION_PARAMS/PROCESS_LOOPBACK) — neither is wired up yet.
package runenv

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"surf-backend/internal/config"
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
// OS-level safety net for when surf-backend itself is force-killed and
// never gets to run its own cleanup at all. Confirmed live: a forced kill of
// surf-backend's own process left a dozen orphaned Chromium helper
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
	log.Printf("runtime: chromium/ffmpeg will be killed automatically if surf-backend exits unexpectedly")
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

// windowsHandle holds the kill-on-close Job Object (if it could be set up);
// nothing else Prepare does needs teardown.
type windowsHandle struct {
	job windows.Handle
}

func (wh *windowsHandle) Shutdown() {
	if wh.job != 0 {
		_ = windows.CloseHandle(wh.job)
	}
}

// ChromeArgs is empty: Chromium uses its normal Windows windowing backend,
// no ozone/X11 flag needed.
func (wh *windowsHandle) ChromeArgs() []string { return nil }

// VideoCaptureArgs grabs a fixed on-screen region with gdigrab. w/h match
// the --window-size Chromium was launched with, and the capture offset
// matches --window-position=0,0, so this only works while that window is
// visible and un-occluded at its launch position (see the package doc for
// the hidden-desktop follow-up).
func (wh *windowsHandle) VideoCaptureArgs(surface string, w, h, fps int) []string {
	return []string{
		"-loglevel", "warning",
		"-f", "gdigrab",
		"-framerate", fmt.Sprint(fps),
		"-offset_x", "0", "-offset_y", "0",
		"-video_size", fmt.Sprintf("%dx%d", w, h),
		"-i", "desktop",
	}
}

// AudioCaptureArgs is unsupported for now: nil disables the PCM lane. See
// the package doc for what a real implementation needs.
func (wh *windowsHandle) AudioCaptureArgs(source string) []string { return nil }

// ResizeSurface is a no-op: the capture region is fixed at launch time via
// --window-size; a live client resize won't retarget it yet.
func (wh *windowsHandle) ResizeSurface(w, h int) error { return nil }
