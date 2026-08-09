//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func prepareConsole(commandLine bool) {
	if !commandLine {
		return
	}
	const attachParentProcess = ^uintptr(0)
	attached, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole").Call(attachParentProcess)
	if attached == 0 {
		return
	}
	if output, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = output
		os.Stderr = output
	}
}

func hideConsole() {
	// Release binaries use the GUI subsystem, so Explorer never creates a
	// console. Detach here as a belt-and-suspenders measure for update helpers.
	_, _, _ = syscall.NewLazyDLL("kernel32.dll").NewProc("FreeConsole").Call()
}

func launchUpdateHelper(staged, target string, restart bool) error {
	mode := "stop"
	if restart {
		mode = "restart"
	}
	command := exec.Command(staged, "_internal", "update-helper", staged, target, mode)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Start()
}
