package media

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewExtractsLoopbackOnlyExtension(t *testing.T) {
	source, err := NewCapture(t.TempDir())
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
	source, err := NewCapture(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer source.Close()
	if _, err := source.OpenAudio(); err == nil {
		t.Fatal("Open succeeded before Attach")
	}
}

func TestNewEnablesLoopbackVideoBridge(t *testing.T) {
	source, err := NewCapture(t.TempDir())
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

func TestVideoCaptureIsSourcePaced(t *testing.T) {
	source, err := NewCapture(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer source.Close()

	script, err := os.ReadFile(filepath.Join(source.ExtensionPath(), "offscreen.js"))
	if err != nil {
		t.Fatalf("read offscreen script: %v", err)
	}
	text := string(script)
	if strings.Contains(text, "videoConfig.fps") ||
		strings.Contains(text, "config.framerate") {
		t.Fatal("offscreen capture still contains an application FPS ceiling")
	}
	for _, want := range []string{
		"track.getCapabilities()",
		"maxFrameRate: clientDisplayFrameRate",
		"ideal: clientDisplayFrameRate",
		"max: clientDisplayFrameRate",
		`type: "video-warning"`,
		"width: constraints.width",
		"height: constraints.height",
		"Math.max(videoConfig.width, videoConfig.height)",
		"Math.min(videoConfig.width, videoConfig.height)",
		"const clientDisplayFrameRate = 60",
		"const maxCaptureDimension = 1024",
		"maxWidth: maxCaptureDimension",
		"maxHeight: maxCaptureDimension",
		"frameTimestamp - videoLastKeyTimestamp >= 2000000",
		"maxBufferSize: 1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("offscreen source pacing is missing %q", want)
		}
	}
}

func TestVideoFrameBridgeHeader(t *testing.T) {
	var got VideoFrame
	source := &Capture{
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
	binary.BigEndian.PutUint32(message[12:16], uint32(len(payload)))
	copy(message[videoHeaderBytes:], payload)

	source.handleVideoFrame(message)
	if !got.Key || got.Width != 768 || got.Height != 950 ||
		string(got.Data) != string(payload) {
		t.Fatalf("decoded frame = %+v", got)
	}
}
