//go:build !darwin && !windows

package process

import (
	"errors"
	"os/exec"
	"time"
)

func guardedCommand(path string, args []string, _ bool, _ time.Duration) (*exec.Cmd, bool, error) {
	return command(path, args...), false, nil
}

func RunChildGuardian([]string) error {
	return errors.New("child guardian is only available on macOS")
}
