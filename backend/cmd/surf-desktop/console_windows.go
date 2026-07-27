//go:build windows

package main

import "syscall"

func hideConsole() {
	window, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	if window != 0 {
		_, _, _ = syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow").Call(window, 0)
	}
}
