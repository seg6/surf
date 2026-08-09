// Package transport owns client-facing WebSocket lifecycle and media
// scheduling. Raw capture frames never cross this boundary.
package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"surf-backend/internal/protocol"
	"surf-backend/internal/telemetry"
)

var (
	errClosed       = errors.New("ws: client closed")
	errBackpressure = errors.New("ws: outbox full")
)

const (
	pingInterval  = 30 * time.Second
	pongWait      = 75 * time.Second
	writeWait     = 20 * time.Second
	readLimit     = 64 << 10
	audioQueueCap = 3 // 60ms; native owns the adaptive playout cushion
)

// Handler is implemented by the browser side.
type Handler interface {
	ClientConnected(c *Client)
	ClientDisconnected(c *Client)
	HandleMessage(c *Client, command protocol.Command)
}

type Hub struct {
	mu       sync.Mutex
	clients  map[*Client]struct{}
	frameSeq uint32
	handler  Handler
	aux      func(*Client, protocol.Command) bool
	connect  func(*Client)
	upgrader websocket.Upgrader

	controlFailures atomic.Uint64
	videoDrops      atomic.Uint64
	audioDrops      atomic.Uint64
	socketWrites    atomic.Uint64
}

func New() *Hub {
	return &Hub{
		clients: map[*Client]struct{}{},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 32768,
			CheckOrigin:     checkOrigin,
		},
	}
}

func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // native iOS socket sends no Origin header
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func (h *Hub) SetHandler(hd Handler) { h.handler = hd }

func (h *Hub) SetAuxHandler(handler func(*Client, protocol.Command) bool) {
	h.mu.Lock()
	h.aux = handler
	h.mu.Unlock()
}

func (h *Hub) SetConnectHandler(handler func(*Client)) {
	h.mu.Lock()
	h.connect = handler
	h.mu.Unlock()
}

func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *Hub) ConnectedDevices() []string {
	h.mu.Lock()
	seen := make(map[string]struct{}, len(h.clients))
	for client := range h.clients {
		if client.deviceID != "" {
			seen[client.deviceID] = struct{}{}
		}
	}
	h.mu.Unlock()
	result := make([]string, 0, len(seen))
	for deviceID := range seen {
		result = append(result, deviceID)
	}
	sort.Strings(result)
	return result
}

// SendDeviceJSON sends one control event to every live socket for deviceID.
// An empty deviceID broadcasts to all paired devices. The result contains the
// unique device IDs that had at least one live socket.
func (h *Hub) SendDeviceJSON(deviceID string, event protocol.Event) []string {
	data, err := json.Marshal(event)
	if err != nil {
		return nil
	}
	h.mu.Lock()
	targets := make([]*Client, 0, len(h.clients))
	seen := map[string]struct{}{}
	for client := range h.clients {
		if client.deviceID == "" || deviceID != "" && client.deviceID != deviceID {
			continue
		}
		targets = append(targets, client)
		seen[client.deviceID] = struct{}{}
	}
	h.mu.Unlock()
	for _, client := range targets {
		_ = client.write(websocket.TextMessage, data)
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func (h *Hub) Stats() map[string]uint64 {
	h.mu.Lock()
	var controlDepth, videoDepth, audioDepth uint64
	for client := range h.clients {
		controlDepth += uint64(len(client.control))
		videoDepth += uint64(len(client.video))
		audioDepth += uint64(len(client.audio))
	}
	h.mu.Unlock()
	return map[string]uint64{
		"transport_control_failures": h.controlFailures.Load(),
		"transport_video_drops":      h.videoDrops.Load(),
		"transport_audio_drops":      h.audioDrops.Load(),
		"transport_socket_writes":    h.socketWrites.Load(),
		"control_queue_depth":        controlDepth,
		"video_queue_depth":          videoDepth,
		"audio_queue_depth":          audioDepth,
	}
}

type outMsg struct {
	msgType int
	data    []byte
	clock   *clockReply
}

type clockReply struct {
	clientSendNS  uint64
	backendRecvNS uint64
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	deviceID string

	control chan outMsg
	video   chan outMsg
	audio   chan outMsg
	done    chan struct{}

	mu            sync.Mutex
	videoDropping bool
}

// SendBinary enqueues a pre-encoded binary media message. It never blocks; an
// error means the lane-specific queue applied its drop policy.
func (c *Client) SendBinary(data []byte) error {
	return c.write(websocket.BinaryMessage, data)
}

func (c *Client) Closed() <-chan struct{} { return c.done }
func (c *Client) Close()                  { _ = c.conn.Close() }
func (c *Client) DeviceID() string        { return c.deviceID }

// Serve upgrades the request and runs the read loop until disconnect.
//
// No explicit TCP_NODELAY here: the underlying net.Conn is a bare
// *net.TCPConn (this server speaks plain HTTP; TLS, if any, is terminated
// by a reverse proxy), and Go's net package defaults
// TCP_NODELAY to true for every TCPConn (net/tcpsock.go: newTCPConn calls
// setNoDelay(fd, true)). Nagle is already off on this side; the client's
// hand-rolled socket (native/client/Classes/RBSocket.m) is the one that
// needed an explicit setsockopt.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.ServeHTTPForDevice(w, r, "")
}

// ServeHTTPForDevice attaches the authenticated device identity to the live
// transport so revocation can terminate it immediately.
func (h *Hub) ServeHTTPForDevice(w http.ResponseWriter, r *http.Request, deviceID string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(readLimit)
	c := &Client{
		hub: h, conn: conn, deviceID: deviceID, done: make(chan struct{}),
		control: make(chan outMsg, 64),
		video:   make(chan outMsg, 4),
		audio:   make(chan outMsg, audioQueueCap),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	go c.writeLoop()

	if h.handler != nil {
		h.handler.ClientConnected(c)
	}
	h.mu.Lock()
	connect := h.connect
	h.mu.Unlock()
	if connect != nil {
		connect(c)
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
		command, err := protocol.DecodeCommand(data)
		if err != nil {
			continue
		}
		if clock, ok := command.(*protocol.ClockCommand); ok {
			received := telemetry.MonoNS()
			c.sendClock(clock.ClientSendNS, received)
			continue
		}
		h.mu.Lock()
		aux := h.aux
		h.mu.Unlock()
		if aux != nil && aux(c, command) {
			continue
		}
		if h.handler != nil {
			h.handler.HandleMessage(c, command)
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

// CloseDevice terminates all active sockets authenticated as deviceID.
func (h *Hub) CloseDevice(deviceID string) {
	h.mu.Lock()
	targets := make([]*Client, 0, 1)
	for client := range h.clients {
		if client.deviceID == deviceID {
			targets = append(targets, client)
		}
	}
	h.mu.Unlock()
	for _, client := range targets {
		client.Close()
	}
}

// writeLoop is the only goroutine touching the write side of the socket.
func (c *Client) writeLoop() {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		var m outMsg
		// Ordered control messages have priority over lossy media.
		select {
		case <-c.done:
			return
		case m = <-c.control:
			if !c.writeOne(m) {
				return
			}
			continue
		default:
		}
		select {
		case <-c.done:
			return
		case <-ping.C:
			m = outMsg{msgType: websocket.PingMessage}
		case m = <-c.control:
		case m = <-c.video:
		default:
			select {
			case <-c.done:
				return
			case <-ping.C:
				m = outMsg{msgType: websocket.PingMessage}
			case m = <-c.control:
			case m = <-c.video:
			case m = <-c.audio:
			}
		}
		if !c.writeOne(m) {
			return
		}
	}
}

func (c *Client) writeOne(m outMsg) bool {
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if m.clock != nil {
		// s2 is the time this reply actually reaches the head of the socket
		// writer, not the earlier enqueue time. Queue dwell must be part of
		// the NTP-style RTT sample rather than misreported as clock offset.
		m.data, _ = json.Marshal(protocol.ClockEvent{
			Type: "clock", ClientSendNS: m.clock.clientSendNS,
			BackendRecvNS: m.clock.backendRecvNS, BackendSendNS: telemetry.MonoNS(),
		})
	}
	if m.msgType == websocket.BinaryMessage {
		protocol.StampSocketWrite(m.data, telemetry.MonoNS())
	}
	if err := c.conn.WriteMessage(m.msgType, m.data); err != nil {
		_ = c.conn.Close() // unblocks the read loop; Serve cleans up
		return false
	}
	if c.hub != nil {
		c.hub.socketWrites.Add(1)
	}
	telemetry.Emit("socket_write", "transport", "websocket", map[string]any{"bytes": len(m.data), "binary": m.msgType == websocket.BinaryMessage})
	return true
}

func (c *Client) sendClock(clientSendNS, backendRecvNS uint64) {
	m := outMsg{msgType: websocket.TextMessage, clock: &clockReply{
		clientSendNS: clientSendNS, backendRecvNS: backendRecvNS,
	}}
	select {
	case c.control <- m:
	case <-c.done:
	default:
		if c.hub != nil {
			c.hub.controlFailures.Add(1)
		}
		_ = c.conn.Close()
	}
}

// write enqueues without ever blocking; a client too slow to drain its
// outbox loses messages (frames are coalesced anyway) and is eventually
// reaped by the ping deadline.
func (c *Client) write(msgType int, data []byte) error {
	m := outMsg{msgType: msgType, data: data}
	if msgType == websocket.TextMessage || msgType == websocket.PingMessage {
		select {
		case c.control <- m:
			return nil
		case <-c.done:
			return errClosed
		default:
			if c.hub != nil {
				c.hub.controlFailures.Add(1)
			}
			_ = c.conn.Close() // control loss makes the connection unhealthy
			return errBackpressure
		}
	}
	if len(data) > 4 && data[4] == protocol.FrameTypeAudio {
		select {
		case c.audio <- m:
			return nil
		default:
			// Audio is independently decodable: discard the oldest chunk.
			if c.hub != nil {
				c.hub.audioDrops.Add(1)
			}
			select {
			case <-c.audio:
			default:
			}
			select {
			case c.audio <- m:
				return nil
			default:
				return errBackpressure
			}
		}
	}
	if len(data) > 5 && data[4] == protocol.FrameTypeVideo {
		idr := data[5]&1 != 0
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.videoDropping && !idr {
			return errBackpressure
		}
		if idr {
			c.videoDropping = false
		}
		select {
		case c.video <- m:
			return nil
		default:
			if c.hub != nil {
				c.hub.videoDrops.Add(1)
			}
			for {
				select {
				case <-c.video:
				default:
					goto drained
				}
			}
		drained:
			c.videoDropping = !idr
			if idr {
				c.video <- m
				return nil
			}
			return errBackpressure
		}
	}
	select {
	case c.control <- m:
		return nil
	case <-c.done:
		return errClosed
	default:
		if c.hub != nil {
			c.hub.controlFailures.Add(1)
		}
		return errBackpressure
	}
}

func (c *Client) SendJSON(v protocol.Event) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = c.write(websocket.TextMessage, b)
}

func (h *Hub) BroadcastJSON(v protocol.Event) {
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
