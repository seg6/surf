//go:build windows

// Chromium and ffmpeg both spawn their own child/helper process trees on
// Windows (Chromium in particular forks GPU/utility/renderer/crashpad
// processes under its top-level PID). Killing just the tracked process
// leaves that whole tree running orphaned — confirmed live: a forced kill
// of Surf's own process left a dozen Chromium helper processes
// behind. taskkill /T terminates the process tree rooted at the given PID,
// so Kill uses that instead of TerminateProcess on the single tracked PID.
package proc

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func Command(path string, args ...string) *exec.Cmd {
	return hiddenCommand(path, args...)
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
