// Package cdp is a thin Chrome DevTools Protocol client: launch headless
// Chromium, dial its browser WebSocket, correlate request/response by id,
// and fan out events. Flat sessions only (Target.attachToTarget with
// flatten:true); the small CDP surface we use doesn't justify a generated
// protocol binding.
package cdp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"surf-backend/internal/process"
)

type Event struct {
	Method    string
	SessionID string
	Params    json.RawMessage
}

type Client struct {
	conn    *websocket.Conn
	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan response

	handlerMu sync.RWMutex
	handler   func(Event)

	// Events are queued and dispatched off the read loop, so a handler that
	// blocks (e.g. waiting on a lock held across a Call) can never stall
	// response delivery and deadlock the client.
	evMu     sync.Mutex
	evQueue  []Event
	evSignal chan struct{}

	closed chan struct{}
}

type response struct {
	result json.RawMessage
	err    error
}

type envelope struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

var devtoolsRe = regexp.MustCompile(`DevTools listening on (ws://\S+)`)

const (
	browserStartupTimeout = 30 * time.Second
	stderrTailLines       = 12
	stderrTailLineBytes   = 1024
)

// boundedLineTail preserves enough Chromium stderr to explain a failed
// launch without allowing a noisy browser process to grow Surf's memory or
// durable logs without bound.
type boundedLineTail struct {
	mu    sync.Mutex
	lines []string
}

func (t *boundedLineTail) add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if len(line) > stderrTailLineBytes {
		line = line[len(line)-stderrTailLineBytes:]
	}
	t.mu.Lock()
	t.lines = append(t.lines, line)
	if len(t.lines) > stderrTailLines {
		t.lines = append([]string(nil), t.lines[len(t.lines)-stderrTailLines:]...)
	}
	t.mu.Unlock()
}

func (t *boundedLineTail) string() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, " | ")
}

type LaunchConfig struct {
	ChromePath     string
	Profile        string
	W, H           int
	NoSandbox      bool
	ExtensionPaths []string
	ExtraArgs      []string
}

// Args builds the managed Chrome headless-new launch flags.
func (cfg LaunchConfig) Args() []string {
	args := []string{
		"--remote-debugging-port=0",
		"--user-data-dir=" + cfg.Profile,
		"--disable-blink-features=AutomationControlled",
		"--disable-popup-blocking", "--no-first-run", "--no-default-browser-check",
		"--disable-session-crashed-bubble", "--hide-crash-restore-bubble", "--noerrdialogs",
		// Surf owns the complete browser lifetime. Chromium background mode can
		// otherwise retain a profile-owning process after Browser.close.
		"--disable-background-mode",
		"--hide-scrollbars",
		"--disable-sync",
		// The anti-throttling set puppeteer always passed: without these,
		// Chromium can treat itself as backgrounded/occluded and throttle or
		// stop producing compositor frames. Headless removes most of the
		// reasons this would happen, but these are harmless to keep as a
		// safety margin.
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-hang-monitor",
		"--disable-ipc-flooding-protection",
		"--disable-breakpad",
		"--disable-default-apps", "--disable-prompt-on-repost",
		"--allow-pre-commit-input", "--force-color-profile=srgb",
		"--metrics-recording-only", "--password-store=basic", "--use-mock-keychain",
		"--disable-features=Translate,MediaRouter,AcceptCHFrame,OptimizationHints",
		fmt.Sprintf("--window-size=%d,%d", cfg.W, cfg.H),
	}
	// Permit native driver selection in modern headless Chrome. Hosts without
	// a usable GPU retain Chromium's built-in software fallback.
	args = append(args, "--test-type", "--headless=new", "--enable-gpu")
	if cfg.NoSandbox {
		args = append(args, "--no-sandbox")
	}
	if len(cfg.ExtensionPaths) != 0 {
		paths := strings.Join(cfg.ExtensionPaths, ",")
		args = append(args,
			"--load-extension="+paths,
			// Branded Chrome ignores --load-extension, but exposes the CDP
			// Extensions domain when this flag is present. Launch loads the
			// unpacked extensions through that domain after connecting.
			"--enable-unsafe-extension-debugging",
		)
	}
	args = append(args, cfg.ExtraArgs...)
	args = append(args, "about:blank")
	return args
}

// Launch starts headless Chromium (see Args) and returns a connected browser
// client.
func Launch(cfg LaunchConfig) (*Client, *process.Started, error) {
	// A browser killed without a normal shutdown can leave this file behind.
	// Snapshot it before launch so the fallback can never attach this Surf
	// instance to an older Chromium that happens to use the same profile.
	previousEndpoint := readActivePortState(cfg.Profile)
	started, err := process.Start(cfg.ChromePath, cfg.Args(), process.Options{
		Stderr: true, Guardian: true,
	})
	if err != nil {
		return nil, nil, err
	}

	wsURL := make(chan string, 1)
	stderrTail := &boundedLineTail{}
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		sc := bufio.NewScanner(started.Stderr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			stderrTail.add(line)
			if m := devtoolsRe.FindStringSubmatch(line); m != nil {
				select {
				case wsURL <- m[1]:
				default:
				}
			}
		}
	}()

	url, err := waitForURL(wsURL, cfg.Profile, previousEndpoint, started.Done)
	if err != nil {
		process.Kill(started.Process.Pid)
		select {
		case <-stderrDone:
		case <-time.After(250 * time.Millisecond):
		}
		if detail := stderrTail.string(); detail != "" {
			err = fmt.Errorf("%w; chromium stderr: %s", err, detail)
		}
		return nil, nil, err
	}
	c, err := Dial(url)
	if err != nil {
		process.Kill(started.Process.Pid)
		return nil, nil, err
	}
	if err := ensureUnpackedExtensions(c, cfg.ExtensionPaths); err != nil {
		c.Close()
		process.Kill(started.Process.Pid)
		return nil, nil, err
	}
	return c, started, nil
}

type extensionCaller interface {
	Call(sessionID, method string, params any) (json.RawMessage, error)
}

// ensureUnpackedExtensions bridges the two browser behaviors Surf supports:
// Chromium still honors --load-extension, while branded Chrome requires the
// Extensions.loadUnpacked CDP command. Already-loaded paths are left alone so
// Chromium extensions are not needlessly restarted during browser startup.
func ensureUnpackedExtensions(client extensionCaller, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	raw, err := client.Call("", "Extensions.getExtensions", nil)
	if err != nil {
		return fmt.Errorf("list unpacked extensions: %w", err)
	}
	var result struct {
		Extensions []struct {
			Path string `json:"path"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("decode unpacked extensions: %w", err)
	}
	loaded := make(map[string]struct{}, len(result.Extensions))
	for _, extension := range result.Extensions {
		if extension.Path != "" {
			loaded[filepath.Clean(extension.Path)] = struct{}{}
		}
	}
	for _, path := range paths {
		cleanPath := filepath.Clean(path)
		if _, ok := loaded[cleanPath]; ok {
			continue
		}
		if _, err := client.Call("", "Extensions.loadUnpacked", map[string]any{"path": cleanPath}); err != nil {
			return fmt.Errorf("load unpacked extension %s: %w", filepath.Base(cleanPath), err)
		}
		loaded[cleanPath] = struct{}{}
	}
	return nil
}

type activePortState struct {
	data    string
	modTime time.Time
	exists  bool
}

func readActivePortState(profile string) activePortState {
	path := filepath.Join(profile, "DevToolsActivePort")
	data, err := os.ReadFile(path)
	if err != nil {
		return activePortState{}
	}
	state := activePortState{data: string(data), exists: true}
	if info, err := os.Stat(path); err == nil {
		state.modTime = info.ModTime()
	}
	return state
}

func (s activePortState) changedFrom(previous activePortState) bool {
	if !previous.exists {
		return s.exists
	}
	return s.exists && (s.data != previous.data || s.modTime.After(previous.modTime))
}

// waitForURL prefers the stderr banner; the DevToolsActivePort file in the
// profile dir is the fallback (some builds log differently).
func waitForURL(
	fromStderr <-chan string,
	profile string,
	previous activePortState,
	processDone <-chan error,
) (string, error) {
	return waitForURLWithin(fromStderr, profile, previous, processDone, browserStartupTimeout)
}

func waitForURLWithin(
	fromStderr <-chan string,
	profile string,
	previous activePortState,
	processDone <-chan error,
	timeout time.Duration,
) (string, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	var launcherExited bool
	var launcherErr error
	for {
		select {
		case u := <-fromStderr:
			if u != "" {
				return u, nil
			}
		case err, ok := <-processDone:
			if ok {
				launcherExited = true
				launcherErr = err
			}
			// Chrome-family executables may be bootstrap processes which hand the
			// requested profile to another process and exit before that process
			// publishes DevToolsActivePort. The endpoint, not this PID, is the
			// startup authority, so retain the result only for timeout diagnostics.
			processDone = nil
		case <-deadline.C:
			if !launcherExited {
				return "", fmt.Errorf("chromium did not expose a DevTools endpoint within %s", timeout)
			}
			if launcherErr != nil {
				return "", fmt.Errorf("chromium did not expose a DevTools endpoint within %s (launcher exited: %w)", timeout, launcherErr)
			}
			return "", fmt.Errorf("chromium did not expose a DevTools endpoint within %s (launcher exited cleanly)", timeout)
		case <-tick.C:
			state := readActivePortState(profile)
			if !state.changedFrom(previous) {
				continue
			}
			lines := strings.Split(strings.TrimSpace(state.data), "\n")
			if len(lines) < 2 {
				continue
			}
			u, err := browserURLFromPort(strings.TrimSpace(lines[0]))
			if err == nil {
				return u, nil
			}
		}
	}
}

func browserURLFromPort(port string) (string, error) {
	resp, err := http.Get("http://127.0.0.1:" + port + "/json/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var v struct {
		URL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	if v.URL == "" {
		return "", fmt.Errorf("no webSocketDebuggerUrl")
	}
	return v.URL, nil
}

func Dial(url string) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(8 << 20)
	c := &Client{
		conn:     conn,
		pending:  map[int64]chan response{},
		evSignal: make(chan struct{}, 1),
		closed:   make(chan struct{}),
	}
	go c.readLoop()
	go c.dispatchLoop()
	return c, nil
}

func (c *Client) enqueueEvent(ev Event) {
	c.evMu.Lock()
	c.evQueue = append(c.evQueue, ev)
	c.evMu.Unlock()
	select {
	case c.evSignal <- struct{}{}:
	default:
	}
}

func (c *Client) dispatchLoop() {
	for {
		select {
		case <-c.evSignal:
		case <-c.closed:
			return
		}
		for {
			c.evMu.Lock()
			if len(c.evQueue) == 0 {
				c.evMu.Unlock()
				break
			}
			batch := c.evQueue
			c.evQueue = nil
			c.evMu.Unlock()
			c.handlerMu.RLock()
			h := c.handler
			c.handlerMu.RUnlock()
			for _, ev := range batch {
				if h != nil {
					h(ev)
				}
			}
		}
	}
}

// OnEvent installs the single event sink; the browser package multiplexes.
func (c *Client) OnEvent(fn func(Event)) {
	c.handlerMu.Lock()
	c.handler = fn
	c.handlerMu.Unlock()
}

func (c *Client) Closed() <-chan struct{} { return c.closed }

func (c *Client) readLoop() {
	defer close(c.closed)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.failAll(err)
			return
		}
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		if env.ID != 0 {
			c.mu.Lock()
			ch := c.pending[env.ID]
			delete(c.pending, env.ID)
			c.mu.Unlock()
			if ch != nil {
				r := response{result: env.Result}
				if env.Error != nil {
					r.err = fmt.Errorf("cdp: %s (%d)", env.Error.Message, env.Error.Code)
				}
				ch <- r
			}
			continue
		}
		if env.Method != "" {
			c.enqueueEvent(Event{Method: env.Method, SessionID: env.SessionID, Params: env.Params})
		}
	}
}

func (c *Client) failAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- response{err: err}
		delete(c.pending, id)
	}
}

// Call issues a command on a session ("" = browser session) and waits for the
// reply. Result is raw JSON; callers unmarshal what they need.
func (c *Client) Call(sessionID, method string, params any) (json.RawMessage, error) {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan response, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	msg := envelope{ID: id, Method: method, Params: raw, SessionID: sessionID}
	b, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, b)
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	select {
	case r := <-ch:
		return r.result, r.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp: %s timed out", method)
	case <-c.closed:
		return nil, io.ErrClosedPipe
	}
}

// Send is Call for callers that don't care about result or errors
// (acks, best-effort input dispatch during navigation, ...).
func (c *Client) Send(sessionID, method string, params any) {
	go func() { _, _ = c.Call(sessionID, method, params) }()
}

// Dispatch writes a command in caller order without waiting for its response.
// It is intended for high-rate input, where Chromium's receipt order matters
// but synchronously waiting for an acknowledgement only stalls the transport
// reader and lets newer touch events pile up behind an old one.
func (c *Client) Dispatch(sessionID, method string, params any) error {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = b
	}
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	b, err := json.Marshal(envelope{ID: id, Method: method, Params: raw, SessionID: sessionID})
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, b)
	c.writeMu.Unlock()
	return err
}

func (c *Client) Close() { _ = c.conn.Close() }
