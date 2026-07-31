package media

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const videoHeaderBytes = 20

type EncoderConfig struct {
	Codec     string `json:"codec"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	BitrateK  int    `json:"bitrateK"`
	Quantizer int    `json:"quantizer"`
}

type VideoFrame struct {
	Data      []byte
	Key       bool
	Fresh     bool
	SourceSeq uint32
	Width     int
	Height    int
}

func (s *Capture) StartVideo(config EncoderConfig, handler func(VideoFrame)) error {
	if handler == nil {
		return errors.New("tab capture video handler is required")
	}
	if config.Codec == "" {
		config.Codec = "avc1.42E01F"
	}
	s.mu.Lock()
	if s.closed || s.client == nil {
		s.mu.Unlock()
		return errors.New("tab capture source is not attached")
	}
	previousConfig := s.videoConfig
	s.videoActive = true
	s.videoConfig = config
	s.videoHandler = handler
	if s.videoRunning {
		s.mu.Unlock()
		s.sendVideoConfig()
		return nil
	}
	s.videoReady = make(chan error, 1)
	ready := s.videoReady
	mediaActive := s.mediaActive
	reacquire := mediaActive && previousConfig != config
	if !mediaActive || reacquire {
		s.ready = make(chan error, 1)
	}
	s.mu.Unlock()

	s.sendVideoConfig()
	if reacquire {
		if err := s.stopCaptureForReconfigure(); err != nil {
			s.StopVideo()
			return err
		}
	}
	if !mediaActive || reacquire {
		if _, err := s.triggerActive(true); err != nil {
			s.StopVideo()
			return err
		}
	}
	retry := time.NewTicker(2 * time.Second)
	defer retry.Stop()
	timeout := time.NewTimer(12 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case err := <-ready:
			if err != nil {
				s.StopVideo()
				return err
			}
			log.Printf("video: Chromium tab encoder active")
			return nil
		case <-retry.C:
			// A freshly installed MV3 worker can consume the first action
			// while its offscreen document is still coming up. Re-trigger
			// capture here so the native client does not have to reconnect.
			log.Printf("video: tab capture cold start pending, retrying")
			if _, err := s.triggerActive(false); err != nil {
				log.Printf("video: tab capture retry: %v", err)
			}
		case <-timeout.C:
			s.StopVideo()
			return errors.New("tab capture encoder did not start")
		}
	}
}

// stopCaptureForReconfigure releases the current audio-only (or differently
// sized) tabCapture stream before requesting a new stream ID for the same tab.
// Chromium does not allow two simultaneous captures of one tab, so merely
// triggering the extension again leaves the video transition stuck forever.
func (s *Capture) stopCaptureForReconfigure() error {
	s.mu.Lock()
	conn := s.conn
	if conn == nil || !s.mediaActive {
		s.mu.Unlock()
		return nil
	}
	inactive := make(chan struct{}, 1)
	s.inactive = inactive
	s.mu.Unlock()

	s.writeMu.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	err := conn.WriteMessage(websocket.TextMessage, []byte("restart"))
	_ = conn.SetWriteDeadline(time.Time{})
	s.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("stop tab capture for video reconfigure: %w", err)
	}
	select {
	case <-inactive:
		return nil
	case <-time.After(2 * time.Second):
		return errors.New("tab capture did not stop for video reconfigure")
	}
}

func (s *Capture) StopVideo() {
	s.mu.Lock()
	if !s.videoActive {
		s.mu.Unlock()
		return
	}
	s.videoActive = false
	s.videoRunning = false
	s.videoHandler = nil
	s.videoReady = nil
	videoConn := s.videoConn
	s.mu.Unlock()

	if videoConn != nil {
		s.writeVideoText(videoConn, map[string]any{"type": "stop-video"})
	}
}

func (s *Capture) RequestVideoKeyframe() {
	s.mu.Lock()
	conn, active := s.videoConn, s.videoActive
	s.mu.Unlock()
	if conn != nil && active {
		s.writeVideoText(conn, map[string]any{"type": "keyframe"})
	}
}

func (s *Capture) sendVideoConfig() {
	s.mu.Lock()
	conn, active, config := s.videoConn, s.videoActive, s.videoConfig
	s.mu.Unlock()
	if conn == nil || !active {
		return
	}
	s.writeVideoText(conn, map[string]any{
		"type": "configure", "codec": config.Codec,
		"width": config.Width, "height": config.Height,
		"bitrateK": config.BitrateK, "quantizer": config.Quantizer,
	})
}

func (s *Capture) writeVideoText(conn *websocket.Conn, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	s.videoWriteMu.Lock()
	_ = conn.WriteMessage(websocket.TextMessage, data)
	s.videoWriteMu.Unlock()
}

func (s *Capture) serveVideoBridge(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		http.Error(w, "loopback only", http.StatusForbidden)
		return
	}
	conn, err := (&websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}).Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	previous := s.videoConn
	s.videoConn = conn
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	s.sendVideoConfig()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch messageType {
		case websocket.BinaryMessage:
			s.handleVideoFrame(data)
		case websocket.TextMessage:
			s.handleVideoMessage(data)
		}
	}
	s.mu.Lock()
	if s.videoConn == conn {
		s.videoConn = nil
		s.videoRunning = false
	}
	s.mu.Unlock()
	_ = conn.Close()
}

func (s *Capture) handleVideoFrame(message []byte) {
	if len(message) < videoHeaderBytes || string(message[:4]) != "SVI2" {
		return
	}
	headerBytes := int(binary.BigEndian.Uint16(message[6:8]))
	payloadBytes := int(binary.BigEndian.Uint32(message[12:16]))
	if headerBytes != videoHeaderBytes || payloadBytes < 1 || headerBytes+payloadBytes != len(message) {
		return
	}
	frame := VideoFrame{
		Data:      append([]byte(nil), message[headerBytes:]...),
		Key:       message[4]&1 != 0,
		Fresh:     message[4]&2 != 0,
		SourceSeq: binary.BigEndian.Uint32(message[16:20]),
		Width:     int(binary.BigEndian.Uint16(message[8:10])),
		Height:    int(binary.BigEndian.Uint16(message[10:12])),
	}
	s.mu.Lock()
	handler, active := s.videoHandler, s.videoActive
	s.mu.Unlock()
	if handler != nil && active {
		handler(frame)
	}
}

func (s *Capture) handleVideoMessage(data []byte) {
	var message struct {
		Type          string  `json:"type"`
		Error         string  `json:"error"`
		SourceWidth   int     `json:"sourceWidth"`
		SourceHeight  int     `json:"sourceHeight"`
		SourceFPS     float64 `json:"sourceFPS"`
		CapabilityFPS float64 `json:"sourceCapabilityFPS"`
		CodedWidth    int     `json:"codedWidth"`
		CodedHeight   int     `json:"codedHeight"`
		DisplayWidth  int     `json:"displayWidth"`
		DisplayHeight int     `json:"displayHeight"`
		VisibleWidth  int     `json:"visibleWidth"`
		VisibleHeight int     `json:"visibleHeight"`
		Rotation      float64 `json:"rotation"`
		Constraint    string  `json:"constraint"`
		RateControl   string  `json:"rateControl"`
		Quantizer     int     `json:"quantizer"`
	}
	if json.Unmarshal(data, &message) != nil {
		return
	}
	switch message.Type {
	case "video-active":
		s.mu.Lock()
		s.videoRunning = true
		ready := s.videoReady
		s.mu.Unlock()
		if ready != nil {
			select {
			case ready <- nil:
			default:
			}
		}
		log.Printf("video: tabCapture source %dx%d@%.1ffps capability=%.1ffps quality=%s qp=%d",
			message.SourceWidth, message.SourceHeight,
			message.SourceFPS, message.CapabilityFPS,
			message.RateControl, message.Quantizer)
	case "video-error":
		if message.Error == "" {
			message.Error = "unknown tab capture error"
		}
		err := fmt.Errorf("tab capture: %s", message.Error)
		s.mu.Lock()
		ready := s.videoReady
		s.mu.Unlock()
		if ready != nil {
			select {
			case ready <- err:
			default:
			}
		}
	case "video-warning":
		log.Printf("video: tabCapture constraint fallback constraint=%q: %s",
			message.Constraint, message.Error)
	case "video-frame":
		log.Printf("video: raw tab frame coded=%dx%d visible=%dx%d display=%dx%d rotation=%.0f",
			message.CodedWidth, message.CodedHeight,
			message.VisibleWidth, message.VisibleHeight,
			message.DisplayWidth, message.DisplayHeight, message.Rotation)
	case "video-output":
		log.Printf("video: WebCodecs output coded=%dx%d display=%dx%d",
			message.CodedWidth, message.CodedHeight,
			message.DisplayWidth, message.DisplayHeight)
	}
}
