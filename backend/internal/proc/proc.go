// Package proc provides OS-specific process launch/kill helpers so the rest
// of the backend doesn't need per-OS branches: Setpgid + process-group kill
// on Linux, kill-the-whole-tree via taskkill on Windows, and (Windows only)
// the ability to launch a process onto a specific desktop — needed so
// Chromium and its ffmpeg capture companion can run invisibly, on a desktop
// the user never sees, instead of merely being minimized or positioned
// off-screen.
package proc

import (
	"io"
	"os"
)

// Options configures Start.
type Options struct {
	// Env is the child's full environment (same convention as
	// exec.Cmd.Env: nil means inherit the caller's).
	Env []string
	// Desktop names a Windows desktop (e.g. `WinSta0\SurfHidden`) the child
	// should be created on. Ignored on every other OS, and ignored on
	// Windows too when empty — meaning "inherit the caller's desktop",
	// exactly like before this field existed.
	Desktop string
	// Stdout/Stderr request a pipe to the corresponding stream; left nil on
	// Started when not requested.
	Stdout bool
	Stderr bool
}

// Started is a launched child process plus whichever pipes Options asked
// for.
type Started struct {
	Process *os.Process
	Stdout  io.ReadCloser
	Stderr  io.ReadCloser
}
