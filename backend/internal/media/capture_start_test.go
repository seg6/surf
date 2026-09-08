package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"surf-backend/internal/cdp"
)

// captureTestPeer answers the CDP calls used to acquire a tab. onCapture feeds
// the real bridge handlers, including asynchronous capture/encoder readiness.
func captureTestPeer(t *testing.T, source *Capture, onCapture func(int)) *atomic.Int32 {
	t.Helper()
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var request struct {
				ID     int64  `json:"id"`
				Method string `json:"method"`
			}
			if conn.ReadJSON(&request) != nil {
				return
			}
			result := json.RawMessage(`{}`)
			switch request.Method {
			case "Target.getTargets":
				result = json.RawMessage(`{"targetInfos":[{"targetId":"tab","url":"chrome-extension://capture-test/background.js"}]}`)
			case "Extensions.triggerAction":
				onCapture(int(attempts.Add(1)))
			}
			if conn.WriteJSON(map[string]any{"id": request.ID, "result": result}) != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	client, err := cdp.Dial("ws" + strings.TrimPrefix(server.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	t.Cleanup(func() { _ = source.Close() })
	source.client = client
	source.extensionID = "capture-test"
	return &attempts
}

func startTestVideo(t *testing.T, source *Capture, config EncoderConfig) error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- source.StartVideo(config, func(VideoFrame) {}) }()
	select {
	case err := <-result:
		return err
	case <-time.After(8 * time.Second):
		_ = source.Close()
		<-result
		t.Fatal("capture failure was hidden until the encoder startup timeout")
		return nil
	}
}

func TestVideoStartReportsCaptureFailureAndRecovers(t *testing.T) {
	for _, previouslyActive := range []bool{false, true} {
		name := "fresh"
		if previouslyActive {
			name = "previously_active"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			config := EncoderConfig{Codec: "avc1.42E01F", Width: 768, Height: 974, FrameRate: 60}
			source := &Capture{mediaActive: previouslyActive, videoConfig: config}
			var recoverCapture atomic.Bool
			attempts := captureTestPeer(t, source, func(int) {
				if !recoverCapture.Load() {
					source.handleBridgeMessage([]byte(`{"type":"error","error":"Could not start video source"}`))
					return
				}
				source.handleBridgeMessage([]byte(`{"type":"active","sampleRate":16000}`))
				source.handleVideoMessage([]byte(`{"type":"video-active"}`))
			})
			if err := startTestVideo(t, source, config); err == nil || !strings.Contains(err.Error(), "Could not start video source") {
				t.Fatalf("source error not reported: %v", err)
			}
			source.mu.Lock()
			active, running := source.mediaActive, source.videoRunning
			source.mu.Unlock()
			if active || running {
				t.Fatalf("failed capture still active=%t running=%t", active, running)
			}
			before := attempts.Load()
			recoverCapture.Store(true)
			if err := startTestVideo(t, source, config); err != nil {
				t.Fatalf("capture did not recover: %v", err)
			}
			if attempts.Load() <= before {
				t.Fatal("recovery did not reacquire the failed source")
			}
		})
	}
}

func TestVideoStartReportsFailureAfterCaptureReady(t *testing.T) {
	t.Parallel()
	config := EncoderConfig{Codec: "avc1.42E01F", Width: 800, Height: 600, FrameRate: 60}
	source := &Capture{mediaActive: true, videoConfig: config}
	captureTestPeer(t, source, func(attempt int) {
		if attempt == 1 {
			source.handleBridgeMessage([]byte(`{"type":"active","sampleRate":16000}`))
			return // Capture is ready, but encoder startup still needs a retry.
		}
		source.handleBridgeMessage([]byte(`{"type":"error","error":"Could not start video source"}`))
	})
	if err := startTestVideo(t, source, config); err == nil || !strings.Contains(err.Error(), "Could not start video source") {
		t.Fatalf("error after capture readiness was lost: %v", err)
	}
}

func TestVideoStartPreservesColdCaptureRetry(t *testing.T) {
	t.Parallel()
	source := &Capture{}
	attempts := captureTestPeer(t, source, func(attempt int) {
		if attempt == 1 {
			source.handleBridgeMessage([]byte(`{"type":"error","error":"temporary capture failure"}`))
			return
		}
		source.handleBridgeMessage([]byte(`{"type":"active","sampleRate":16000}`))
		source.handleVideoMessage([]byte(`{"type":"video-active"}`))
	})
	if err := startTestVideo(t, source, EncoderConfig{Width: 800, Height: 600}); err != nil {
		t.Fatalf("transient capture failure prevented recovery: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("capture attempts=%d, want 2", attempts.Load())
	}
}

func TestVideoStartReportsFailureAfterAudioReconnect(t *testing.T) {
	t.Parallel()
	config := EncoderConfig{Codec: "avc1.42E01F", Width: 800, Height: 600, FrameRate: 60}
	source := &Capture{mediaActive: true, videoConfig: config}
	captureTestPeer(t, source, func(int) {
		// Reusing and closing the parked audio source replaces and clears its
		// readiness channel while video startup is still pending.
		audio, err := source.OpenAudio()
		if err != nil {
			t.Errorf("reuse audio: %v", err)
		} else {
			_ = audio.Close()
		}
		source.handleBridgeMessage([]byte(`{"type":"error","error":"Could not start video source"}`))
	})
	if err := startTestVideo(t, source, config); err == nil || !strings.Contains(err.Error(), "Could not start video source") {
		t.Fatalf("audio reconnect hid the video source error: %v", err)
	}
}

func TestVideoStartReportsFailureAfterUnreadCaptureReady(t *testing.T) {
	t.Parallel()
	source := &Capture{}
	captureTestPeer(t, source, func(int) {
		// Both notifications arrive before triggerAction returns. The later
		// failure must win over the queued success.
		source.handleBridgeMessage([]byte(`{"type":"active","sampleRate":16000}`))
		source.handleBridgeMessage([]byte(`{"type":"error","error":"Could not start video source"}`))
	})
	if err := startTestVideo(t, source, EncoderConfig{Width: 800, Height: 600}); err == nil || !strings.Contains(err.Error(), "Could not start video source") {
		t.Fatalf("queued readiness hid the source error: %v", err)
	}
}
