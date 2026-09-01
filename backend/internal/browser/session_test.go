package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"surf-backend/internal/config"
)

func TestBrowserSessionRoundTrip(t *testing.T) {
	home := t.TempDir()
	controller := &Controller{
		cfg: &config.Config{SurfHome: home},
		tabs: map[int]*Tab{
			1: {ID: 1, URL: "https://example.com"},
			2: {ID: 2, URL: "chrome://settings"},
			3: {ID: 3, URL: "https://example.org"},
		},
		activeID: 3,
		mobile:   true,
		dark:     true,
	}
	if err := controller.SaveSession(); err != nil {
		t.Fatal(err)
	}
	session := loadBrowserSession(home)
	if session.Version != browserSessionVersion || !session.Mobile || !session.Dark || session.Active != 1 {
		t.Fatalf("unexpected session metadata: %#v", session)
	}
	if len(session.Tabs) != 2 || session.Tabs[0] != "https://example.com" || session.Tabs[1] != "https://example.org" {
		t.Fatalf("unexpected tabs: %#v", session.Tabs)
	}
	info, err := os.Stat(filepath.Join(home, "browser-session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session mode = %o, want 600", info.Mode().Perm())
	}
}

func TestEmptyControllerDoesNotEraseBrowserSession(t *testing.T) {
	home := t.TempDir()
	path := browserSessionPath(home)
	want := []byte("existing")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	controller := &Controller{cfg: &config.Config{SurfHome: home}, tabs: map[int]*Tab{}}
	if err := controller.SaveSession(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("empty controller replaced session with %q", got)
	}
}

func TestSurfNewTabReplacesPreviousBrowserSession(t *testing.T) {
	home := t.TempDir()
	path := browserSessionPath(home)
	if err := os.WriteFile(path, []byte(`{"version":1,"tabs":["https://github.com/cloud-hypervisor/cloud-hypervisor"],"active":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	controller := &Controller{
		cfg:      &config.Config{SurfHome: home},
		tabs:     map[int]*Tab{1: {ID: 1, URL: "about:blank#surf-new"}},
		activeID: 1,
	}
	if err := controller.SaveSession(); err != nil {
		t.Fatal(err)
	}
	session := loadBrowserSession(home)
	if len(session.Tabs) != 1 || session.Tabs[0] != "about:blank#surf-new" || session.Active != 0 {
		t.Fatalf("new-tab session was not saved: %#v", session)
	}
}

func TestChangedBrowserSessionIsSavedBeforeShutdown(t *testing.T) {
	home := t.TempDir()
	controller := &Controller{
		cfg:      &config.Config{SurfHome: home},
		tabs:     map[int]*Tab{1: {ID: 1, URL: "https://example.com/changed"}},
		activeID: 1,
	}
	controller.scheduleSessionSave()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(browserSessionPath(home))
		if err == nil {
			var session browserSession
			if json.Unmarshal(data, &session) == nil && len(session.Tabs) == 1 && session.Tabs[0] == "https://example.com/changed" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("changed session was not saved by the debounce deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := controller.flushSession(); err != nil {
		t.Fatal(err)
	}
}

func TestRestoredTargetsKeepOrderAndActiveTabAcrossAttachRace(t *testing.T) {
	controller := &Controller{tabs: map[int]*Tab{}}
	targets := []targetInfo{
		{TargetID: "first", URL: "https://one.example"},
		{TargetID: "second", URL: "https://two.example"},
		{TargetID: "third", URL: "https://three.example"},
	}
	controller.beginRestore(targets, "second")
	if controller.restoreOrder["first"] != 1 || controller.restoreOrder["second"] != 2 || controller.restoreOrder["third"] != 3 {
		t.Fatalf("unexpected restore order: %#v", controller.restoreOrder)
	}
	controller.tabs[1] = &Tab{ID: 1}
	controller.tabs[2] = &Tab{ID: 2}
	controller.tabs[3] = &Tab{ID: 3}
	if handled, activate := controller.finishRestoreTarget("third", 3); !handled || activate != 0 {
		t.Fatalf("third completion = handled %t activate %d", handled, activate)
	}
	if handled, activate := controller.finishRestoreTarget("first", 1); !handled || activate != 0 {
		t.Fatalf("first completion = handled %t activate %d", handled, activate)
	}
	if handled, activate := controller.finishRestoreTarget("second", 2); !handled || activate != 2 {
		t.Fatalf("final completion = handled %t activate %d", handled, activate)
	}
	if controller.restorePending != nil || controller.restoreOrder != nil {
		t.Fatal("restore batch was not released")
	}
}
