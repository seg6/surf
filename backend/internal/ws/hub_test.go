package ws

import (
	"net/http/httptest"
	"testing"
)

func TestCheckOrigin(t *testing.T) {
	h := NewHub()
	req := httptest.NewRequest("GET", "http://surf.example/ws", nil)
	if !h.upgrader.CheckOrigin(req) {
		t.Fatal("missing Origin should be allowed for native clients")
	}

	req = httptest.NewRequest("GET", "http://surf.example/ws", nil)
	req.Header.Set("Origin", "http://surf.example")
	if !h.upgrader.CheckOrigin(req) {
		t.Fatal("same-origin websocket should be allowed")
	}

	req = httptest.NewRequest("GET", "http://surf.example/ws", nil)
	req.Header.Set("Origin", "http://evil.example")
	if h.upgrader.CheckOrigin(req) {
		t.Fatal("cross-origin websocket should be rejected")
	}
}
