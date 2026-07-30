//go:build windows

package main

import (
	"os"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

func setStartAtLogin(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		err := key.DeleteValue("Surf")
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return key.SetStringValue("Surf", strconv.Quote(executable))
}
