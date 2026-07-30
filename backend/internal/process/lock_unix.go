//go:build !windows

package process

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// InstanceLock is an advisory lock released automatically if its owner exits.
type InstanceLock struct{ file *os.File }

// AcquireInstanceLock prevents two Surf backends from owning one browser
// profile. The lock is kernel-owned, so a crash cannot leave it stuck.
func AcquireInstanceLock(path string) (*InstanceLock, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return &InstanceLock{}, false, nil
		}
		return nil, false, err
	}
	return &InstanceLock{file: file}, true, nil
}

func (l *InstanceLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return file.Close()
}
