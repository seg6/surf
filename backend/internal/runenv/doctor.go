package runenv

import (
	"os/exec"

	"surf-backend/internal/config"
)

type Check struct {
	Name     string
	Path     string
	Required bool
	OK       bool
	Err      error
}

func Doctor(cfg *config.Config) []Check {
	checks := []Check{
		checkTool("chromium", cfg.ChromePath, true),
		checkTool("ffmpeg", cfg.FFmpegPath, true),
	}
	if cfg.ManageDisplay {
		checks = append(checks, checkTool("Xvfb", cfg.XvfbPath, true))
	}
	checks = append(checks, checkTool("xrandr", cfg.XrandrPath, false))
	if cfg.ManagePulse {
		checks = append(checks, checkTool("pulseaudio", cfg.PulseaudioPath, true), checkTool("pactl", cfg.PactlPath, true))
	} else if cfg.EnsurePulseSink {
		checks = append(checks, checkTool("pactl", cfg.PactlPath, true))
	}
	return checks
}

func checkTool(name, path string, required bool) Check {
	_, err := exec.LookPath(path)
	return Check{Name: name, Path: path, Required: required, OK: err == nil, Err: err}
}
