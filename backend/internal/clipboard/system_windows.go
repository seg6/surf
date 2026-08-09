//go:build windows

package clipboard

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procEmptyClipboard = user32.NewProc("EmptyClipboard")
	procGetClipboard   = user32.NewProc("GetClipboardData")
	procSetClipboard   = user32.NewProc("SetClipboardData")
	procGlobalAlloc    = kernel32.NewProc("GlobalAlloc")
	procGlobalLock     = kernel32.NewProc("GlobalLock")
	procGlobalUnlock   = kernel32.NewProc("GlobalUnlock")
	procGlobalFree     = kernel32.NewProc("GlobalFree")
)

type windowsSystem struct{}

func (windowsSystem) Name() string { return "Windows" }

func openWindowsClipboard(ctx context.Context) error {
	for attempt := 0; attempt < 20; attempt++ {
		opened, _, _ := procOpenClipboard.Call(0)
		if opened != 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return errors.New("clipboard is busy")
}

func (windowsSystem) Read(ctx context.Context) (string, error) {
	if err := openWindowsClipboard(ctx); err != nil {
		return "", fmt.Errorf("read Windows clipboard: %w", err)
	}
	defer procCloseClipboard.Call()
	handle, _, callErr := procGetClipboard.Call(cfUnicodeText)
	if handle == 0 {
		if callErr != syscall.Errno(0) {
			return "", fmt.Errorf("read Windows clipboard: %w", callErr)
		}
		return "", nil
	}
	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		return "", fmt.Errorf("lock Windows clipboard: %w", callErr)
	}
	defer procGlobalUnlock.Call(handle)
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(pointer))), nil
}

func (windowsSystem) Write(ctx context.Context, text string) error {
	value, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	if err := openWindowsClipboard(ctx); err != nil {
		return fmt.Errorf("write Windows clipboard: %w", err)
	}
	defer procCloseClipboard.Call()
	if emptied, _, callErr := procEmptyClipboard.Call(); emptied == 0 {
		return fmt.Errorf("empty Windows clipboard: %w", callErr)
	}
	bytes := uintptr(len(value) * 2)
	handle, _, callErr := procGlobalAlloc.Call(gmemMoveable, bytes)
	if handle == 0 {
		return fmt.Errorf("allocate Windows clipboard: %w", callErr)
	}
	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		procGlobalFree.Call(handle)
		return fmt.Errorf("lock Windows clipboard: %w", callErr)
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(pointer)), len(value)), value)
	procGlobalUnlock.Call(handle)
	if accepted, _, callErr := procSetClipboard.Call(cfUnicodeText, handle); accepted == 0 {
		procGlobalFree.Call(handle)
		return fmt.Errorf("set Windows clipboard: %w", callErr)
	}
	return nil // ownership of handle transferred to the clipboard
}

func newSystemClipboard() systemClipboard { return windowsSystem{} }
