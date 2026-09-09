//go:build windows

package process

import "fmt"

func killOwnedProcess(pid int) error {
	return fmt.Errorf("browser process containment is unavailable; close it on the computer")
}
