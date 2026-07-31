//go:build windows

package atomicfile

import "golang.org/x/sys/windows"

// Replace atomically replaces an existing registry file on Windows and asks
// the filesystem to flush the rename before returning.
func Replace(temporary, target string) error {
	from, err := windows.UTF16PtrFromString(temporary)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
