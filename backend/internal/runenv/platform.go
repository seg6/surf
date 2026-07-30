package runenv

import "surf-backend/internal/config"

// Platform abstracts the host-specific process-lifetime and runtime setup
// that remains after Chromium tab capture made media capture cross-platform.
// One implementation exists per supported GOOS, selected by newPlatform.
type Platform interface {
	// Prepare performs host setup and returns a handle that stays alive for
	// the server's lifetime.
	Prepare(cfg *config.Config) (Handle, error)
	// Doctor lists the external tools this platform requires.
	Doctor(cfg *config.Config) []Check
}

// Handle is the live instance Prepare returned.
type Handle interface {
	// Shutdown tears down whatever Prepare started.
	Shutdown()
}
