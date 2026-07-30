package tabcapture

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewExtractsLoopbackOnlyExtension(t *testing.T) {
	source, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer source.Close()

	manifest, err := os.ReadFile(filepath.Join(source.ExtensionPath(), "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifest), `"tabCapture"`) ||
		!strings.Contains(string(manifest), `"offscreen"`) {
		t.Fatalf("manifest does not declare audio permissions: %s", manifest)
	}
	config, err := os.ReadFile(filepath.Join(source.ExtensionPath(), "config.js"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(config), `ws://127.0.0.1:`) ||
		!strings.Contains(string(config), `/audio/`) {
		t.Fatalf("bridge is not loopback-scoped: %s", config)
	}
}

func TestOpenBeforeAttachFails(t *testing.T) {
	source, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer source.Close()
	if _, err := source.Open(); err == nil {
		t.Fatal("Open succeeded before Attach")
	}
}

func TestNewEnablesLoopbackVideoBridge(t *testing.T) {
	source, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer source.Close()

	config, err := os.ReadFile(filepath.Join(source.ExtensionPath(), "config.js"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(config), `/video/`) {
		t.Fatalf("video bridge is not enabled: %s", config)
	}
}

func TestVideoFrameBridgeHeader(t *testing.T) {
	var got VideoFrame
	source := &Source{
		videoActive:  true,
		videoHandler: func(frame VideoFrame) { got = frame },
	}
	payload := []byte{0, 0, 0, 1, 0x65, 1, 2, 3}
	message := make([]byte, videoHeaderBytes+len(payload))
	copy(message[:4], "SVI1")
	message[4] = 1
	binary.BigEndian.PutUint16(message[6:8], videoHeaderBytes)
	binary.BigEndian.PutUint16(message[8:10], 768)
	binary.BigEndian.PutUint16(message[10:12], 950)
	binary.BigEndian.PutUint64(message[12:20], 1234)
	binary.BigEndian.PutUint32(message[20:24], uint32(len(payload)))
	copy(message[videoHeaderBytes:], payload)

	source.handleVideoFrame(message)
	if !got.Key || got.Width != 768 || got.Height != 950 ||
		got.Timestamp != 1234 || string(got.Data) != string(payload) {
		t.Fatalf("decoded frame = %+v", got)
	}
}
