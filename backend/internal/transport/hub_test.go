package transport

import (
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"

	"surf-backend/internal/protocol"
)

func TestCheckOrigin(t *testing.T) {
	h := New()
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

func testClient() *Client {
	return &Client{control: make(chan outMsg, 2), video: make(chan outMsg, 2), audio: make(chan outMsg, 2), done: make(chan struct{})}
}

func TestVideoOverflowDropsUntilIDR(t *testing.T) {
	c := testClient()
	p := func(seq uint32, idr bool) []byte {
		return protocol.EncodeVideoAU(seq, idr, 10, 10, []byte{1})
	}
	if c.SendBinary(p(1, true)) != nil || c.SendBinary(p(2, false)) != nil {
		t.Fatal("initial GOP")
	}
	if c.SendBinary(p(3, false)) != errBackpressure {
		t.Fatal("overflow must drop")
	}
	if len(c.video) != 0 {
		t.Fatal("dependent queued frames were not discarded")
	}
	if c.SendBinary(p(4, false)) != errBackpressure {
		t.Fatal("must wait for IDR")
	}
	if c.SendBinary(p(5, true)) != nil || len(c.video) != 1 {
		t.Fatal("IDR did not resync")
	}
}

func TestAudioOverflowDropsOldest(t *testing.T) {
	c := testClient()
	for seq := uint32(1); seq <= 3; seq++ {
		if c.write(websocket.BinaryMessage, protocol.EncodeAudioPCM(seq, 16000, 1, []byte{byte(seq)})) != nil {
			t.Fatal(seq)
		}
	}
	if len(c.audio) != 2 {
		t.Fatalf("audio depth %d", len(c.audio))
	}
	first := <-c.audio
	if first.data[11] != 2 {
		t.Fatalf("oldest retained: seq bytes %v", first.data[8:12])
	}
}

func TestConnectedDeviceTargetingDeduplicatesSockets(t *testing.T) {
	hub := New()
	first, second, other := testClient(), testClient(), testClient()
	first.hub, first.deviceID = hub, "device-a"
	second.hub, second.deviceID = hub, "device-a"
	other.hub, other.deviceID = hub, "device-b"
	hub.clients[first] = struct{}{}
	hub.clients[second] = struct{}{}
	hub.clients[other] = struct{}{}
	if got := hub.ConnectedDevices(); len(got) != 2 || got[0] != "device-a" || got[1] != "device-b" {
		t.Fatalf("connected=%v", got)
	}
	targets := hub.SendDeviceJSON("device-a", protocol.EmptyEvent{Type: "log-request"})
	if len(targets) != 1 || targets[0] != "device-a" || len(first.control) != 1 || len(second.control) != 1 || len(other.control) != 0 {
		t.Fatalf("targets=%v queues=%d/%d/%d", targets, len(first.control), len(second.control), len(other.control))
	}
}
