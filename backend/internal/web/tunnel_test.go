package web

import (
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"surf-backend/internal/config"

	"github.com/gorilla/websocket"
)

func TestTunnelCarriesOpaqueBytes(t *testing.T) {
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		conn, acceptErr := target.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	port := target.Addr().(*net.TCPAddr).Port
	s := &Server{cfg: &config.Config{Port: port}, tunnelSlots: make(chan struct{}, 1)}
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + APIRoot + "/tunnel"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := []byte{0x16, 0x03, 0x03, 0, 5, 0, 1, 2, 3, 4}
	if err := conn.WriteMessage(websocket.BinaryMessage, want); err != nil {
		t.Fatal(err)
	}
	typ, got, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.BinaryMessage || string(got) != string(want) {
		t.Fatalf("echo = type %d %x, want binary %x", typ, got, want)
	}
}

func TestTunnelRejectsTextFrames(t *testing.T) {
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		conn, _ := target.Accept()
		if conn != nil {
			defer conn.Close()
			_, _ = io.Copy(io.Discard, conn)
		}
	}()

	s := &Server{cfg: &config.Config{Port: target.Addr().(*net.TCPAddr).Port}, tunnelSlots: make(chan struct{}, 1)}
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + APIRoot + "/tunnel"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("not opaque")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("text frame did not close tunnel")
	}
}
