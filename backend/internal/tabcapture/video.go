package tabcapture

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

const videoHeaderBytes = 24

type VideoConfig struct {
	Codec    string `json:"codec"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FPS      int    `json:"fps"`
	BitrateK int    `json:"bitrateK"`
}

type VideoFrame struct {
	Data      []byte
	Key       bool
	Width     int
	Height    int
	Timestamp uint64
}

func (s *Source) StartVideo(config VideoConfig, handler func(VideoFrame)) error {
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
	if !mediaActive || reacquire {
		if _, err := s.triggerActive(!mediaActive); err != nil {
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

func (s *Source) StopVideo() {
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
	audioConn := s.conn
	stopMedia := s.capture == nil
	if stopMedia {
		s.mediaActive = false
	}
	s.mu.Unlock()

	if videoConn != nil {
		s.writeVideoText(videoConn, map[string]any{"type": "stop-video"})
	}
	if stopMedia && audioConn != nil {
		s.writeMu.Lock()
		_ = audioConn.WriteMessage(websocket.TextMessage, []byte("stop"))
		s.writeMu.Unlock()
	}
}

func (s *Source) RequestVideoKeyframe() {
	s.mu.Lock()
	conn, active := s.videoConn, s.videoActive
	s.mu.Unlock()
	if conn != nil && active {
		s.writeVideoText(conn, map[string]any{"type": "keyframe"})
	}
}

func (s *Source) sendVideoConfig() {
	s.mu.Lock()
	conn, active, config := s.videoConn, s.videoActive, s.videoConfig
	s.mu.Unlock()
	if conn == nil || !active {
		return
	}
	s.writeVideoText(conn, map[string]any{
		"type": "configure", "codec": config.Codec,
		"width": config.Width, "height": config.Height,
		"fps": config.FPS, "bitrateK": config.BitrateK,
	})
}

func (s *Source) writeVideoText(conn *websocket.Conn, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	s.videoWriteMu.Lock()
	_ = conn.WriteMessage(websocket.TextMessage, data)
	s.videoWriteMu.Unlock()
}

func (s *Source) serveVideoBridge(w http.ResponseWriter, r *http.Request) {
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

func (s *Source) handleVideoFrame(message []byte) {
	if len(message) < videoHeaderBytes || string(message[:4]) != "SVI1" {
		return
	}
	headerBytes := int(binary.BigEndian.Uint16(message[6:8]))
	payloadBytes := int(binary.BigEndian.Uint32(message[20:24]))
	if headerBytes != videoHeaderBytes || payloadBytes < 1 || headerBytes+payloadBytes != len(message) {
		return
	}
	frame := VideoFrame{
		Data:      append([]byte(nil), message[headerBytes:]...),
		Key:       message[4]&1 != 0,
		Width:     int(binary.BigEndian.Uint16(message[8:10])),
		Height:    int(binary.BigEndian.Uint16(message[10:12])),
		Timestamp: binary.BigEndian.Uint64(message[12:20]),
	}
	s.mu.Lock()
	handler, active := s.videoHandler, s.videoActive
	s.mu.Unlock()
	if handler != nil && active {
		handler(frame)
	}
}

func (s *Source) handleVideoMessage(data []byte) {
	var message struct {
		Type          string  `json:"type"`
		Error         string  `json:"error"`
		SourceWidth   int     `json:"sourceWidth"`
		SourceHeight  int     `json:"sourceHeight"`
		SourceFPS     float64 `json:"sourceFPS"`
		CodedWidth    int     `json:"codedWidth"`
		CodedHeight   int     `json:"codedHeight"`
		DisplayWidth  int     `json:"displayWidth"`
		DisplayHeight int     `json:"displayHeight"`
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
		log.Printf("video: tabCapture source %dx%d@%.1ffps",
			message.SourceWidth, message.SourceHeight, message.SourceFPS)
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
	case "video-frame":
		log.Printf("video: raw tab frame coded=%dx%d display=%dx%d",
			message.CodedWidth, message.CodedHeight,
			message.DisplayWidth, message.DisplayHeight)
	case "video-output":
		log.Printf("video: WebCodecs output coded=%dx%d display=%dx%d",
			message.CodedWidth, message.CodedHeight,
			message.DisplayWidth, message.DisplayHeight)
	}
}
