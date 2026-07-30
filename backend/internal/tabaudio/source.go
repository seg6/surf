// Package tabaudio captures Chromium's active tab on every supported host
// without rendering that audio locally. A built-in extension sends 16 kHz
// mono PCM over a loopback-only WebSocket to Surf's audio pipeline.
package tabaudio

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"surf-backend/internal/cdp"
)

const extensionName = "Surf Audio Bridge"

//go:embed extension/*
var extensionFiles embed.FS

type Source struct {
	extensionPath string
	listener      net.Listener
	server        *http.Server

	mu          sync.Mutex
	client      *cdp.Client
	extensionID string
	conn        *websocket.Conn
	capture     *capture
	ready       chan error
	closed      bool
	writeMu     sync.Mutex
}

type capture struct {
	source *Source
	reader *io.PipeReader
	writer *io.PipeWriter
	once   sync.Once
}

type bridgeMessage struct {
	Type       string `json:"type"`
	Error      string `json:"error"`
	SampleRate int    `json:"sampleRate"`
}

func New(home string) (*Source, error) {
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return nil, fmt.Errorf("tab audio token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes[:])
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("tab audio listener: %w", err)
	}
	s := &Source{listener: listener}
	extensionPath := filepath.Join(home, "runtime", "tab-audio-extension")
	if err := extractExtension(extensionPath, listener.Addr().String(), token); err != nil {
		_ = listener.Close()
		return nil, err
	}
	s.extensionPath = extensionPath

	mux := http.NewServeMux()
	mux.HandleFunc("/audio/"+token, s.serveBridge)
	s.server = &http.Server{Handler: mux}
	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("tab audio bridge: %v", err)
		}
	}()
	return s, nil
}

func extractExtension(dst, address, token string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return fmt.Errorf("tab audio extension directory: %w", err)
	}
	entries, err := fs.ReadDir(extensionFiles, "extension")
	if err != nil {
		return fmt.Errorf("tab audio extension assets: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "config.js" {
			continue
		}
		data, err := extensionFiles.ReadFile("extension/" + entry.Name())
		if err != nil {
			return fmt.Errorf("tab audio extension %s: %w", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), data, 0o600); err != nil {
			return fmt.Errorf("tab audio extension %s: %w", entry.Name(), err)
		}
	}
	url := "ws://" + address + "/audio/" + token
	config := "globalThis.SURF_AUDIO_CONFIG = {url: " + strconv.Quote(url) + "};\n"
	if err := os.WriteFile(filepath.Join(dst, "config.js"), []byte(config), 0o600); err != nil {
		return fmt.Errorf("tab audio extension config: %w", err)
	}
	return nil
}

func (s *Source) ExtensionPath() string { return s.extensionPath }

// Attach finds the built-in extension after Chromium has loaded it.
func (s *Source) Attach(client *cdp.Client) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		raw, err := client.Call("", "Extensions.getExtensions", nil)
		if err == nil {
			var installed struct {
				Extensions []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"extensions"`
			}
			if json.Unmarshal(raw, &installed) == nil {
				for _, extension := range installed.Extensions {
					if extension.Name == extensionName {
						s.mu.Lock()
						s.client = client
						s.extensionID = extension.ID
						s.mu.Unlock()
						return nil
					}
				}
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("tab audio extension: %w", err)
			}
			return fmt.Errorf("tab audio extension %q was not loaded", extensionName)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Open starts capture of the active Chromium tab and returns signed
// little-endian 16 kHz mono PCM.
func (s *Source) Open() (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	next := &capture{source: s, reader: reader, writer: writer}

	s.mu.Lock()
	if s.closed || s.client == nil || s.extensionID == "" {
		s.mu.Unlock()
		_ = reader.Close()
		_ = writer.Close()
		return nil, errors.New("tab audio source is not attached")
	}
	previous := s.capture
	s.capture = next
	s.ready = make(chan error, 1)
	ready := s.ready
	s.mu.Unlock()
	if previous != nil {
		previous.close(false)
	}

	started, err := s.triggerActive(true)
	if err != nil {
		next.close(true)
		return nil, err
	}
	if started {
		log.Printf("audio: Chromium tab capture active")
		return next, nil
	}
	select {
	case err := <-ready:
		if err != nil {
			next.close(true)
			return nil, err
		}
		log.Printf("audio: Chromium tab capture active")
		return next, nil
	case <-time.After(10 * time.Second):
		next.close(true)
		return nil, errors.New("tab audio extension did not start capture")
	}
}

// SwitchActive moves a running capture to Chromium's newly active tab.
func (s *Source) SwitchActive() {
	s.mu.Lock()
	active := s.capture != nil
	s.mu.Unlock()
	if !active {
		return
	}
	if _, err := s.triggerActive(false); err != nil {
		log.Printf("audio: switch active tab: %v", err)
	}
}

func (s *Source) triggerActive(coldStart bool) (bool, error) {
	s.mu.Lock()
	client, extensionID := s.client, s.extensionID
	s.mu.Unlock()
	if client == nil || extensionID == "" {
		return false, errors.New("tab audio source is not attached")
	}
	targetID, err := activeTabTarget(client)
	if err != nil {
		return false, err
	}
	trigger := func() error {
		_, err := client.Call("", "Extensions.triggerAction", map[string]any{
			"id": extensionID, "targetId": targetID,
		})
		return err
	}
	if err := trigger(); err != nil {
		return false, fmt.Errorf("trigger tab audio extension: %w", err)
	}
	if !coldStart {
		return false, nil
	}
	// Usually the listener is already registered and the first action starts
	// capture. Do not immediately fire a second action: two overlapping
	// getMediaStreamId calls can invalidate one another.
	s.mu.Lock()
	ready := s.ready
	s.mu.Unlock()
	if ready != nil {
		select {
		case err := <-ready:
			if err == nil {
				return true, nil
			}
			log.Printf("audio: initial tab capture attempt failed, retrying: %v", err)
		case <-time.After(500 * time.Millisecond):
		}
	}
	// A freshly installed MV3 extension may need the first action only to
	// start its service worker. Once its listener is registered, trigger the
	// action again.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if extensionWorkerReady(client, extensionID) {
			time.Sleep(50 * time.Millisecond)
			if err := trigger(); err != nil {
				return false, fmt.Errorf("trigger initialized tab audio extension: %w", err)
			}
			return false, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false, errors.New("tab audio extension service worker did not start")
}

func activeTabTarget(client *cdp.Client) (string, error) {
	raw, err := client.Call("", "Target.getTargets", map[string]any{
		"filter": []map[string]any{{"type": "tab", "exclude": false}},
	})
	if err != nil {
		return "", fmt.Errorf("list tab targets: %w", err)
	}
	var result struct {
		Targets []struct {
			ID           string          `json:"targetId"`
			EmbedderData json.RawMessage `json:"embedderData"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode tab targets: %w", err)
	}
	var fallback string
	for _, target := range result.Targets {
		if fallback == "" {
			fallback = target.ID
		}
		var embedder struct {
			Active bool `json:"tabActive"`
		}
		if json.Unmarshal(target.EmbedderData, &embedder) == nil && embedder.Active {
			return target.ID, nil
		}
	}
	if fallback == "" {
		return "", errors.New("no Chromium tab target")
	}
	return fallback, nil
}

func extensionWorkerReady(client *cdp.Client, extensionID string) bool {
	raw, err := client.Call("", "Target.getTargets", map[string]any{
		"filter": []map[string]any{{"exclude": false}},
	})
	if err != nil {
		return false
	}
	var result struct {
		Targets []struct {
			URL string `json:"url"`
		} `json:"targetInfos"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return false
	}
	want := "chrome-extension://" + extensionID + "/background.js"
	for _, target := range result.Targets {
		if target.URL == want {
			return true
		}
	}
	return false
}

func (s *Source) serveBridge(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
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
	previous := s.conn
	s.conn = conn
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch messageType {
		case websocket.BinaryMessage:
			s.mu.Lock()
			current := s.capture
			s.mu.Unlock()
			if current != nil {
				if _, err := current.writer.Write(data); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					log.Printf("tab audio PCM bridge: %v", err)
				}
			}
		case websocket.TextMessage:
			s.handleBridgeMessage(data)
		}
	}
	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
	}
	s.mu.Unlock()
	_ = conn.Close()
}

func (s *Source) handleBridgeMessage(data []byte) {
	var message bridgeMessage
	if json.Unmarshal(data, &message) != nil {
		return
	}
	switch message.Type {
	case "active":
		if message.SampleRate != 16000 {
			s.signalReady(fmt.Errorf("tab audio sample rate is %d, want 16000", message.SampleRate))
		} else {
			s.signalReady(nil)
		}
	case "error":
		if strings.TrimSpace(message.Error) == "" {
			message.Error = "unknown extension error"
		}
		s.signalReady(errors.New(message.Error))
	}
}

func (s *Source) signalReady(err error) {
	s.mu.Lock()
	ready := s.ready
	s.mu.Unlock()
	if ready != nil {
		select {
		case ready <- err:
		default:
		}
	}
}

func (c *capture) Read(p []byte) (int, error) { return c.reader.Read(p) }

func (c *capture) Close() error {
	c.close(true)
	return nil
}

func (c *capture) close(stopExtension bool) {
	c.once.Do(func() {
		s := c.source
		s.mu.Lock()
		if s.capture == c {
			s.capture = nil
			s.ready = nil
		}
		conn := s.conn
		s.mu.Unlock()
		_ = c.writer.Close()
		_ = c.reader.Close()
		if stopExtension && conn != nil {
			s.writeMu.Lock()
			_ = conn.WriteMessage(websocket.TextMessage, []byte("stop"))
			s.writeMu.Unlock()
		}
	})
}

func (s *Source) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	current, conn := s.capture, s.conn
	s.capture, s.conn = nil, nil
	s.mu.Unlock()
	if current != nil {
		current.close(false)
	}
	if conn != nil {
		_ = conn.Close()
	}
	if s.server != nil {
		_ = s.server.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	return nil
}
