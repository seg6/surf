//go:build windows

package process

import "errors"

func RunChildGuardian([]string) error {
	return errors.New("child guardian is only available on macOS")
}
