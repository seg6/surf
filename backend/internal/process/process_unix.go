//go:build !windows

package process

import (
	"os"
	"os/exec"
	"syscall"
)

// ProtectChildren is unnecessary on Unix: Start creates a dedicated process
// group, Linux also requests a parent-death signal, and explicit shutdown
// kills the whole group.
func ProtectChildren() func() { return func() {} }

func command(path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	configureChild(cmd)
	return cmd
}

// Kill sends SIGKILL to pid's whole process group (Command above always
// starts a new one via Setpgid), falling back to killing just the tracked
// pid if the group kill fails for some reason.
func Kill(pid int) {
	if pid <= 0 {
		return
	}
	if syscall.Kill(-pid, syscall.SIGKILL) != nil {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}
}

// Start launches path with args in its own process group (Options.Desktop
// is a Windows-only concept, ignored here).
func Start(path string, args []string, opts Options) (*Started, error) {
	cmd := command(path, args...)
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
	} else if opts.StdoutWriter != nil {
		cmd.Stdout = opts.StdoutWriter
	}
	if opts.Stderr {
		r, err := cmd.StderrPipe()
		if err != nil {
			return nil, err
		}
		started.Stderr = r
	} else if opts.StderrWriter != nil {
		cmd.Stderr = opts.StderrWriter
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	started.Process = cmd.Process
	done := make(chan error, 1)
	started.Done = done
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	return started, nil
}
