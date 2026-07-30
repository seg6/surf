// Package platform owns host-specific process orchestration for Surf: it
// dispatches to the implementation selected by the current GOOS.
package platform

import "surf-backend/internal/config"

// Runtime is the live host runtime returned by Start; call Shutdown when the
// server exits.
type Runtime struct {
	handle handle
}

// Shutdown tears down whatever the platform's Prepare started.
func (r *Runtime) Shutdown() {
	if r.handle != nil {
		r.handle.Shutdown()
	}
}

// Start prepares host runtime state and process-lifetime safeguards.
func Start(cfg *config.Config) (*Runtime, error) {
	handle, err := newPlatform().Prepare(cfg)
	if err != nil {
		return nil, err
	}
	return &Runtime{handle: handle}, nil
}
