package browser

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
	"surf-backend/internal/transport"
	"surf-backend/internal/web"
)

type browserState string

const (
	browserStopped  browserState = "stopped"
	browserStarting browserState = "starting"
	browserRunning  browserState = "running"
	browserStopping browserState = "stopping"
)

var errManagerClosed = errors.New("browser manager closed")

type managedBrowser interface {
	Start() error
	Shutdown()
	SaveSession() error
	IdleSafe() bool
	Died() <-chan struct{}
	Health() error
	Stats() map[string]any
	ClientConnected(*transport.Client)
	ClientDisconnected(*transport.Client)
	HandleMessage(*transport.Client, protocol.Command)
	handleUpload(http.ResponseWriter, *http.Request)
}

type browserFactory func() (managedBrowser, error)

// Manager keeps the Surf daemon independent from Chromium's lifetime. Only
// authenticated native sockets make the browser hot; management, pairing,
// updates, logs and clipboard APIs remain available while it is parked.
type Manager struct {
	cfg     *config.Config
	hub     *transport.Hub
	factory browserFactory

	mu            sync.Mutex
	state         browserState
	browser       managedBrowser
	generation    uint64
	transition    chan struct{}
	clients       map[*transport.Client]uint64
	idleTimer     *time.Timer
	idleUntil     time.Time
	retryTimer    *time.Timer
	retryBase     time.Duration
	closing       bool
	lastError     string
	starts        uint64
	restarts      uint64
	startFailures int
	rapidCrashes  int
	lastStarted   time.Time
	session       browserSession

	onStartFailure func(error) bool
	onStartSuccess func()
}

func NewManager(cfg *config.Config, hub *transport.Hub) *Manager {
	m := &Manager{
		cfg: cfg, hub: hub, state: browserStopped,
		clients: map[*transport.Client]uint64{}, retryBase: time.Second,
		session: loadBrowserSession(cfg.SurfHome),
	}
	m.factory = func() (managedBrowser, error) { return New(cfg, hub) }
	return m
}

// SetStartupRecovery installs the profile recovery policy owned by the app
// package. It is configured before the Hub can accept clients.
func (m *Manager) SetStartupRecovery(onFailure func(error) bool, onSuccess func()) {
	m.mu.Lock()
	m.onStartFailure = onFailure
	m.onStartSuccess = onSuccess
	m.mu.Unlock()
}

func (m *Manager) ClientConnected(client *transport.Client) {
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return
	}
	m.clients[client] = 0
	m.cancelIdleLocked()
	m.cancelRetryLocked()
	m.mu.Unlock()
	m.attachClient(client)
}

func (m *Manager) attachClient(client *transport.Client) {
	browser, generation, err := m.ensureRunning()
	if err != nil {
		if errors.Is(err, errManagerClosed) {
			return
		}
		log.Printf("browser: unavailable for client: %v", err)
		client.SendJSON(protocol.VideoConfigEvent{Type: "video-config", State: "unavailable", Reason: "browser-start-failed"})
		m.scheduleRetry()
		return
	}
	m.mu.Lock()
	attached, connected := m.clients[client]
	if !connected || attached == generation || m.state != browserRunning || m.browser != browser || m.generation != generation {
		m.mu.Unlock()
		return
	}
	// Keep attachment and disconnection ordered. Controller.ClientConnected
	// only queues state and a delayed subscription; it does not call Manager.
	m.clients[client] = generation
	browser.ClientConnected(client)
	m.mu.Unlock()
}

func (m *Manager) ClientDisconnected(client *transport.Client) {
	m.mu.Lock()
	generation, connected := m.clients[client]
	delete(m.clients, client)
	browser := m.browser
	active := connected && generation != 0 && m.state == browserRunning && generation == m.generation
	m.mu.Unlock()
	if active {
		browser.ClientDisconnected(client)
	}
	m.mu.Lock()
	if len(m.clients) == 0 {
		m.cancelRetryLocked()
		m.scheduleIdleLocked(m.cfg.BrowserIdleTimeout)
	}
	m.mu.Unlock()
}

func (m *Manager) HandleMessage(client *transport.Client, command protocol.Command) {
	m.mu.Lock()
	generation, connected := m.clients[client]
	browser := m.browser
	ready := connected && generation != 0 && m.state == browserRunning && generation == m.generation
	m.mu.Unlock()
	if ready {
		browser.HandleMessage(client, command)
	}
}

func (m *Manager) ensureRunning() (managedBrowser, uint64, error) {
	for {
		m.mu.Lock()
		if m.closing {
			m.mu.Unlock()
			return nil, 0, errManagerClosed
		}
		switch m.state {
		case browserRunning:
			browser, generation := m.browser, m.generation
			m.mu.Unlock()
			return browser, generation, nil
		case browserStarting, browserStopping:
			transition := m.transition
			m.mu.Unlock()
			<-transition
			continue
		case browserStopped:
			m.state = browserStarting
			m.transition = make(chan struct{})
			transition := m.transition
			m.mu.Unlock()
			return m.start(transition)
		default:
			m.mu.Unlock()
			return nil, 0, fmt.Errorf("invalid browser state")
		}
	}
}

func (m *Manager) start(transition chan struct{}) (managedBrowser, uint64, error) {
	for {
		m.mu.Lock()
		if m.closing {
			m.state = browserStopped
			m.browser = nil
			close(transition)
			m.mu.Unlock()
			return nil, 0, errManagerClosed
		}
		m.mu.Unlock()
		browser, err := m.factory()
		if err == nil {
			err = browser.Start()
		}
		if err != nil {
			m.mu.Lock()
			if m.closing {
				m.state = browserStopped
				m.browser = nil
				close(transition)
				m.mu.Unlock()
				return nil, 0, errManagerClosed
			}
			recoverStart := m.onStartFailure
			m.mu.Unlock()
			if recoverStart != nil && recoverStart(err) {
				log.Printf("browser: startup recovery prepared a clean profile; retrying")
				continue
			}
			m.mu.Lock()
			m.lastError = err.Error()
			m.startFailures++
			m.state = browserStopped
			m.browser = nil
			close(transition)
			m.mu.Unlock()
			return nil, 0, err
		}

		m.mu.Lock()
		if m.closing {
			m.mu.Unlock()
			m.saveSession(browser, "aborted start")
			browser.Shutdown()
			m.mu.Lock()
			m.state = browserStopped
			m.browser = nil
			close(transition)
			m.mu.Unlock()
			return nil, 0, errManagerClosed
		}
		m.browser = browser
		m.state = browserRunning
		m.generation++
		m.starts++
		generation := m.generation
		m.lastError = ""
		m.startFailures = 0
		m.lastStarted = time.Now()
		m.cancelRetryLocked()
		onSuccess := m.onStartSuccess
		close(transition)
		m.mu.Unlock()
		if onSuccess != nil {
			onSuccess()
		}
		log.Printf("browser: started on demand generation=%d", generation)
		go m.watch(browser, generation)
		return browser, generation, nil
	}
}

func (m *Manager) watch(browser managedBrowser, generation uint64) {
	<-browser.Died()
	m.mu.Lock()
	if m.closing || m.state != browserRunning || m.browser != browser || m.generation != generation {
		m.mu.Unlock()
		return
	}
	m.state = browserStopping
	m.transition = make(chan struct{})
	transition := m.transition
	for client := range m.clients {
		m.clients[client] = 0
	}
	m.lastError = "chromium connection lost"
	m.restarts++
	if time.Since(m.lastStarted) < 30*time.Second {
		m.rapidCrashes++
	} else {
		m.rapidCrashes = 0
	}
	connected := len(m.clients) > 0
	m.mu.Unlock()

	m.saveSession(browser, "unexpected exit")
	browser.Shutdown()
	m.mu.Lock()
	if m.browser == browser && m.generation == generation {
		m.browser = nil
		m.state = browserStopped
		close(transition)
	}
	m.mu.Unlock()
	log.Printf("browser: exited unexpectedly; daemon remains available")
	if connected {
		m.scheduleRetry()
	}
}

func (m *Manager) scheduleRetry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || len(m.clients) == 0 || m.retryTimer != nil {
		return
	}
	failures := max(m.startFailures, m.rapidCrashes)
	delay := m.retryBase << min(max(failures-1, 0), 6)
	if delay > time.Minute {
		delay = time.Minute
	}
	m.retryTimer = time.AfterFunc(delay, func() {
		m.mu.Lock()
		m.retryTimer = nil
		clients := make([]*transport.Client, 0, len(m.clients))
		for client := range m.clients {
			clients = append(clients, client)
		}
		m.mu.Unlock()
		for _, client := range clients {
			m.attachClient(client)
		}
	})
}

func (m *Manager) scheduleIdleLocked(delay time.Duration) {
	if m.closing || delay <= 0 || len(m.clients) != 0 || m.state != browserRunning {
		return
	}
	m.cancelIdleLocked()
	m.idleUntil = time.Now().Add(delay)
	generation := m.generation
	m.idleTimer = time.AfterFunc(delay, func() { m.parkIdle(generation) })
	log.Printf("browser: idle shutdown scheduled in %s", delay)
}

func (m *Manager) parkIdle(generation uint64) {
	m.mu.Lock()
	m.idleTimer = nil
	m.idleUntil = time.Time{}
	if m.closing || len(m.clients) != 0 || m.state != browserRunning || m.generation != generation {
		m.mu.Unlock()
		return
	}
	browser := m.browser
	if !browser.IdleSafe() {
		delay := m.cfg.BrowserIdleTimeout
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		m.scheduleIdleLocked(delay)
		m.mu.Unlock()
		log.Printf("browser: idle shutdown deferred while downloads are active")
		return
	}
	m.state = browserStopping
	m.transition = make(chan struct{})
	transition := m.transition
	m.mu.Unlock()

	m.saveSession(browser, "idle shutdown")
	browser.Shutdown()
	m.mu.Lock()
	if m.browser == browser && m.generation == generation {
		m.browser = nil
		m.state = browserStopped
		close(transition)
	}
	m.mu.Unlock()
	log.Printf("browser: stopped after idle grace")
}

func (m *Manager) cancelIdleLocked() {
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}
	m.idleUntil = time.Time{}
}

func (m *Manager) cancelRetryLocked() {
	if m.retryTimer != nil {
		m.retryTimer.Stop()
		m.retryTimer = nil
	}
}

// Shutdown ends the active browser lifetime and waits for any concurrent
// start/stop transition. The process implementation owns the cross-platform
// graceful-close then process-tree-kill fallback.
func (m *Manager) Shutdown() {
	for {
		m.mu.Lock()
		m.closing = true
		m.cancelIdleLocked()
		m.cancelRetryLocked()
		switch m.state {
		case browserStarting, browserStopping:
			transition := m.transition
			m.mu.Unlock()
			<-transition
			continue
		case browserStopped:
			m.mu.Unlock()
			return
		case browserRunning:
			browser, generation := m.browser, m.generation
			m.state = browserStopping
			m.transition = make(chan struct{})
			transition := m.transition
			m.mu.Unlock()
			m.saveSession(browser, "server shutdown")
			browser.Shutdown()
			m.mu.Lock()
			if m.browser == browser && m.generation == generation {
				m.browser = nil
				m.state = browserStopped
				close(transition)
			}
			m.mu.Unlock()
			return
		}
	}
}

// Health is daemon health, not browser residency. A deliberately parked or
// currently restarting Chromium must not make a desktop supervisor restart
// the entire Surf process.
func (m *Manager) Health() error { return nil }

func (m *Manager) Stats() map[string]any {
	m.mu.Lock()
	state, browser := m.state, m.browser
	idleUntil := m.idleUntil
	lastError := m.lastError
	starts, restarts := m.starts, m.restarts
	clients := len(m.clients)
	session := m.session
	m.mu.Unlock()
	stats := map[string]any{
		"clients": clients, "browserState": string(state),
		"browserStarts": starts, "browserRestarts": restarts,
		"browserIdleTimeoutMs": m.cfg.BrowserIdleTimeout.Milliseconds(),
		"browserLastError":     lastError,
	}
	if !idleUntil.IsZero() {
		stats["browserIdleRemainingMs"] = max(time.Until(idleUntil).Milliseconds(), 0)
	}
	if state == browserRunning && browser != nil {
		for key, value := range browser.Stats() {
			stats[key] = value
		}
		if err := browser.Health(); err != nil {
			stats["browserHealth"] = err.Error()
		} else {
			stats["browserHealth"] = "ok"
		}
	} else {
		stats["tabs"] = len(session.Tabs)
		if len(session.Tabs) > 0 && session.Active >= 0 && session.Active < len(session.Tabs) {
			stats["activeURL"] = session.Tabs[session.Active]
		}
	}
	for key, value := range m.hub.Stats() {
		if _, exists := stats[key]; !exists {
			stats[key] = value
		}
	}
	return stats
}

func (m *Manager) saveSession(browser managedBrowser, reason string) {
	if err := browser.SaveSession(); err != nil {
		log.Printf("browser: save session for %s: %v", reason, err)
		return
	}
	session := loadBrowserSession(m.cfg.SurfHome)
	if len(session.Tabs) == 0 {
		return
	}
	m.mu.Lock()
	m.session = session
	m.mu.Unlock()
}

// RegisterRoutes registers browser-adjacent authenticated HTTP routes once;
// handlers resolve the current browser generation at request time.
func (m *Manager) RegisterRoutes(server *web.Server) {
	server.Gated(web.APIRoot+"/tab-icons/", func(w http.ResponseWriter, r *http.Request) {
		browser := m.currentController()
		if browser == nil {
			http.NotFound(w, r)
			return
		}
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, web.APIRoot+"/tab-icons/"))
		browser.mu.Lock()
		var icon *favicon
		if tab := browser.tabs[id]; tab != nil {
			icon = browser.icons[tab.IconKey]
		}
		browser.mu.Unlock()
		if icon == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", icon.ctype)
		w.Header().Set("Cache-Control", "public, max-age=604800")
		_, _ = w.Write(icon.data)
	})
	server.Gated(web.APIRoot+"/uploads", func(w http.ResponseWriter, r *http.Request) {
		browser := m.currentController()
		if browser == nil {
			http.Error(w, "browser is not running", http.StatusServiceUnavailable)
			return
		}
		browser.handleUpload(w, r)
	})
	server.Gated(web.APIRoot+"/downloads/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(strings.TrimPrefix(r.URL.Path, web.APIRoot+"/downloads/"))
		if name == "." || name == "/" || strings.HasPrefix(name, ".") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
		http.ServeFile(w, r, filepath.Join(m.cfg.DownloadsDir, name))
	})
}

func (m *Manager) currentController() *Controller {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != browserRunning {
		return nil
	}
	browser, _ := m.browser.(*Controller)
	return browser
}
