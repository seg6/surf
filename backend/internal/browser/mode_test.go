package browser

import (
	"testing"

	"surf-backend/internal/config"
	"surf-backend/internal/transport"
)

func TestBrowserModeRejectsUnsafeActions(t *testing.T) {
	m := NewManager(&config.Config{}, transport.New(), false)
	m.setState("setup", "", false)
	revision := m.Status().Revision
	for _, action := range []struct {
		action   string
		revision uint64
		force    bool
	}{{"open", revision - 1, false}, {"resume", revision, true}, {"delete", revision, false}} {
		if err := m.Request(action.action, action.revision, action.force); err == nil {
			t.Fatalf("accepted %+v", action)
		}
	}
	m.op.Lock()
	if err := m.Request("resume", revision, false); err == nil {
		t.Fatal("concurrent action accepted")
	}
	m.op.Unlock()
	standalone := NewManager(&config.Config{}, transport.New(), true)
	if err := standalone.Request("resume", 1, false); err == nil {
		t.Fatal("standalone started streaming")
	}
	if err := m.PrepareShutdown(revision - 1); err == nil {
		t.Fatal("stale restart accepted")
	}
	if err := standalone.PrepareShutdown(1); err == nil {
		t.Fatal("standalone restarted")
	}
	if err := m.PrepareShutdown(revision); err != nil {
		t.Fatal(err)
	}
	if err := m.Request("resume", revision, false); err == nil {
		t.Fatal("setup raced confirmed shutdown")
	}
	m.Shutdown() // shutdown also works before Start and after a reservation.
}
