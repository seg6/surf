//go:build !windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
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
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return &desktopLock{}, false, nil
	}
	return &desktopLock{file: file}, true, nil
}

func (l *desktopLock) Close() error {
	if l.file == nil {
		return nil
	}
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	return l.file.Close()
}
