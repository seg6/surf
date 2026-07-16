// Package ws owns the client-facing WebSocket side: connection lifecycle,
// keepalive, and pipelined-but-coalesced frame delivery: up to maxInflight
// frames ride the pipe unacked (so throughput isn't bound by RTT), the
// client acks every frame it receives ({t:'ready'}, even ones it skips
// rendering), and only the LATEST undelivered frame is kept. The final
// frame of any change is always delivered and the pipe can never wedge.
package ws

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"rbrowser/internal/protocol"
)

var (
	errClosed       = errors.New("ws: client closed")
	errBackpressure = errors.New("ws: outbox full")
)

const (
	pingInterval = 30 * time.Second
	pongWait     = 75 * time.Second
	writeWait    = 20 * time.Second

	// maxInflight is the frame pipelining window. 3 covers ~200ms of RTT at
	// 15fps without letting a stalled client accumulate a backlog.
	maxInflight = 3
)

// Handler is implemented by the browser side.
type Handler interface {
	ClientConnected(c *Client)
	ClientDisconnected(c *Client)
	HandleMessage(c *Client, m *protocol.ClientMessage)
}

type Hub struct {
	mu       sync.Mutex
	clients  map[*Client]struct{}
	frameSeq uint32
	handler  Handler
	upgrader websocket.Upgrader
}

func NewHub() *Hub {
	return &Hub{
		clients: map[*Client]struct{}{},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 32768,
			// Same-origin is enforced by the token in the query string; the
			// origin header on iOS 6 is unreliable.
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

func (h *Hub) SetHandler(hd Handler) { h.handler = hd }

func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *Hub) HasNativeClient() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.Native {
			return true
		}
	}
	return false
}

// HasCastClient reports whether anyone still consumes the JPEG screencast —
// when every connected client is in video mode (or none are connected), the
// cast can stop and give its CPU to x264.
func (h *Hub) HasCastClient() bool {
	h.mu.Lock()
	targets := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		targets = append(targets, c)
	}
	h.mu.Unlock()
	for _, c := range targets {
		if !c.VideoMode() {
			return true
		}
	}
	return false
}

type outMsg struct {
	msgType int
	data    []byte
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn

	// All writes go through a buffered outbox drained by one writer
	// goroutine, so no caller (especially the CDP event dispatcher) can ever
	// block on a slow client socket.
	out  chan outMsg
	done chan struct{}

	mu        sync.Mutex
	inflight  int
	pending   *protocol.Frame
	videoMode bool
	Native    bool
}

// SetVideoMode flips the client between the JPEG cast lane and the H.264
// lane. In video mode the client receives no cast frames — only type-3 AUs
// plus the sharp settle JPEG (Frame.Sharp).
func (c *Client) SetVideoMode(on bool) {
	c.mu.Lock()
	c.videoMode = on
	// The ack window belongs to the JPEG lane; entering/leaving video mode
	// resets it so a stale window can't wedge the sharp-frame path.
	c.inflight = 0
	c.pending = nil
	c.mu.Unlock()
}

func (c *Client) VideoMode() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.videoMode
}

// SendBinary enqueues a pre-encoded binary message (type-3/4 frames) on the
// shared outbox, bypassing the type-1 ack window. Never blocks; an error
// means the outbox was full and the message was dropped.
func (c *Client) SendBinary(data []byte) error {
	return c.write(websocket.BinaryMessage, data)
}

// Serve upgrades the request and runs the read loop until disconnect.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, native bool) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &Client{hub: h, conn: conn, out: make(chan outMsg, 32), done: make(chan struct{}), Native: native}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	go c.writeLoop()

	if h.handler != nil {
		h.handler.ClientConnected(c)
	}

	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		var m protocol.ClientMessage
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if h.handler != nil {
			h.handler.HandleMessage(c, &m)
		}
	}

	close(c.done)
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	_ = conn.Close()
	if h.handler != nil {
		h.handler.ClientDisconnected(c)
	}
}

// writeLoop is the only goroutine touching the write side of the socket.
func (c *Client) writeLoop() {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		var m outMsg
		select {
		case <-c.done:
			return
		case <-ping.C:
			m = outMsg{msgType: websocket.PingMessage}
		case m = <-c.out:
		}
		_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteMessage(m.msgType, m.data); err != nil {
			_ = c.conn.Close() // unblocks the read loop; Serve cleans up
			return
		}
	}
}

// write enqueues without ever blocking; a client too slow to drain its
// outbox loses messages (frames are coalesced anyway) and is eventually
// reaped by the ping deadline.
func (c *Client) write(msgType int, data []byte) error {
	select {
	case c.out <- outMsg{msgType: msgType, data: data}:
		return nil
	case <-c.done:
		return errClosed
	default:
		return errBackpressure
	}
}

func (c *Client) SendJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = c.write(websocket.TextMessage, b)
}

// Ready is the client's per-frame ack (sent even for frames it skipped
// rendering): shrink the window, flush the pending frame if any.
func (c *Client) Ready() {
	c.mu.Lock()
	if c.inflight > 0 {
		c.inflight--
	}
	f := c.takePendingLocked()
	c.mu.Unlock()
	c.sendFrame(f)
}

// PokeReset handles the client watchdog: assume acks were lost, reset the
// window, and let the caller push a fresh frame.
func (c *Client) PokeReset() {
	c.mu.Lock()
	c.inflight = 0
	c.pending = nil
	c.mu.Unlock()
}

func (c *Client) takePendingLocked() *protocol.Frame {
	if c.pending == nil || c.inflight >= maxInflight {
		return nil
	}
	c.inflight++
	f := c.pending
	c.pending = nil
	return f
}

func (c *Client) sendFrame(f *protocol.Frame) {
	if f == nil {
		return
	}
	if c.write(websocket.BinaryMessage, f.Encode()) != nil {
		// Frame dropped (backpressure/closing): give the window slot back so
		// the next frame isn't gated on an ack that will never come.
		c.mu.Lock()
		if c.inflight > 0 {
			c.inflight--
		}
		c.mu.Unlock()
	}
}

func (c *Client) queue(f *protocol.Frame) {
	c.mu.Lock()
	c.pending = f
	send := c.takePendingLocked()
	c.mu.Unlock()
	c.sendFrame(send)
}

// QueueFrame stamps a sequence number and delivers to one client (only != nil)
// or all of them, replacing any not-yet-sent frame.
func (h *Hub) QueueFrame(f *protocol.Frame, only *Client) {
	h.mu.Lock()
	h.frameSeq++
	if h.frameSeq == 0 {
		h.frameSeq = 1
	}
	f.Seq = h.frameSeq
	targets := make([]*Client, 0, len(h.clients))
	if only != nil {
		targets = append(targets, only)
	} else {
		for c := range h.clients {
			targets = append(targets, c)
		}
	}
	h.mu.Unlock()
	for _, c := range targets {
		// Video-mode clients get no cast JPEGs; the sharp settle frame is the
		// one exception (docs/03 §3). Directed sends (only != nil) always go
		// through — they answer this client's own poke/size request.
		if only == nil && !f.Sharp && c.VideoMode() {
			continue
		}
		c.queue(f)
	}
}

func (h *Hub) BroadcastJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	targets := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		targets = append(targets, c)
	}
	h.mu.Unlock()
	for _, c := range targets {
		_ = c.write(websocket.TextMessage, b)
	}
}
