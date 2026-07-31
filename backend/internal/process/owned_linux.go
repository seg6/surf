//go:build linux

package process

import (
	"os"
	"path/filepath"
	"strconv"
)

func MatchesExecutable(pid int, expected string) bool {
	if pid <= 0 || expected == "" {
		return false
	}
	actual, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		return false
	}
	want, err := filepath.EvalSymlinks(expected)
	if err != nil {
		want = expected
	}
	got, err := filepath.EvalSymlinks(actual)
	if err != nil {
		got = actual
	}
	return filepath.Clean(got) == filepath.Clean(want)
}
