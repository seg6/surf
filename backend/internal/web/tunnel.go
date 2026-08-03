package web

import (
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

const (
	tunnelFrameLimit = 64 << 10
	tunnelIdle       = 2 * time.Minute
)

var tunnelUpgrader = websocket.Upgrader{
	ReadBufferSize:  16 << 10,
	WriteBufferSize: 16 << 10,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// handleTunnel carries an opaque TCP byte stream inside a WebSocket. The
// client runs Surf's normal pinned TLS connection inside this stream, so an
// HTTP reverse proxy may terminate the outer TLS layer without seeing or
// changing the Surf session. The only reachable target is this Surf process's
// own loopback listener.
func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case s.tunnelSlots <- struct{}{}:
		defer func() { <-s.tunnelSlots }()
	default:
		http.Error(w, "tunnel capacity reached", http.StatusServiceUnavailable)
		return
	}

	target, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.cfg.Port)), 5*time.Second)
	if err != nil {
		http.Error(w, "Surf listener unavailable", http.StatusServiceUnavailable)
		return
	}
	defer target.Close()

	ws, err := tunnelUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	ws.SetReadLimit(tunnelFrameLimit)

	done := make(chan error, 1)
	go func() { done <- tunnelTCPToWebSocket(ws, target) }()
	err = tunnelWebSocketToTCP(ws, target)
	_ = target.SetDeadline(time.Now())
	select {
	case <-done:
	case <-time.After(time.Second):
	}
	if err != nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && err != io.EOF {
		log.Printf("tunnel: closed: %v", err)
	}
}

func tunnelWebSocketToTCP(ws *websocket.Conn, target net.Conn) error {
	for {
		messageType, payload, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage {
			return websocket.ErrBadHandshake
		}
		_ = target.SetWriteDeadline(time.Now().Add(tunnelIdle))
		if _, err := target.Write(payload); err != nil {
			return err
		}
	}
}

func tunnelTCPToWebSocket(ws *websocket.Conn, target net.Conn) error {
	buffer := make([]byte, 16<<10)
	for {
		_ = target.SetReadDeadline(time.Now().Add(tunnelIdle))
		n, err := target.Read(buffer)
		if n > 0 {
			_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if writeErr := ws.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			return err
		}
	}
}
