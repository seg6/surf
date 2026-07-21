// Package cdp is a thin Chrome DevTools Protocol client: launch Chromium,
// dial its browser WebSocket, correlate request/response by id, and fan out
// events. Flat sessions only (Target.attachToTarget with flatten:true); the
// small CDP surface we use doesn't justify a generated protocol binding.
package cdp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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

// Launch starts Chromium headful (Xvfb provides the display) with the same
// flag set puppeteer was given, and returns a connected browser client.
func Launch(chromePath, profile string, w, h int) (*Client, *exec.Cmd, error) {
	args := []string{
		"--remote-debugging-port=0",
		"--user-data-dir=" + profile,
		"--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu",
		"--disable-blink-features=AutomationControlled",
		"--disable-popup-blocking", "--no-first-run", "--no-default-browser-check",
		"--disable-session-crashed-bubble", "--hide-crash-restore-bubble", "--noerrdialogs",
		"--hide-scrollbars",
		"--disable-background-networking", "--disable-sync",
		// The anti-throttling set puppeteer always passed: without these,
		// Chromium treats the Xvfb window as backgrounded/occluded and stops
		// producing compositor frames (dead screencast, multi-second
		// screenshots).
		// Wheel scrolls must apply instantly: the client predicts scroll
		// locally and reconciles against frame scroll offsets — an animated
		// scroll makes the server look permanently behind.
		"--disable-smooth-scrolling",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-hang-monitor",
		"--disable-ipc-flooding-protection",
		"--disable-breakpad", "--disable-component-update",
		"--disable-default-apps", "--disable-prompt-on-repost",
		"--test-type",
		"--allow-pre-commit-input", "--force-color-profile=srgb",
		"--metrics-recording-only", "--password-store=basic", "--use-mock-keychain",
		"--disable-features=Translate,MediaRouter,AcceptCHFrame,OptimizationHints",
		fmt.Sprintf("--window-size=%d,%d", w, h),
		// Fullscreen/kiosk: no toolbar/omnibox/tab strip on the X display. The
		// CDP screencast sees page pixels either way, but the video lane films
		// Xvfb directly, so a normal browser window would stream Chromium chrome.
		// Position must be pinned: with no WM the default window origin is (10,10),
		// which shifts and clips the filmed page.
		"--kiosk",
		"--start-fullscreen",
		"--window-position=0,0",
		"about:blank",
	}
	cmd := exec.Command(chromePath, args...)
	cmd.Env = os.Environ()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	wsURL := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			if m := devtoolsRe.FindStringSubmatch(sc.Text()); m != nil {
				select {
				case wsURL <- m[1]:
				default:
				}
			}
		}
	}()

	url, err := waitForURL(wsURL, profile)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, nil, err
	}
	c, err := Dial(url)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, nil, err
	}
	return c, cmd, nil
}

func (c *Client) ForceFullscreen() {
	var target struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	raw, err := c.Call("", "Target.getTargets", nil)
	if err != nil || json.Unmarshal(raw, &target) != nil {
		return
	}
	for _, info := range target.TargetInfos {
		if info.Type != "page" {
			continue
		}
		var win struct {
			WindowID int `json:"windowId"`
		}
		raw, err := c.Call("", "Browser.getWindowForTarget", map[string]any{"targetId": info.TargetID})
		if err != nil || json.Unmarshal(raw, &win) != nil || win.WindowID == 0 {
			continue
		}
		c.Send("", "Browser.setWindowBounds", map[string]any{"windowId": win.WindowID, "bounds": map[string]any{"windowState": "fullscreen"}})
		return
	}
}

// waitForURL prefers the stderr banner; the DevToolsActivePort file in the
// profile dir is the fallback (some builds log differently).
func waitForURL(fromStderr <-chan string, profile string) (string, error) {
	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case u := <-fromStderr:
			return u, nil
		case <-deadline:
			return "", fmt.Errorf("chromium did not expose a DevTools endpoint within 30s")
		case <-tick.C:
			b, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
			if err != nil {
				continue
			}
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
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
	conn.SetReadLimit(64 << 20) // screencast frames are big
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

func (c *Client) Close() { _ = c.conn.Close() }
