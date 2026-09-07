package browser

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func publishDownload(source, destination string) error {
	err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return linkDownload(source, destination)
	}
	if err != nil {
		return &os.LinkError{Op: "publish download", Old: source, New: destination, Err: err}
	}
	return nil
}
