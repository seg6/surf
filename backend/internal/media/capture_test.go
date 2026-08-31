package media

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/process"
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

func TestClosingAudioReaderParksCaptureForHostIsolation(t *testing.T) {
	source := &Capture{mediaActive: true}
	reader, writer := io.Pipe()
	active := &capture{source: source, reader: reader, writer: writer}
	source.capture = active

	if err := active.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if source.capture != nil {
		t.Fatal("closed PCM reader is still registered")
	}
	if !source.mediaActive {
		t.Fatal("closing a client reader stopped the host-isolating tab capture")
	}
}

func TestStoppingVideoParksSharedCapture(t *testing.T) {
	source := &Capture{mediaActive: true, videoActive: true, videoRunning: true}
	source.StopVideo()
	if source.videoActive || source.videoRunning {
		t.Fatal("video encoder was not stopped")
	}
	if !source.mediaActive {
		t.Fatal("stopping video stopped the shared host-isolating tab capture")
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

func TestBrowserTabCaptureIntegration(t *testing.T) {
	path := os.Getenv("SURF_TEST_BROWSER")
	if path == "" {
		t.Skip("set SURF_TEST_BROWSER to run the browser integration test")
	}
	source, err := NewCapture(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer source.Close()
	client, browserProcess, err := cdp.Launch(cdp.LaunchConfig{
		ChromePath: path,
		Profile:    t.TempDir(),
		W:          800, H: 600,
		ExtensionPaths: []string{source.ExtensionPath()},
		ExtraArgs:      []string{"--enable-unsafe-extension-debugging"},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer process.Kill(browserProcess.Pid)
	defer client.Close()
	if err := source.Attach(client); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := source.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
}

func TestVideoCaptureUsesSourceClockWithoutQueuesOrTimers(t *testing.T) {
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
	for _, want := range []string{
		"function configuredFrameRate()",
		"videoConfig.frameRate",
		"track.getCapabilities()",
		"maxFrameRate: configuredFrameRate()",
		"ideal: frameRate",
		"max: frameRate",
		"framerate: frameRate",
		"requestedFPS: frameRate",
		`type: "video-warning"`,
		"width: constraints.width",
		"height: constraints.height",
		"width: videoConfig.height",
		"height: videoConfig.width",
		"maxWidth: Math.max(videoConfig.width, videoConfig.height)",
		"maxHeight: Math.max(videoConfig.width, videoConfig.height)",
		`["variable", "quantizer"]`,
		"bitrateMode: rateControl",
		`["prefer-software"]`,
		"encoderPreference: hardwareAcceleration",
		"encodeOptions.avc = {quantizer}",
		"const keyFrame = videoNeedsKeyframe",
		"maxBufferSize: 1",
		"videoLatestFrame.clone()",
		"const scheduleVideoEncode = () =>",
		"videoLatestSourceSequence !== lastEncodedSourceSequence",
		"if (!fresh && !videoNeedsKeyframe)",
		"videoPendingFrames.length > 0",
		"encoder.encodeQueueSize > 0",
		"videoPumpWake = scheduleVideoEncode",
		`event.data === "restart"`,
		`sendJSON({type: "inactive"})`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("offscreen source pacing is missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"const captureFrameRate",
		"const outputFrameRate",
		"frameTimestamp - videoLastKeyTimestamp",
		"intervalMS",
		"lastSubmitAt",
		"setTimeout(() =>",
		"nextTick += intervalMS",
		"encoder.encodeQueueSize > 1",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("offscreen source pacing still contains burst/queue behavior %q", forbidden)
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
	copy(message[:4], "SVI2")
	message[4] = 3
	binary.BigEndian.PutUint16(message[6:8], videoHeaderBytes)
	binary.BigEndian.PutUint16(message[8:10], 768)
	binary.BigEndian.PutUint16(message[10:12], 950)
	binary.BigEndian.PutUint32(message[12:16], uint32(len(payload)))
	binary.BigEndian.PutUint32(message[16:20], 42)
	binary.BigEndian.PutUint32(message[20:24], 33333)
	binary.BigEndian.PutUint32(message[24:28], 1200)
	binary.BigEndian.PutUint32(message[28:32], 8400)
	copy(message[videoHeaderBytes:], payload)

	source.handleVideoFrame(message)
	if !got.Key || !got.Fresh || got.SourceSeq != 42 ||
		got.Width != 768 || got.Height != 950 ||
		got.RawGap != 33333*time.Microsecond ||
		got.SubmitWait != 1200*time.Microsecond ||
		got.EncodeTime != 8400*time.Microsecond ||
		string(got.Data) != string(payload) {
		t.Fatalf("decoded frame = %+v", got)
	}
}
