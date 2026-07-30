//go:build windows

package process

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
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
	var overlapped windows.Overlapped
	err = windows.LockFileEx(windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		_ = file.Close()
		return &InstanceLock{}, false, nil
	}
	if err != nil {
		_ = file.Close()
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
	var overlapped windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	return file.Close()
}
