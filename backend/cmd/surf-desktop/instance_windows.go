//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type desktopLock struct{ file *os.File }

func acquireDesktopInstance(home string) (*desktopLock, bool, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, false, err
	}
	file, err := os.OpenFile(filepath.Join(home, "desktop.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	var overlapped windows.Overlapped
	err = windows.LockFileEx(windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		file.Close()
		return &desktopLock{}, false, nil
	}
	if err != nil {
		file.Close()
		return nil, false, err
	}
	return &desktopLock{file: file}, true, nil
}

func (l *desktopLock) Close() error {
	if l.file == nil {
		return nil
	}
	var overlapped windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &overlapped)
	return l.file.Close()
}
