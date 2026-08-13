package browser

import (
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
	"surf-backend/internal/transport"
)

type fakeManagedBrowser struct {
	mu              sync.Mutex
	died            chan struct{}
	shutdownOnce    sync.Once
	shutdownStarted chan struct{}
	shutdownBlock   chan struct{}
	startStarted    chan struct{}
	startBlock      chan struct{}
	startErr        error
	idleSafe        bool
	connected       int
	disconnected    int
	shutdowns       int
	saves           int
}

func newFakeManagedBrowser() *fakeManagedBrowser {
	return &fakeManagedBrowser{
		died: make(chan struct{}), shutdownStarted: make(chan struct{}), idleSafe: true,
	}
}

func (f *fakeManagedBrowser) Start() error {
	if f.startStarted != nil {
		close(f.startStarted)
	}
	if f.startBlock != nil {
		<-f.startBlock
	}
	if f.startErr != nil {
		f.Shutdown()
	}
	return f.startErr
}
func (f *fakeManagedBrowser) Shutdown() {
	f.mu.Lock()
	f.shutdowns++
	f.mu.Unlock()
	f.shutdownOnce.Do(func() {
		close(f.shutdownStarted)
		if f.shutdownBlock != nil {
			<-f.shutdownBlock
		}
		close(f.died)
	})
}
func (f *fakeManagedBrowser) SaveSession() error {
	f.mu.Lock()
	f.saves++
	f.mu.Unlock()
	return nil
}
func (f *fakeManagedBrowser) IdleSafe() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.idleSafe
}
func (f *fakeManagedBrowser) Died() <-chan struct{} { return f.died }
func (f *fakeManagedBrowser) Health() error         { return nil }
func (f *fakeManagedBrowser) Stats() map[string]any { return map[string]any{"tabs": 1} }
func (f *fakeManagedBrowser) ClientConnected(*transport.Client) {
	f.mu.Lock()
	f.connected++
	f.mu.Unlock()
}
func (f *fakeManagedBrowser) ClientDisconnected(*transport.Client) {
	f.mu.Lock()
	f.disconnected++
	f.mu.Unlock()
}
func (f *fakeManagedBrowser) HandleMessage(*transport.Client, protocol.Command) {}
func (f *fakeManagedBrowser) handleUpload(http.ResponseWriter, *http.Request)   {}

func (f *fakeManagedBrowser) crash() { f.shutdownOnce.Do(func() { close(f.died) }) }

func (f *fakeManagedBrowser) counts() (connected, disconnected, shutdowns, saves int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected, f.disconnected, f.shutdowns, f.saves
}

func newTestManager(t *testing.T, idle time.Duration, browsers ...*fakeManagedBrowser) (*Manager, func() int) {
	t.Helper()
	m := NewManager(&config.Config{SurfHome: t.TempDir(), BrowserIdleTimeout: idle}, transport.New())
	m.retryBase = 5 * time.Millisecond
	var mu sync.Mutex
	created := 0
	m.factory = func() (managedBrowser, error) {
		mu.Lock()
		defer mu.Unlock()
		if created >= len(browsers) {
			return nil, errors.New("unexpected browser start")
		}
		browser := browsers[created]
		created++
		return browser, nil
	}
	return m, func() int {
		mu.Lock()
		defer mu.Unlock()
		return created
	}
}

func await(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func TestManagerStartsLazilyAndCancelsIdleShutdown(t *testing.T) {
	browser := newFakeManagedBrowser()
	manager, created := newTestManager(t, 40*time.Millisecond, browser)
	defer manager.Shutdown()
	if created() != 0 || manager.Stats()["browserState"] != "stopped" {
		t.Fatal("manager started Chromium before a client connected")
	}
	first := &transport.Client{}
	manager.ClientConnected(first)
	if created() != 1 {
		t.Fatalf("browser starts = %d, want 1", created())
	}
	manager.ClientDisconnected(first)
	time.Sleep(10 * time.Millisecond)
	second := &transport.Client{}
	manager.ClientConnected(second)
	time.Sleep(50 * time.Millisecond)
	connected, disconnected, shutdowns, _ := browser.counts()
	if connected != 2 || disconnected != 1 || shutdowns != 0 {
		t.Fatalf("counts = connected %d disconnected %d shutdowns %d", connected, disconnected, shutdowns)
	}
	manager.ClientDisconnected(second)
	await(t, "idle browser shutdown", func() bool {
		_, _, shutdowns, saves := browser.counts()
		return shutdowns == 1 && saves == 1
	})
}

func TestManagerReconnectWaitsForBrowserTeardown(t *testing.T) {
	first := newFakeManagedBrowser()
	first.shutdownBlock = make(chan struct{})
	second := newFakeManagedBrowser()
	manager, created := newTestManager(t, 5*time.Millisecond, first, second)
	defer manager.Shutdown()
	one := &transport.Client{}
	manager.ClientConnected(one)
	manager.ClientDisconnected(one)
	select {
	case <-first.shutdownStarted:
	case <-time.After(time.Second):
		t.Fatal("idle shutdown did not begin")
	}
	two := &transport.Client{}
	connected := make(chan struct{})
	go func() {
		manager.ClientConnected(two)
		close(connected)
	}()
	time.Sleep(10 * time.Millisecond)
	if created() != 1 {
		t.Fatalf("replacement started before teardown: %d browsers", created())
	}
	close(first.shutdownBlock)
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("reconnect did not finish after teardown")
	}
	if created() != 2 {
		t.Fatalf("browser starts = %d, want 2", created())
	}
	manager.ClientDisconnected(two)
}

func TestManagerRestartsOnlyBrowserAfterCrash(t *testing.T) {
	first := newFakeManagedBrowser()
	second := newFakeManagedBrowser()
	manager, created := newTestManager(t, time.Minute, first, second)
	defer manager.Shutdown()
	client := &transport.Client{}
	manager.ClientConnected(client)
	first.crash()
	await(t, "replacement browser", func() bool { return created() == 2 })
	await(t, "client attachment to replacement", func() bool {
		connected, _, _, _ := second.counts()
		return connected == 1
	})
	stats := manager.Stats()
	if stats["browserState"] != "running" || stats["browserRestarts"] != uint64(1) {
		t.Fatalf("unexpected crash recovery stats: %#v", stats)
	}
	manager.ClientDisconnected(client)
}

func TestManagerDefersIdleShutdownForDownload(t *testing.T) {
	browser := newFakeManagedBrowser()
	browser.idleSafe = false
	manager, _ := newTestManager(t, 8*time.Millisecond, browser)
	defer manager.Shutdown()
	client := &transport.Client{}
	manager.ClientConnected(client)
	manager.ClientDisconnected(client)
	time.Sleep(20 * time.Millisecond)
	_, _, shutdowns, _ := browser.counts()
	if shutdowns != 0 {
		t.Fatal("browser stopped while download was active")
	}
	browser.mu.Lock()
	browser.idleSafe = true
	browser.mu.Unlock()
	await(t, "shutdown after download", func() bool {
		_, _, shutdowns, _ := browser.counts()
		return shutdowns == 1
	})
}

func TestManagerZeroIdleTimeoutKeepsBrowserWarm(t *testing.T) {
	browser := newFakeManagedBrowser()
	manager, _ := newTestManager(t, 0, browser)
	client := &transport.Client{}
	manager.ClientConnected(client)
	manager.ClientDisconnected(client)
	time.Sleep(20 * time.Millisecond)
	_, _, shutdowns, _ := browser.counts()
	if shutdowns != 0 {
		t.Fatal("zero timeout parked the browser")
	}
	manager.Shutdown()
}

func TestManagerShutdownWaitsForConcurrentStart(t *testing.T) {
	browser := newFakeManagedBrowser()
	browser.startStarted = make(chan struct{})
	browser.startBlock = make(chan struct{})
	manager, _ := newTestManager(t, time.Minute, browser)
	connected := make(chan struct{})
	go func() {
		manager.ClientConnected(&transport.Client{})
		close(connected)
	}()
	select {
	case <-browser.startStarted:
	case <-time.After(time.Second):
		t.Fatal("browser start did not begin")
	}
	shutdown := make(chan struct{})
	go func() {
		manager.Shutdown()
		close(shutdown)
	}()
	time.Sleep(10 * time.Millisecond)
	select {
	case <-shutdown:
		t.Fatal("shutdown returned while browser start was still running")
	default:
	}
	close(browser.startBlock)
	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after browser start")
	}
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("connecting goroutine did not finish")
	}
	_, _, shutdowns, _ := browser.counts()
	if shutdowns != 1 {
		t.Fatalf("shutdowns = %d, want 1", shutdowns)
	}
}

func TestManagerRetriesImmediatelyAfterProfileRecovery(t *testing.T) {
	failed := newFakeManagedBrowser()
	failed.startErr = errors.New("profile would not open")
	running := newFakeManagedBrowser()
	manager, created := newTestManager(t, time.Minute, failed, running)
	recoveries := 0
	manager.SetStartupRecovery(func(error) bool {
		recoveries++
		return true
	}, nil)
	browser, _, err := manager.ensureRunning()
	if err != nil {
		t.Fatal(err)
	}
	if browser != running || created() != 2 || recoveries != 1 {
		t.Fatalf("browser=%p starts=%d recoveries=%d", browser, created(), recoveries)
	}
	manager.Shutdown()
}
