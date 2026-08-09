//go:build windows

package process

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// MatchesExecutable verifies that pid is running the expected executable.
// It is used only as a fallback for a locally recorded Surf server whose
// authenticated control endpoint is no longer responsive.
func MatchesExecutable(pid int, expected string) bool {
	if pid <= 0 || expected == "" {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return false
	}
	actual := windows.UTF16ToString(buffer[:size])
	expectedPath, err := filepath.Abs(expected)
	if err != nil {
		return false
	}
	actualPath, err := filepath.Abs(actual)
	return err == nil && strings.EqualFold(filepath.Clean(actualPath), filepath.Clean(expectedPath))
}
