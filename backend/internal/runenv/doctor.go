package runenv

import (
	"os/exec"

	"surf-backend/internal/config"
)

type Check struct {
	Name string
	Path string
	OK   bool
	Err  error
}

func Doctor(cfg *config.Config) []Check {
	checks := []Check{
		checkTool("chromium", cfg.ChromePath),
		checkTool("ffmpeg", cfg.FFmpegPath),
	}
	if cfg.ManageDisplay {
		checks = append(checks, checkTool("Xvfb", cfg.XvfbPath))
	}
	checks = append(checks, checkTool("xrandr", cfg.XrandrPath))
	if cfg.ManagePulse {
		checks = append(checks, checkTool("pulseaudio", cfg.PulseaudioPath), checkTool("pactl", cfg.PactlPath))
	}
	return checks
}

func checkTool(name, path string) Check {
	_, err := exec.LookPath(path)
	return Check{Name: name, Path: path, OK: err == nil, Err: err}
}
