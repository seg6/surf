package platform

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

// Doctor lists the external tool checks for the platform matching
// runtime.GOOS.
func Doctor(cfg *config.Config) []Check {
	return newPlatform().Doctor(cfg)
}

func checkTool(name, path string, required bool) Check {
	_, err := exec.LookPath(path)
	return Check{Name: name, Path: path, Required: required, OK: err == nil, Err: err}
}
