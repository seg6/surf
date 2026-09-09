package browser

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
	"surf-backend/internal/transport"
	"surf-backend/internal/web"
)

// Manager is the stable transport/HTTP handler for a server lifetime. The
// operation lock serializes complete handoffs; mu protects only short state
// reads and dispatches, and is never held across a browser launch or shutdown.
type Manager struct {
	op             sync.Mutex
	mu             sync.RWMutex
	cfg            *config.Config
	hub            *transport.Hub
	current        *Controller
	resources      *Controller
	setup          *setupBrowser
	state          protocol.BrowserModeEvent
	clients        map[*transport.Client]bool
	stop           chan struct{}
	done           chan struct{}
	stopped        chan struct{}
	doneOnce       sync.Once
	closeOnce      sync.Once
	stopOnce       sync.Once
	monitorStarted bool // guarded by op
}

func NewManager(cfg *config.Config, hub *transport.Hub, standalone bool) *Manager {
	return &Manager{cfg: cfg, hub: hub, clients: make(map[*transport.Client]bool),
		state: protocol.BrowserModeEvent{Type: "browser-mode", State: "starting", Revision: 1, Host: cfg.ServerName, Standalone: standalone},
		stop:  make(chan struct{}), done: make(chan struct{}), stopped: make(chan struct{})}
}

func (m *Manager) Start() error {
	m.op.Lock()
	defer m.op.Unlock()
	var err error
	if m.state.Standalone {
		m.setup, err = startSetup(m.cfg)
		if err == nil {
			m.setState("setup", "Close this browser to finish standalone setup.", false)
		}
	} else {
		err = m.startStreaming(false)
	}
	if err != nil {
		return err
	}
	m.monitorStarted = true
	go m.monitor()
	return nil
}

func (m *Manager) Status() protocol.BrowserModeEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}
func (m *Manager) Died() <-chan struct{} { return m.done }

// PrepareShutdown binds a dashboard restart/update to the confirmed browser
// revision and prevents another setup request between validation and shutdown.
func (m *Manager) PrepareShutdown(revision uint64) error {
	if !m.op.TryLock() {
		return fmt.Errorf("browser is changing mode; wait before restarting")
	}
	defer m.op.Unlock()
	state := m.Status()
	if state.Standalone || state.Revision != revision {
		return fmt.Errorf("browser state changed; review it before restarting")
	}
	m.stopOnce.Do(func() { close(m.stop) })
	return nil
}

func (m *Manager) setState(state, message string, force bool) {
	m.mu.Lock()
	m.state.State, m.state.Message, m.state.CanForce = state, message, force
	m.state.Revision++
	if m.current != nil {
		m.current.setSuspended(state != "streaming")
	}
	event := m.state
	clients := make(map[*transport.Client]bool, len(m.clients))
	for c, aware := range m.clients {
		clients[c] = aware
	}
	m.mu.Unlock()
	log.Printf("browser mode: state=%s revision=%d standalone=%t", state, event.Revision, event.Standalone)
	for c, aware := range clients {
		c.SetMediaPaused(state != "streaming")
		if aware {
			c.SendJSON(event)
		} else if state != "streaming" {
			c.SendJSON(protocol.TextEvent{Type: "toast", Text: "Browser setup on " + event.Host + ". Browsing is paused; update this client for remote resume controls."})
		}
	}
}

func (m *Manager) Request(action string, revision uint64, force bool) error {
	if action != "open" && action != "resume" {
		return fmt.Errorf("unknown browser action")
	}
	if !m.op.TryLock() {
		return fmt.Errorf("browser is changing mode; wait for it to finish")
	}
	state := m.Status()
	select {
	case <-m.stop:
		m.op.Unlock()
		return fmt.Errorf("Surf is shutting down")
	default:
	}
	if revision != state.Revision {
		m.op.Unlock()
		return fmt.Errorf("browser state changed; review its current status and try again")
	}
	if action == "resume" && state.Standalone {
		m.op.Unlock()
		return fmt.Errorf("standalone setup has no streaming server; close the browser or run surf quit")
	}
	if force && (action != "resume" || !state.CanForce) {
		m.op.Unlock()
		return fmt.Errorf("force close is only available after a normal close failed")
	}
	if action == "open" && (state.State == "failed" || state.CanForce) {
		m.op.Unlock()
		return fmt.Errorf("resume Surf before opening another setup session")
	}
	if action == "resume" && state.State == "streaming" {
		m.op.Unlock()
		return nil
	}
	if action == "open" && state.State == "setup" {
		setup := m.setup
		go func() {
			defer m.op.Unlock()
			if err := setup.focus(); err != nil {
				log.Printf("browser setup focus: %v", err)
			}
		}()
		return nil
	}
	if action == "open" {
		m.setState("opening", "Opening the browser on this computer…", false)
	} else {
		m.setState("resuming", "Closing the local browser and resuming Surf…", false)
	}
	go func() {
		defer m.op.Unlock()
		if action == "open" {
			m.openSetup()
		} else {
			m.resume(force)
		}
	}()
	return nil
}

func (m *Manager) openSetup() {
	m.mu.Lock()
	old := m.current
	m.current = nil
	m.mu.Unlock()
	if old != nil {
		// Commands already in progress finish against the old browser. Queued
		// ones see its suspended flag and are discarded before the snapshot.
		old.inputMu.Lock()
		old.inputMu.Unlock()
		old.touch.cancel(true)
		old.pointer.cancel(true)
		for _, c := range m.connected() {
			old.ClientDisconnected(c)
		}
		if err := old.flushSession(); err != nil {
			old.sessionTimerMu.Lock()
			old.sessionClosed = false
			old.sessionTimerMu.Unlock()
			m.mu.Lock()
			m.current = old
			m.mu.Unlock()
			m.setState("streaming", "Could not save tabs; browser setup was canceled.", false)
			m.reconnect(old)
			return
		}
		if err := cdp.CloseBrowser(old.cdp, old.cmd, false, 10*time.Second); err != nil {
			m.mu.Lock()
			m.current = old
			m.mu.Unlock()
			m.setState("failed", err.Error(), true)
			return
		}
		old.Shutdown()
	}
	setup, err := startSetup(m.cfg)
	if err != nil {
		log.Printf("browser setup launch failed: %v", err)
		if setup != nil {
			m.setup = setup
			m.setState("setup", "Setup could not start and its browser is still closing. Try Resume again before forcing it closed.", true)
			return
		}
		if rollback := m.startStreaming(true); rollback != nil {
			m.setState("failed", "Browser setup failed and Surf could not resume: "+rollback.Error(), false)
		} else {
			m.setState("streaming", "Could not open browser setup: "+err.Error(), false)
		}
		return
	}
	m.setup = setup
	m.setState("setup", "Close all setup windows to resume automatically, or choose Resume on devices.", false)
}

func (m *Manager) resume(force bool) {
	if m.setup != nil {
		_ = m.setup.poll()
		if err := m.setup.close(force, 10*time.Second); err != nil {
			m.setState("setup", err.Error(), true)
			return
		}
		m.setup.client.Close()
		m.setup = nil
	}
	m.mu.Lock()
	old := m.current
	m.current = nil
	m.mu.Unlock()
	if old != nil {
		if err := cdp.CloseBrowser(old.cdp, old.cmd, force, 10*time.Second); err != nil {
			m.mu.Lock()
			m.current = old
			m.mu.Unlock()
			m.setState("failed", err.Error(), true)
			return
		}
		old.Shutdown()
	}
	if err := m.startStreaming(true); err != nil {
		m.setState("failed", "Could not resume Surf: "+err.Error(), false)
	}
}

func (m *Manager) startStreaming(exact bool) error {
	b, err := New(m.cfg, m.hub)
	if err != nil {
		return err
	}
	b.exactSession = exact || b.startupSession.FromSetup
	b.setSuspended(true)
	if err := b.Start(); err != nil {
		return err
	}
	m.mu.Lock()
	m.current = b
	m.resources = b
	m.mu.Unlock()
	m.setState("streaming", "", false)
	m.reconnect(b)
	return nil
}
func (m *Manager) reconnect(b *Controller) {
	for _, c := range m.connected() {
		b.ClientConnected(c)
	}
}
func (m *Manager) connected() []*transport.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cs := make([]*transport.Client, 0, len(m.clients))
	for c := range m.clients {
		cs = append(cs, c)
	}
	return cs
}

func (m *Manager) monitor() {
	defer close(m.stopped)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
		}
		if !m.op.TryLock() {
			continue
		}
		select {
		case <-m.stop:
			m.op.Unlock()
			return
		default:
		}
		if m.setup != nil {
			s := m.setup
			// Polling a hung extension must not lock out Resume or Quit. A
			// snapshot may finish while a handoff starts; revalidate its owner.
			m.op.Unlock()
			closed := false
			select {
			case <-s.client.Closed():
				closed = true
			default:
			}
			if !closed {
				if err := s.poll(); err != nil {
					log.Printf("browser setup observer: %v", err)
				}
				s.mu.Lock()
				closed = s.ready && s.windows == 0
				s.mu.Unlock()
			}
			if !m.op.TryLock() {
				continue
			}
			if m.setup != s {
				m.op.Unlock()
				continue
			}
			select {
			case <-m.stop:
				m.op.Unlock()
				return
			default:
			}
			if closed {
				if m.Status().Standalone {
					_ = s.close(false, 10*time.Second)
					m.doneOnce.Do(func() { close(m.done) })
				} else {
					m.setState("resuming", "Resuming Surf…", false)
					m.resume(false)
				}
			}
		} else if m.Status().State == "streaming" {
			m.mu.RLock()
			b := m.current
			m.mu.RUnlock()
			if b != nil {
				select {
				case <-b.Died():
					m.doneOnce.Do(func() { close(m.done) })
				default:
				}
			}
		}
		m.op.Unlock()
	}
}

func (m *Manager) Shutdown() {
	m.closeOnce.Do(func() {
		m.stopOnce.Do(func() { close(m.stop) })
		m.op.Lock()
		if m.setup != nil {
			_ = m.setup.poll()
			if err := m.setup.close(false, 10*time.Second); err != nil {
				_ = m.setup.close(true, time.Second)
			}
			m.setup.client.Close()
		}
		m.mu.Lock()
		b := m.current
		m.current = nil
		m.mu.Unlock()
		if b != nil {
			b.Shutdown()
		}
		started := m.monitorStarted
		m.op.Unlock()
		if started {
			<-m.stopped
		}
	})
}

func (m *Manager) ClientConnected(c *transport.Client) {
	m.mu.Lock()
	m.clients[c] = false
	b := m.current
	streaming := m.state.State == "streaming"
	c.SetMediaPaused(!streaming)
	m.mu.Unlock()
	if b != nil && streaming {
		b.ClientConnected(c)
	} else {
		c.SendJSON(protocol.TextEvent{Type: "toast", Text: "Browser setup is active on the computer. Browsing is paused."})
	}
}
func (m *Manager) ClientDisconnected(c *transport.Client) {
	m.mu.Lock()
	delete(m.clients, c)
	b := m.current
	m.mu.Unlock()
	if b != nil {
		b.ClientDisconnected(c)
	}
}
func (m *Manager) HandleMessage(c *transport.Client, command protocol.Command) {
	if command.Kind() == "browser-watch" {
		m.mu.Lock()
		m.clients[c] = true
		m.mu.Unlock()
		c.SendJSON(m.Status())
		return
	}
	if resume, ok := command.(*protocol.BrowserResumeCommand); ok {
		if err := m.Request("resume", resume.Revision, resume.Force); err != nil {
			c.SendJSON(protocol.TextEvent{Type: "toast", Text: err.Error()})
			c.SendJSON(m.Status())
		}
		return
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current != nil && m.state.State == "streaming" {
		m.current.HandleMessage(c, command)
	}
}
func (m *Manager) Health() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state.State == "streaming" && m.current != nil {
		return m.current.Health()
	}
	return nil
}
func (m *Manager) Stats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := map[string]any{}
	if m.current != nil {
		stats = m.current.Stats()
	}
	stats["browserMode"] = m.state
	stats["clients"] = m.hub.ClientCount()
	return stats
}

func (m *Manager) RegisterRoutes(s *web.Server) {
	for _, path := range []string{"/tab-icons/", "/uploads", "/downloads/"} {
		path := path
		s.Gated(web.APIRoot+path, func(w http.ResponseWriter, r *http.Request) {
			m.mu.RLock()
			b := m.resources
			streaming := m.state.State == "streaming"
			m.mu.RUnlock()
			if b == nil || (path == "/uploads" && !streaming) {
				http.Error(w, "Browser setup is active; resume Surf first", http.StatusConflict)
				return
			}
			switch path {
			case "/uploads":
				b.handleUpload(w, r)
			case "/downloads/":
				b.handleDownload(w, r)
			default:
				b.handleTabIcon(w, r)
			}
		})
	}
}
