package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"surf-backend/internal/cdp"
	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
	"surf-backend/internal/testhost"
	"surf-backend/internal/transport"
)

func TestSetupSessionFiltersInternalTabs(t *testing.T) {
	previous := browserSession{Mobile: true, Dark: true}
	got := filteredSetupSession([]string{"https://one.test", "chrome://settings", "https://one.test", "chrome://newtab/"}, 2, previous)
	if len(got.Tabs) != 3 || got.Active != 1 || !got.Mobile || !got.Dark || got.Tabs[2] != "about:blank#surf-new" {
		t.Fatalf("%+v", got)
	}
	empty := filteredSetupSession([]string{"chrome://settings"}, 0, previous)
	if len(empty.Tabs) != 1 || empty.Tabs[0] != "about:blank#surf-new" {
		t.Fatalf("%+v", empty)
	}
}

func TestManagedSetupHandoffIntegration(t *testing.T) {
	chrome := visibleTestBrowser(t)
	home := t.TempDir()
	cfg := &config.Config{SurfHome: home, Profile: filepath.Join(home, "profile"), ChromePath: chrome, ViewW: 320, ViewH: 480}
	manager := NewManager(cfg, transport.New(), false)
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown()
	for i := 0; i < 2; i++ {
		before := manager.Status()
		if err := manager.Request("open", before.Revision, false); err != nil {
			t.Fatal(err)
		}
		state := waitBrowserState(t, manager, "setup")
		if err := manager.Request("resume", before.Revision, false); err == nil {
			t.Fatal("stale resume accepted")
		}
		if err := manager.Request("resume", state.Revision, true); err == nil {
			t.Fatal("force allowed without failed graceful close")
		}
		if err := manager.Request("resume", state.Revision, false); err != nil {
			t.Fatal(err)
		}
		waitBrowserState(t, manager, "streaming")
		if err := manager.Health(); err != nil {
			t.Fatal(err)
		}
	}
	// Closing normal setup windows resumes without an explicit request, and
	// the same profile retains cookies written by the visible browser.
	if err := manager.Request("open", manager.Status().Revision, false); err != nil {
		t.Fatal(err)
	}
	waitBrowserState(t, manager, "setup")
	manager.op.Lock()
	setup := manager.setup
	_, err := setup.client.Call("", "Storage.setCookies", map[string]any{"cookies": []any{map[string]any{"name": "surf-setup-test", "value": "retained", "url": "https://example.test"}}})
	if err == nil {
		err = setup.client.Dispatch(setup.sessionID, "Runtime.evaluate", map[string]any{"expression": `chrome.windows.getAll({windowTypes:['normal']}).then(ws=>Promise.all(ws.map(w=>chrome.windows.remove(w.id))))`})
	}
	manager.op.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	waitBrowserState(t, manager, "streaming")
	manager.op.Lock()
	raw, err := manager.current.cdp.Call("", "Storage.getCookies", nil)
	manager.op.Unlock()
	if err != nil || !strings.Contains(string(raw), "surf-setup-test") || !strings.Contains(string(raw), "retained") {
		t.Fatalf("profile cookies lost: %s %v", raw, err)
	}
}

func TestManagedSetupKeepsClientConnectionIntegration(t *testing.T) {
	chrome := visibleTestBrowser(t)
	home := t.TempDir()
	hub := transport.New()
	m := NewManager(&config.Config{
		SurfHome: home, Profile: filepath.Join(home, "profile"), ChromePath: chrome,
		ViewW: 320, ViewH: 480, StreamBitrateK: 16000, StreamQuantizer: 12,
	}, hub, false)
	hub.SetHandler(m)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	server := httptest.NewServer(hub)
	defer server.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	modes := make(chan protocol.BrowserModeEvent, 32)
	frames := make(chan int, 8)
	errors := make(chan error, 1)
	go func() {
		paused := false
		generation := 0
		for {
			kind, data, err := client.ReadMessage()
			if err != nil {
				errors <- err
				return
			}
			if kind == websocket.BinaryMessage {
				if paused {
					errors <- fmt.Errorf("media delivered after the paused boundary")
					return
				}
				if len(data) > 5 && data[4] == protocol.FrameTypeVideo && generation != 0 {
					select {
					case frames <- generation:
					default:
					}
				}
				continue
			}
			var event struct {
				Type       string `json:"t"`
				State      string
				Generation int
			}
			if err := json.Unmarshal(data, &event); err != nil {
				errors <- err
				return
			}
			if event.Type == "browser-mode" {
				var mode protocol.BrowserModeEvent
				if err := json.Unmarshal(data, &mode); err != nil {
					errors <- err
					return
				}
				paused = mode.State != "streaming"
				modes <- mode
			} else if event.Type == "video-config" && event.State == "ready" {
				generation = event.Generation
			}
		}
	}()
	if err := client.WriteJSON(map[string]any{"t": "browser-watch"}); err != nil {
		t.Fatal(err)
	}
	waitMode := func(want string) protocol.BrowserModeEvent {
		deadline := time.NewTimer(15 * time.Second)
		defer deadline.Stop()
		for {
			select {
			case mode := <-modes:
				if mode.State == want {
					return mode
				}
			case err := <-errors:
				t.Fatalf("socket closed during %s: %v", want, err)
			case <-deadline.C:
				t.Fatalf("socket did not receive %s", want)
			}
		}
	}
	waitFrame := func(previous int) int {
		deadline := time.NewTimer(15 * time.Second)
		defer deadline.Stop()
		for {
			select {
			case generation := <-frames:
				if generation != previous {
					return generation
				}
			case err := <-errors:
				t.Fatalf("frame stream: %v", err)
			case <-deadline.C:
				t.Fatal("new browser did not deliver fresh video")
			}
		}
	}
	initial := waitMode("streaming")
	firstGeneration := waitFrame(0)
	if err := m.Request("open", initial.Revision, false); err != nil {
		t.Fatal(err)
	}
	setup := waitMode("setup")
	waitBrowserState(t, m, "setup")
	if hub.ClientCount() != 1 {
		t.Fatal("handoff dropped the connection")
	}
	// A client arriving after setup started must receive the paused state,
	// not subscribe to a stale browser or wait forever for video.
	late, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer late.Close()
	if err := late.WriteJSON(map[string]any{"t": "browser-watch"}); err != nil {
		t.Fatal(err)
	}
	late.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		kind, data, err := late.ReadMessage()
		if err != nil || kind == websocket.BinaryMessage {
			t.Fatalf("late client did not enter setup: kind=%d err=%v", kind, err)
		}
		var mode protocol.BrowserModeEvent
		if json.Unmarshal(data, &mode) == nil && mode.Type == "browser-mode" {
			if mode.State != "setup" || mode.Revision != setup.Revision {
				t.Fatalf("late client got stale mode: %+v", mode)
			}
			break
		}
	}
	late.Close()
	if err := client.WriteJSON(map[string]any{"t": "browser-resume", "revision": setup.Revision, "force": false}); err != nil {
		t.Fatal(err)
	}
	waitMode("streaming")
	waitFrame(firstGeneration)
	if hub.ClientCount() != 1 {
		t.Fatal("resume replaced the connection")
	}
}

func TestStandaloneSetupExitsWithoutStreamingIntegration(t *testing.T) {
	chrome := visibleTestBrowser(t)
	home := t.TempDir()
	cfg := &config.Config{SurfHome: home, Profile: filepath.Join(home, "profile"), ChromePath: chrome, ViewW: 320, ViewH: 480}
	m := NewManager(cfg, transport.New(), true)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	if err := m.Request("resume", m.Status().Revision, false); err == nil {
		t.Fatal("standalone resume accepted")
	}
	m.op.Lock()
	_, err := m.setup.client.Call("", "Storage.setCookies", map[string]any{"cookies": []any{map[string]any{"name": "standalone-sign-in", "value": "retained", "url": "https://example.test"}}})
	if err == nil {
		err = m.setup.client.Dispatch("", "Browser.close", nil)
	}
	m.op.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.Died():
	case <-time.After(10 * time.Second):
		t.Fatal("standalone did not finish")
	}
	if m.Status().State == "streaming" || m.current != nil {
		t.Fatal("standalone launched streaming")
	}
	m.Shutdown()
	// A later, ordinary server launch must consume standalone changes too.
	next := NewManager(cfg, transport.New(), false)
	if err := next.Start(); err != nil {
		t.Fatal(err)
	}
	defer next.Shutdown()
	raw, err := next.current.cdp.Call("", "Storage.getCookies", nil)
	if err != nil || !strings.Contains(string(raw), "standalone-sign-in") {
		t.Fatalf("standalone sign-in lost: %s %v", raw, err)
	}
}

func TestSetupWithoutDisplayRollsBackIntegration(t *testing.T) {
	chrome := visibleTestBrowser(t)
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	home := t.TempDir()
	m := NewManager(&config.Config{SurfHome: home, Profile: filepath.Join(home, "profile"), ChromePath: chrome, ViewW: 320, ViewH: 480}, transport.New(), false)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	if err := m.Request("open", m.Status().Revision, false); err != nil {
		t.Fatal(err)
	}
	state := waitBrowserState(t, m, "streaming")
	if !strings.Contains(state.Message, "desktop session") || m.Health() != nil {
		t.Fatalf("failed to roll back: %+v", state)
	}
}

func waitBrowserState(t *testing.T, m *Manager, want string) protocol.BrowserModeEvent {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		state := m.Status()
		if state.State == want {
			// State is published just before the serial operation releases its lock.
			if m.op.TryLock() {
				m.op.Unlock()
				return state
			}
		}
		if state.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("waiting for %s: %+v", want, state)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestVisibleSetupIntegration(t *testing.T) {
	chrome := visibleTestBrowser(t)
	home := t.TempDir()
	cfg := &config.Config{SurfHome: home, Profile: filepath.Join(home, "profile"), ChromePath: chrome}
	want := browserSession{Version: 1, Tabs: []string{"about:blank", "about:blank#surf-new"}, Active: 1, Mobile: true, FromSetup: true}
	if err := writeBrowserSession(home, want); err != nil {
		t.Fatal(err)
	}
	setup, err := startSetup(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cdp.CloseBrowser(setup.client, setup.child, true, time.Second); setup.client.Close() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := setup.poll(); err != nil {
			t.Fatal(err)
		}
		setup.mu.Lock()
		got := setup.snapshot
		ready := setup.ready
		setup.mu.Unlock()
		if ready && sameSession(got, want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("got=%+v ready=%t want=%+v", got, ready, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := setup.focus(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "<!doctype html><title>Setup test</title>")
	}))
	defer server.Close()
	base, _ := json.Marshal(server.URL)
	expression := `(async()=>{const old=await chrome.tabs.query({});const base=` + string(base) + `;const a=await chrome.tabs.create({url:base+'/one'});const b=await chrome.tabs.create({url:base+'/two'});await chrome.tabs.move(b.id,{index:0});await chrome.tabs.create({url:'chrome://settings'});await chrome.tabs.remove(old.map(t=>t.id));const extra=await chrome.windows.create({url:base+'/closed-window'});await chrome.windows.remove(extra.id);await chrome.tabs.update(a.id,{active:true});})()`
	raw, err := setup.client.Call(setup.sessionID, "Runtime.evaluate", map[string]any{"expression": expression, "awaitPromise": true})
	if err != nil || strings.Contains(string(raw), "exceptionDetails") {
		t.Fatalf("local tab edits: %s %v", raw, err)
	}
	want.Tabs, want.Active = []string{server.URL + "/two", server.URL + "/one"}, 1
	deadline = time.Now().Add(5 * time.Second)
	for {
		if err := setup.poll(); err != nil {
			t.Fatal(err)
		}
		if sameSession(loadBrowserSession(home), want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("local tab changes not persisted: %+v want %+v", loadBrowserSession(home), want)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := cdp.CloseBrowser(setup.client, setup.child, false, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := loadBrowserSession(home); !sameSession(got, want) {
		t.Fatalf("session lost: %+v", got)
	}
}

func visibleTestBrowser(t *testing.T) string { return testhost.VisibleBrowser(t) }
