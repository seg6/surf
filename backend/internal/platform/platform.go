package platform

import "surf-backend/internal/config"

// platform abstracts the host-specific process-lifetime and runtime setup
// that remains after Chromium tab capture made media capture cross-platform.
// One implementation exists per supported GOOS, selected by newPlatform.
type platform interface {
	// Prepare performs host setup and returns a handle that stays alive for
	// the server's lifetime.
	Prepare(cfg *config.Config) (handle, error)
	// Doctor lists the external tools this platform requires.
	Doctor(cfg *config.Config) []Check
}

// handle is the live instance Prepare returned.
type handle interface {
	// Shutdown tears down whatever Prepare started.
	Shutdown()
}
