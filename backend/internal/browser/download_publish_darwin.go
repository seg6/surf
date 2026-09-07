package browser

import (
	"os"

	"golang.org/x/sys/unix"
)

func publishDownload(source, destination string) error {
	if err := unix.RenamexNp(source, destination, unix.RENAME_EXCL); err != nil {
		return &os.LinkError{Op: "publish download", Old: source, New: destination, Err: err}
	}
	return nil
}
