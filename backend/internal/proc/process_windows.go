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
	"os/exec"
	"strconv"
)

func Command(path string, args ...string) *exec.Cmd {
	return exec.Command(path, args...)
}

func Kill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Best effort: fall back to killing just the tracked process if taskkill
	// itself is unavailable for some reason (it ships with every Windows
	// install, so this should be unreachable in practice).
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
		_ = cmd.Process.Kill()
	}
}
