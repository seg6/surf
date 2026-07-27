// Package proc provides OS-specific process launch/kill helpers so the rest
// of the backend doesn't need per-OS branches: Setpgid + process-group kill
// on Linux, kill-the-whole-tree via taskkill on Windows.
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
	// Stdin/Stdout/Stderr request a pipe to the corresponding stream; left
	// nil on Started when not requested.
	Stdin  bool
	Stdout bool
	Stderr bool
}

// Started is a launched child process plus whichever pipes Options asked
// for.
type Started struct {
	Process *os.Process
	Stdin   io.WriteCloser
	Stdout  io.ReadCloser
	Stderr  io.ReadCloser
}
