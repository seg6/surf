//go:build !windows && !linux

package process

import (
	"os/exec"
	"syscall"
)

func configureChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
