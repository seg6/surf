//go:build !windows

package main

import "os/exec"

func hideConsole()        {}
func prepareConsole(bool) {}

func launchUpdateHelper(staged, target string, restart bool) error {
	mode := "stop"
	if restart {
		mode = "restart"
	}
	return exec.Command(staged, "_internal", "update-helper", staged, target, mode).Start()
}
