// Package process provides OS-specific process launch/kill helpers so the rest
// of the backend doesn't need per-OS branches: process-group teardown on Unix,
// and hidden process-tree teardown on Windows.
package process

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Options configures Start.
type Options struct {
	// Visible permits a GUI browser window; helpers still suppress consoles.
	Visible bool
	// Contain requests an independently owned browser process group/job.
	Contain bool
	// Env is the child's full environment (same convention as
	// exec.Cmd.Env: nil means inherit the caller's).
	Env []string
	// Stdin/Stdout/Stderr request a pipe to the corresponding stream; left
	// nil on Started when not requested.
	Stdin  bool
	Stdout bool
	Stderr bool
	// StdoutWriter and StderrWriter attach existing destinations instead of
	// requesting pipes. They are useful for supervised children whose output
	// should be appended directly to the parent's log.
	StdoutWriter io.Writer
	StderrWriter io.Writer
	// Guardian asks platforms without a native parent-death facility to wrap
	// the child in Surf's private parent watcher. It is currently meaningful
	// on macOS; Linux and Windows already provide kernel-backed containment.
	Guardian      bool
	GuardianGrace time.Duration
}

// Started is a launched child process plus whichever pipes Options asked
// for.
type Started struct {
	killOwned    func() error
	waitOwned    func(time.Duration) error
	releaseOwned func()
	verifyOwned  func(uintptr) error
	Process      *os.Process
	Pid          int
	Stdin        io.WriteCloser
	Stdout       io.ReadCloser
	Stderr       io.ReadCloser
	// Done receives the result of exec.Cmd.Wait exactly once. Start always
	// reaps the child, even when its caller only needs the OS process handle.
	Done <-chan error
}

func (s *Started) WaitOwned(timeout time.Duration) error {
	if s != nil && s.waitOwned != nil {
		return s.waitOwned(timeout)
	}
	return nil
}

func (s *Started) ReleaseOwned() {
	if s != nil && s.releaseOwned != nil {
		s.releaseOwned()
	}
}

// Kill terminates the complete owned process tree, never only the root PID.
func (s *Started) Kill() error {
	if s == nil || s.Process == nil {
		return nil
	}
	if s.killOwned != nil {
		return s.killOwned()
	}
	select {
	case <-s.Done:
		return fmt.Errorf("owned browser process already exited; refusing an unverified PID kill")
	default:
	}
	return killOwnedProcess(s.Process.Pid)
}
