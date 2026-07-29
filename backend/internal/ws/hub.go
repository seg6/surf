// Package ws owns client-facing WebSocket connection lifecycle and media
// scheduling. JPEG capture frames never cross this boundary.
package ws

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
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
	ClientConnected(c *ClientTransport)
	ClientDisconnected(c *ClientTransport)
	HandleMessage(c *ClientTransport, command protocol.Command)
}

type Hub struct {
	mu       sync.Mutex
	clients  map[*ClientTransport]struct{}
	frameSeq uint32
	handler  Handler
	upgrader websocket.Upgrader

	controlFailures atomic.Uint64
	videoDrops      atomic.Uint64
	audioDrops      atomic.Uint64
	socketWrites    atomic.Uint64
}

func NewHub() *Hub {
	return &Hub{
		clients: map[*ClientTransport]struct{}{},
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

func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
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

type ClientTransport struct {
	hub  *Hub
	conn *websocket.Conn

	control chan outMsg
	video   chan outMsg
	audio   chan outMsg
	done    chan struct{}

	mu            sync.Mutex
	videoDropping bool
}

// SendBinary enqueues a pre-encoded binary media message. It never blocks; an
// error means the lane-specific queue applied its drop policy.
func (c *ClientTransport) SendBinary(data []byte) error {
	return c.write(websocket.BinaryMessage, data)
}

func (c *ClientTransport) Closed() <-chan struct{} { return c.done }
func (c *ClientTransport) Close()                  { _ = c.conn.Close() }

// Serve upgrades the request and runs the read loop until disconnect.
//
// No explicit TCP_NODELAY here: the underlying net.Conn is a bare
// *net.TCPConn (this server speaks plain HTTP; TLS, if any, is terminated
// by a reverse proxy), and Go's net package defaults
// TCP_NODELAY to true for every TCPConn (net/tcpsock.go: newTCPConn calls
// setNoDelay(fd, true)). Nagle is already off on this side; the client's
// hand-rolled socket (native/client/Classes/RBSocket.m) is the one that
// needed an explicit setsockopt.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(readLimit)
	c := &ClientTransport{
		hub: h, conn: conn, done: make(chan struct{}),
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

// writeLoop is the only goroutine touching the write side of the socket.
func (c *ClientTransport) writeLoop() {
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

func (c *ClientTransport) writeOne(m outMsg) bool {
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

func (c *ClientTransport) sendClock(clientSendNS, backendRecvNS uint64) {
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
func (c *ClientTransport) write(msgType int, data []byte) error {
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

func (c *ClientTransport) SendJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = c.write(websocket.TextMessage, b)
}

func (h *Hub) BroadcastJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	targets := make([]*ClientTransport, 0, len(h.clients))
	for c := range h.clients {
		targets = append(targets, c)
	}
	h.mu.Unlock()
	for _, c := range targets {
		_ = c.write(websocket.TextMessage, b)
	}
}
