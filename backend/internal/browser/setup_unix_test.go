//go:build !windows

package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"surf-backend/internal/config"
	"surf-backend/internal/transport"
)

func TestSetupForceRequiresFailedGracefulCloseIntegration(t *testing.T) {
	chrome := visibleTestBrowser(t)
	home := t.TempDir()
	m := NewManager(&config.Config{SurfHome: home, Profile: filepath.Join(home, "profile"), ChromePath: chrome, ViewW: 320, ViewH: 480}, transport.New(), false)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	if err := m.Request("open", m.Status().Revision, false); err != nil {
		t.Fatal(err)
	}
	state := waitBrowserState(t, m, "setup")
	m.op.Lock()
	raw, err := m.setup.client.Call("", "SystemInfo.getProcessInfo", nil)
	m.op.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Processes []struct {
			Type string
			ID   int
		} `json:"processInfo"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	pid := 0
	for _, p := range info.Processes {
		if p.Type == "browser" {
			pid = p.ID
		}
	}
	if pid <= 0 {
		t.Fatal("no owned test browser")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Release()
	// Suspend only this test's verified browser to simulate a hung GUI.
	if err := process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	defer process.Signal(syscall.SIGCONT)
	if err := m.Request("resume", state.Revision, false); err != nil {
		t.Fatal(err)
	}
	failed := waitBrowserState(t, m, "setup")
	if !failed.CanForce || failed.Revision <= state.Revision {
		t.Fatalf("no explicit recovery state: %+v", failed)
	}
	if err := m.Request("resume", state.Revision, true); err == nil {
		t.Fatal("stale force accepted")
	}
	if err := m.Request("resume", failed.Revision, true); err != nil {
		t.Fatal(err)
	}
	waitBrowserState(t, m, "streaming")
}
