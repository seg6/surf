//go:build windows

// Package runenv's Windows platform runs Chromium on a hidden desktop: a
// second, non-interactive desktop object inside the same window station as
// the caller (WinSta0), created via CreateDesktopW. Chromium is launched
// onto it via STARTUPINFO.lpDesktop, so it never renders to the user's real
// screen or can steal focus — stronger than moving/hiding a window, since
// the window manager never puts it in front of anything to begin with.
// Confirmed live: a normally-launched Chromium shows a real MainWindowHandle
// and title from the interactive desktop; the same Chromium launched with
// lpDesktop set to the hidden one shows MainWindowHandle=0 for every one of
// its processes — genuinely invisible, not just minimized.
//
// The H.264 lane doesn't survive the trip, though: gdigrab's
// GetDC(NULL)/BitBlt fails with ERROR_ACCESS_DENIED against a desktop that
// has never been the foreground one — confirmed live, including with an
// explicitly permissive DACL on the desktop (so it isn't a plain
// permissions problem), and confirmed to be exactly that by switching the
// hidden desktop to foreground with SwitchDesktop, which made capture work
// immediately. Windows only fully composites the active desktop; a hidden
// one never gets to be foreground without briefly flashing on screen and
// defeating the entire point. So VideoCaptureArgs returns nil whenever a
// hidden desktop is active, and browsing falls back to the JPEG/CDP
// screencast lane, which operates at the Chromium compositor level over
// the DevTools protocol and doesn't care which desktop the process is on.
//
// Neither the standard library nor golang.org/x/sys/windows expose
// STARTUPINFO.lpDesktop or CreateDesktopW (confirmed against the Go 1.26
// source: syscall.SysProcAttr on Windows has no Desktop field at all), so
// this bypasses os/exec for the Chromium launch — see internal/proc's
// Start/Options.Desktop and this file's createHiddenDesktop. If desktop
// creation fails for any reason, Prepare falls back to launching visible
// (HiddenDesktop returns ""), same as before this existed.
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
	"syscall"
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
	wh := &windowsHandle{job: createKillOnCloseJob()}
	if cfg.HiddenDesktop {
		name, handle, err := createHiddenDesktop(hiddenDesktopName)
		if err != nil {
			log.Printf("runtime: hidden desktop unavailable, launching visible: %v", err)
		} else {
			wh.desktopName = name
			wh.desktopHandle = handle
			log.Printf("runtime: chromium/ffmpeg will run on the hidden desktop %q", name)
		}
	}
	return wh, nil
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

// --- hidden desktop -------------------------------------------------------
//
// CreateDesktopW/CloseDesktop are USER32 functions; neither the standard
// library's syscall package nor golang.org/x/sys/windows wrap them (both
// focus on kernel32/advapi32-level primitives), so they're loaded by hand
// here the same way x/sys/windows itself wraps functions internally.

var (
	modUser32          = syscall.NewLazyDLL("user32.dll")
	procCreateDesktopW = modUser32.NewProc("CreateDesktopW")
	procCloseDesktop   = modUser32.NewProc("CloseDesktop")
)

// hiddenDesktopName is the bare desktop name passed to CreateDesktopW; the
// window-station-qualified form ("WinSta0\SurfHidden") is what
// STARTUPINFO.lpDesktop and gdigrab's launch both need.
const hiddenDesktopName = "SurfHidden"

const (
	desktopReadObjects   = 0x0001
	desktopCreateWindow  = 0x0002
	desktopCreateMenu    = 0x0004
	desktopHookControl   = 0x0008
	desktopJournalRecord = 0x0010
	desktopJournalPlay   = 0x0020
	desktopEnumerate     = 0x0040
	desktopWriteObjects  = 0x0080
	desktopSwitchDesktop = 0x0100
	genericAll           = 0x10000000

	desktopFullAccess = desktopReadObjects | desktopCreateWindow | desktopCreateMenu |
		desktopHookControl | desktopJournalRecord | desktopJournalPlay |
		desktopEnumerate | desktopWriteObjects | desktopSwitchDesktop | genericAll
)

// createHiddenDesktop creates (or, on a second run with a stale profile
// dir, opens) a non-interactive desktop in the caller's own window station.
// It returns the window-station-qualified name to pass as
// STARTUPINFO.lpDesktop, plus the handle so Shutdown can close it.
func createHiddenDesktop(name string) (qualifiedName string, handle windows.Handle, err error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", 0, err
	}
	r1, _, e1 := procCreateDesktopW.Call(
		uintptr(unsafe.Pointer(namePtr)),
		0, // device: reserved, must be NULL
		0, // devmode: reserved, must be NULL
		0, // flags
		uintptr(desktopFullAccess),
		0, // security attributes: NULL, default ACL
	)
	if r1 == 0 {
		return "", 0, fmt.Errorf("CreateDesktopW: %w", e1)
	}
	return `WinSta0\` + name, windows.Handle(r1), nil
}

func closeDesktop(h windows.Handle) {
	if h != 0 {
		procCloseDesktop.Call(uintptr(h))
	}
}

// windowsHandle holds the kill-on-close Job Object and hidden desktop (if
// either could be set up).
type windowsHandle struct {
	job           windows.Handle
	desktopName   string
	desktopHandle windows.Handle
}

func (wh *windowsHandle) Shutdown() {
	if wh.job != 0 {
		_ = windows.CloseHandle(wh.job)
	}
	closeDesktop(wh.desktopHandle)
}

// ChromeArgs is empty: Chromium uses its normal Windows windowing backend,
// no ozone/X11 flag needed.
func (wh *windowsHandle) ChromeArgs() []string { return nil }

// VideoCaptureArgs grabs the whole screen with gdigrab. Without a hidden
// desktop this captures a fixed on-screen region matching Chromium's
// --window-position=0,0/--window-size, which only works while that window
// stays visible and un-occluded.
//
// With a hidden desktop, the H.264 lane is unsupported (nil): confirmed
// live that gdigrab's GetDC(NULL)/BitBlt fails with ERROR_ACCESS_DENIED
// against a desktop that has never been made the foreground one, DACL
// notwithstanding — switching the hidden desktop to foreground (which would
// flash it visibly on screen, defeating the point) made capture succeed
// immediately, confirming this is Windows only fully compositing the
// active desktop, not a permissions problem this code can fix. Browsing
// still works via the JPEG/CDP screencast lane, which operates at the
// Chromium compositor level over the DevTools protocol and doesn't care
// which desktop the process is attached to.
func (wh *windowsHandle) VideoCaptureArgs(surface string, w, h, fps int) []string {
	if wh.desktopName != "" {
		return nil
	}
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
// --window-size (or, in hidden-desktop mode, gdigrab already grabs the
// whole hidden desktop regardless of viewport size); a live client resize
// won't retarget either.
func (wh *windowsHandle) ResizeSurface(w, h int) error { return nil }

// HiddenDesktop returns the window-station-qualified desktop name Chromium
// and the capture ffmpeg should launch onto, or "" if none was set up.
func (wh *windowsHandle) HiddenDesktop() string { return wh.desktopName }
