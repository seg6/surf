//go:build windows

package process

import (
	"fmt"
	"golang.org/x/sys/windows"
)

// Windows tracks the complete job, even after a bootstrapper exits. Confirm
// that CDP is attached to a process belonging to a job before allowing setup.
func (s *Started) TrackBrowser(pid int) error {
	if s.killOwned == nil {
		return fmt.Errorf("browser containment is unavailable")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	// Job membership in the specific browser scope is checked by its callback.
	if s.verifyOwned != nil {
		return s.verifyOwned(uintptr(handle))
	}
	return fmt.Errorf("browser ownership cannot be verified")
}
