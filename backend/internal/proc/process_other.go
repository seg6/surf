//go:build !linux && !windows

package proc

import "os/exec"

func Command(path string, args ...string) *exec.Cmd {
	return exec.Command(path, args...)
}

func Kill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
