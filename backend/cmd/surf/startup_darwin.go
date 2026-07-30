//go:build darwin

package main

import (
	"html"
	"os"
	"path/filepath"
)

func setStartAtLogin(enabled bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "space.seg6.surf.desktop.plist")
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>space.seg6.surf.desktop</string>
<key>ProgramArguments</key><array><string>` + html.EscapeString(executable) + `</string></array>
<key>RunAtLoad</key><true/>
</dict></plist>
`
	return os.WriteFile(path, []byte(content), 0o644)
}
