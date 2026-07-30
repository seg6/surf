//go:build darwin

package platform

import (
	"testing"

	"surf-backend/internal/config"
)

func TestStubPlatformDoctorChecksChromium(t *testing.T) {
	checks := Doctor(&config.Config{ChromePath: "missing-chromium"})
	if len(checks) != 1 || checks[0].Name != "chromium" {
		t.Fatalf("checks=%v, want chromium only", checks)
	}
}

func TestStubPlatformPrepares(t *testing.T) {
	h, err := newPlatform().Prepare(&config.Config{})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer h.Shutdown()
}
