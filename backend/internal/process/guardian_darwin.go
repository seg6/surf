//go:build darwin

package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func guardedCommand(path string, args []string, enabled bool, grace time.Duration) (*exec.Cmd, bool, error) {
	if !enabled {
		return command(path, args...), false, nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil, false, fmt.Errorf("resolve Surf child guardian: %w", err)
	}
	guardianArgs := []string{"_internal", "child-guard", "--grace-ms", strconv.FormatInt(grace.Milliseconds(), 10), "--", path}
	guardianArgs = append(guardianArgs, args...)
	return command(self, guardianArgs...), true, nil
}

// RunChildGuardian is Surf's private macOS parent-death shim. The guardian and
// Chromium share a dedicated process group. If the Surf server disappears,
// stdin reaches EOF, the guarded child's stdin is closed, and the child gets a
// bounded grace period before the whole group is killed. During normal
// shutdown the child exits first and is reaped here.
func RunChildGuardian(args []string) error {
	if len(args) < 4 || args[0] != "--grace-ms" || args[2] != "--" || args[3] == "" {
		return errors.New("invalid child guardian invocation")
	}
	graceMS, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || graceMS < 0 || graceMS > 30000 {
		return errors.New("invalid child guardian grace period")
	}
	childInput, err := cmdStdinPipe()
	if err != nil {
		return err
	}
	defer childInput.reader.Close()
	defer childInput.writer.Close()
	cmd := exec.Command(args[3], args[4:]...)
	cmd.Stdin = childInput.reader
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	parentGone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		close(parentGone)
	}()
	select {
	case err := <-done:
		_ = childInput.writer.Close()
		return err
	case <-parentGone:
		_ = childInput.writer.Close()
		if graceMS > 0 {
			select {
			case err := <-done:
				return err
			case <-time.After(time.Duration(graceMS) * time.Millisecond):
			}
		}
		// This process was started as its own group leader and its child inherits
		// that group. SIGKILL deliberately includes the guardian itself.
		return syscall.Kill(-os.Getpid(), syscall.SIGKILL)
	}
}

type guardianInput struct {
	reader *os.File
	writer *os.File
}

func cmdStdinPipe() (guardianInput, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return guardianInput{}, fmt.Errorf("create child guardian input: %w", err)
	}
	return guardianInput{reader: reader, writer: writer}, nil
}
