//go:build windows

// Chromium and ffmpeg both spawn their own child/helper process trees on
// Windows (Chromium in particular forks GPU/utility/renderer/crashpad
// processes under its top-level PID). Killing just the tracked process
// leaves that whole tree running orphaned — confirmed live: a forced kill
// of surf-backend's own process left a dozen Chromium helper processes
// behind. taskkill /T terminates the process tree rooted at the given PID,
// so Kill uses that instead of TerminateProcess on the single tracked PID.
package proc

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

func Command(path string, args ...string) *exec.Cmd {
	return exec.Command(path, args...)
}

func Kill(pid int) {
	if pid <= 0 {
		return
	}
	// Best effort: fall back to killing just the tracked process if taskkill
	// itself is unavailable for some reason (it ships with every Windows
	// install, so this should be unreachable in practice).
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run(); err != nil {
		if p, ferr := os.FindProcess(pid); ferr == nil {
			_ = p.Kill()
		}
	}
}

// Start launches path with args. Without a Desktop this is just
// exec.Command dressed up in the cross-platform Started shape; with one, it
// bypasses os/exec entirely and calls CreateProcess directly with
// STARTUPINFO.lpDesktop set — something syscall.SysProcAttr has no field
// for (confirmed against the Go 1.26 source: HideWindow, CmdLine,
// CreationFlags, Token, ProcessAttributes, ThreadAttributes,
// NoInheritHandles, AdditionalInheritedHandles, ParentProcess, and nothing
// else), so there is no way to express this through exec.Cmd at all.
func Start(path string, args []string, opts Options) (*Started, error) {
	if opts.Desktop == "" {
		return startViaExec(path, args, opts)
	}
	return startOnDesktop(path, args, opts)
}

func startViaExec(path string, args []string, opts Options) (*Started, error) {
	cmd := exec.Command(path, args...)
	cmd.Env = opts.Env
	started := &Started{}
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

// startOnDesktop replicates just enough of exec.Cmd's Start/StdoutPipe/
// StderrPipe behavior by hand — building the pipes ourselves and marking
// only their write ends inheritable, then calling CreateProcess directly
// with StartupInfo.Desktop set — so the child (and everything it spawns,
// since child processes inherit their creator's desktop unless launched
// with CREATE_BREAKAWAY_FROM_JOB, which nothing here passes) ends up on the
// named desktop instead of the interactive one.
func startOnDesktop(path string, args []string, opts Options) (_ *Started, err error) {
	var stdoutRead, stdoutWrite, stderrRead, stderrWrite *os.File
	if opts.Stdout {
		if stdoutRead, stdoutWrite, err = os.Pipe(); err != nil {
			return nil, fmt.Errorf("stdout pipe: %w", err)
		}
		defer stdoutWrite.Close()
	}
	if opts.Stderr {
		if stderrRead, stderrWrite, err = os.Pipe(); err != nil {
			return nil, fmt.Errorf("stderr pipe: %w", err)
		}
		defer stderrWrite.Close()
	}
	defer func() {
		// The read ends are only handed back to the caller once Start
		// actually succeeds; close them ourselves on any earlier failure.
		if err != nil {
			if stdoutRead != nil {
				stdoutRead.Close()
			}
			if stderrRead != nil {
				stderrRead.Close()
			}
		}
	}()

	if stdoutWrite != nil {
		if e := windows.SetHandleInformation(windows.Handle(stdoutWrite.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); e != nil {
			return nil, fmt.Errorf("mark stdout handle inheritable: %w", e)
		}
	}
	if stderrWrite != nil {
		if e := windows.SetHandleInformation(windows.Handle(stderrWrite.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); e != nil {
			return nil, fmt.Errorf("mark stderr handle inheritable: %w", e)
		}
	}

	desktopPtr, err := windows.UTF16PtrFromString(opts.Desktop)
	if err != nil {
		return nil, err
	}
	si := &windows.StartupInfo{Desktop: desktopPtr}
	si.Cb = uint32(unsafe.Sizeof(*si))
	if stdoutWrite != nil || stderrWrite != nil {
		si.Flags |= windows.STARTF_USESTDHANDLES
		if stdoutWrite != nil {
			si.StdOutput = windows.Handle(stdoutWrite.Fd())
		}
		if stderrWrite != nil {
			si.StdErr = windows.Handle(stderrWrite.Fd())
		}
	}

	cmdLine := windows.EscapeArg(path)
	for _, a := range args {
		cmdLine += " " + windows.EscapeArg(a)
	}
	cmdLinePtr, err := windows.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, err
	}
	var envPtr *uint16
	if len(opts.Env) > 0 {
		envPtr, err = createEnvBlock(opts.Env)
		if err != nil {
			return nil, err
		}
	}

	pi := &windows.ProcessInformation{}
	if err := windows.CreateProcess(nil, cmdLinePtr, nil, nil, true,
		windows.CREATE_UNICODE_ENVIRONMENT, envPtr, nil, si, pi); err != nil {
		return nil, fmt.Errorf("CreateProcess: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)

	proc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		return nil, err
	}
	return &Started{Process: proc, Stdout: stdoutRead, Stderr: stderrRead}, nil
}

// createEnvBlock builds the NUL-terminated-pairs-plus-final-NUL block
// CreateProcess expects for its lpEnvironment argument. Windows documents a
// preference for alphabetically sorted entries; skipped here since
// CreateProcess does not actually reject unsorted blocks and the
// environments passed through this path are always small.
func createEnvBlock(env []string) (*uint16, error) {
	var buf []uint16
	for _, kv := range env {
		u, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, err
		}
		buf = append(buf, u...) // UTF16FromString includes the trailing NUL
	}
	buf = append(buf, 0) // final NUL terminates the block
	return &buf[0], nil
}
