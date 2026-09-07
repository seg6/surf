package browser

import (
	"os"

	"golang.org/x/sys/windows"
)

func publishDownload(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	// No MOVEFILE_REPLACE_EXISTING: preserve an existing destination.
	if err := windows.MoveFileEx(from, to, 0); err != nil {
		return &os.LinkError{Op: "publish download", Old: source, New: destination, Err: err}
	}
	return nil
}
