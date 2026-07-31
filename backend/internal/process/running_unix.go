//go:build !windows

package process

import (
	"errors"
	"syscall"
)

func Running(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
