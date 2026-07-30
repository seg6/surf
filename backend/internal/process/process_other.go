//go:build !linux && !windows

package process

import (
	"os"
	"os/exec"
)

func Kill(pid int) {
	if pid <= 0 {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

// Start launches path with args (Options.Desktop is a Windows-only concept,
// ignored here).
func Start(path string, args []string, opts Options) (*Started, error) {
	cmd := exec.Command(path, args...)
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
