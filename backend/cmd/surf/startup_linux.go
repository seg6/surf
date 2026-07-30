//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func setStartAtLogin(enabled bool) error {
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(config, "autostart", "surf.desktop")
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if appImage := os.Getenv("APPIMAGE"); appImage != "" {
		executable = appImage
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Surf\nExec=%s\nX-GNOME-Autostart-enabled=true\n", strconv.Quote(executable))
	return os.WriteFile(path, []byte(content), 0o644)
}
