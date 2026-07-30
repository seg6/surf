//go:build windows

// Chromium spawns its own child/helper process tree on
// Windows (Chromium in particular forks GPU/utility/renderer/crashpad
// processes under its top-level PID). Killing just the tracked process
// leaves that whole tree running orphaned — confirmed live: a forced kill
// of Surf's own process left a dozen Chromium helper processes
// behind. taskkill /T terminates the process tree rooted at the given PID,
// so Kill uses that instead of TerminateProcess on the single tracked PID.
package process

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

// ProtectChildren assigns Surf to a kill-on-close Job Object. Chromium's
// helpers inherit the job, so Windows also cleans the process tree when Surf
// itself is force-killed and cannot execute normal shutdown.
func ProtectChildren() func() {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		log.Printf("runtime: CreateJobObject failed, no kill-on-crash safety net: %v", err)
		return func() {}
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
		return func() {}
	}
	self, err := windows.GetCurrentProcess()
	if err != nil {
		log.Printf("runtime: GetCurrentProcess failed, no kill-on-crash safety net: %v", err)
		_ = windows.CloseHandle(job)
		return func() {}
	}
	if err := windows.AssignProcessToJobObject(job, self); err != nil {
		log.Printf("runtime: AssignProcessToJobObject failed, no kill-on-crash safety net: %v", err)
		_ = windows.CloseHandle(job)
		return func() {}
	}
	log.Printf("runtime: chromium will be killed automatically if Surf exits unexpectedly")
	return func() { _ = windows.CloseHandle(job) }
}

func hiddenCommand(path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	return cmd
}

func Kill(pid int) {
	if pid <= 0 {
		return
	}
	// Best effort: fall back to killing just the tracked process if taskkill
	// itself is unavailable for some reason (it ships with every Windows
	// install, so this should be unreachable in practice).
	if err := hiddenCommand("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run(); err != nil {
		if p, ferr := os.FindProcess(pid); ferr == nil {
			_ = p.Kill()
		}
	}
}

// Start launches path without allocating or showing a console window.
func Start(path string, args []string, opts Options) (*Started, error) {
	cmd := hiddenCommand(path, args...)
	cmd.Env = opts.Env
	started := &Started{}
	if opts.Stdin {
		w, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		started.Stdin = w
	}
	if opts.Stdout {
		r, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		started.Stdout = r
	}
	if opts.Stderr {
		r, err := cmd.StderrPipe()
		if err != nil {
			return nil, err
		}
		started.Stderr = r
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	started.Process = cmd.Process
	return started, nil
}
