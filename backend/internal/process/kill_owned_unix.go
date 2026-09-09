//go:build !windows

package process

import "syscall"

// Unlike the legacy best-effort Kill, this never falls back to a recycled PID.
func killOwnedProcess(pid int) error { return syscall.Kill(-pid, syscall.SIGKILL) }
