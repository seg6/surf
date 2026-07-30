//go:build windows

package runenv

import (
	"testing"

	"golang.org/x/sys/windows"

	"surf-backend/internal/config"
)

func TestResolveChromePathPassesThroughExplicitPath(t *testing.T) {
	if got := resolveChromePath(`C:\custom\chrome.exe`); got != `C:\custom\chrome.exe` {
		t.Fatalf("resolveChromePath=%q, want passthrough", got)
	}
}

func TestWindowsDoctorChecksChromium(t *testing.T) {
	checks := Doctor(&config.Config{ChromePath: `C:\missing\chrome.exe`})
	if len(checks) != 1 || checks[0].Name != "chromium" {
		t.Fatalf("checks=%v, want chromium only", checks)
	}
}

func TestWindowsPlatformPrepares(t *testing.T) {
	h, err := newPlatform().Prepare(&config.Config{ChromePath: `C:\custom\chrome.exe`})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer h.Shutdown()
}

// TestCreateKillOnCloseJobBestEffort is intentionally soft: whether a Job
// Object can be created/assigned depends on the host's own process/job
// nesting (a sandbox or CI runner may already restrict this), and
// createKillOnCloseJob is documented as best-effort for exactly that reason.
func TestCreateKillOnCloseJobBestEffort(t *testing.T) {
	job := createKillOnCloseJob()
	if job == 0 {
		t.Log("createKillOnCloseJob returned 0 in this environment (best-effort, not a failure)")
		return
	}
	defer windows.CloseHandle(job)
}
