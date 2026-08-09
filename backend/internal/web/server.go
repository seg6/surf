// Package web serves Surf's TLS-only, versioned native API.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"surf-backend/internal/auth"
	"surf-backend/internal/clipboard"
	"surf-backend/internal/config"
	"surf-backend/internal/control"
	"surf-backend/internal/identity"
	"surf-backend/internal/logstore"
	"surf-backend/internal/protocol"
	"surf-backend/internal/transport"
	"surf-backend/internal/updater"

	qrcode "github.com/skip2/go-qrcode"
)

const APIRoot = "/api/v1"

type Server struct {
	cfg           *config.Config
	auth          *auth.Manager
	identity      *identity.Identity
	hub           *transport.Hub
	extra         map[string]http.HandlerFunc
	health        func() error
	stats         func() map[string]any
	client        *clientPackage
	adminToken    string
	shutdown      func()
	clipboardMu   sync.Mutex
	clipboard     map[string]*clipboardRequest
	hostClipboard *clipboard.Controller
	tunnelSlots   chan struct{}
}

type clipboardRequest struct {
	expected map[string]bool
	acked    map[string]bool
	notify   chan struct{}
}

func New(cfg *config.Config, manager *auth.Manager, ident *identity.Identity, hub *transport.Hub) *Server {
	clipboardController, clipboardErr := clipboard.New(cfg.SurfHome)
	if clipboardErr != nil {
		log.Printf("clipboard: settings ignored: %v", clipboardErr)
	}
	s := &Server{
		cfg: cfg, auth: manager, identity: ident, hub: hub,
		extra: map[string]http.HandlerFunc{}, clipboard: map[string]*clipboardRequest{},
		hostClipboard: clipboardController, tunnelSlots: make(chan struct{}, 8),
	}
	hub.SetAuxHandler(s.handleAuxCommand)
	hub.SetConnectHandler(s.handleClientConnected)
	if client := embeddedClientPackage(); client != nil {
		s.client = client
		s.extra[APIRoot+"/updates/client"] = client.ServeHTTP
		log.Printf("updates: embedded iOS client %s protocol %s (%d bytes)", client.Version, client.Protocol, len(client.Data))
	}
	return s
}

func (s *Server) StartClipboardSync(ctx context.Context) {
	s.hostClipboard.Start(ctx, func(state clipboard.State) {
		s.hub.BroadcastJSON(protocol.ClipboardEvent{Type: "clipboard", Text: state.Text, Sync: true})
	})
}

func (s *Server) SetHealthCheck(fn func() error)    { s.health = fn }
func (s *Server) SetStats(fn func() map[string]any) { s.stats = fn }
func (s *Server) SetAdminToken(token string)        { s.adminToken = token }
func (s *Server) SetShutdown(fn func())             { s.shutdown = fn }

func (s *Server) setClientPackage(bundle *clientPackage) {
	s.client = bundle
	if bundle == nil {
		delete(s.extra, APIRoot+"/updates/client")
		return
	}
	s.extra[APIRoot+"/updates/client"] = bundle.ServeHTTP
}

// Gated registers an authenticated API route. Prefix matching is used when
// the pattern ends in a slash.
func (s *Server) Gated(pattern string, handler http.HandlerFunc) {
	if !strings.HasPrefix(pattern, APIRoot+"/") {
		panic("Surf API route is not under /api/v1: " + pattern)
	}
	s.extra[pattern] = handler
}

func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.route) }

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	p := r.URL.Path
	if !strings.HasPrefix(p, APIRoot+"/") && p != APIRoot {
		http.NotFound(w, r)
		return
	}
	switch {
	case p == APIRoot+"/health":
		s.handleHealth(w, r)
		return
	case p == APIRoot+"/server":
		s.handleServerInfo(w, r)
		return
	case p == APIRoot+"/tunnel":
		s.handleTunnel(w, r)
		return
	case p == APIRoot+"/pairing/request":
		s.handlePairRequest(w, r)
		return
	case strings.HasPrefix(p, APIRoot+"/pairing/status/"):
		s.handlePairStatus(w, r, strings.TrimPrefix(p, APIRoot+"/pairing/status/"))
		return
	case strings.HasPrefix(p, APIRoot+"/pairing/confirm/"):
		s.handlePairConfirm(w, r, strings.TrimPrefix(p, APIRoot+"/pairing/confirm/"))
		return
	case strings.HasPrefix(p, APIRoot+"/pairing/cancel/"):
		s.handlePairCancel(w, r, strings.TrimPrefix(p, APIRoot+"/pairing/cancel/"))
		return
	case strings.HasPrefix(p, APIRoot+"/pairing/ack/"):
		s.handlePairAck(w, r, strings.TrimPrefix(p, APIRoot+"/pairing/ack/"))
		return
	case p == APIRoot+"/auth/challenge":
		s.handleAuthChallenge(w, r)
		return
	case p == APIRoot+"/auth/complete":
		s.handleAuthComplete(w, r)
		return
	case p == APIRoot+"/ws":
		s.handleWS(w, r)
		return
	case strings.HasPrefix(p, APIRoot+"/admin/"):
		s.handleAdmin(w, r)
		return
	}

	deviceID, ok := s.auth.Valid(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if p == APIRoot+"/auth/revoke" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.auth.Revoke(deviceID) {
			http.Error(w, "device is no longer paired", http.StatusUnauthorized)
			return
		}
		s.auth.ClearCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if p == APIRoot+"/config" {
		s.handleConfig(w, r, deviceID)
		return
	}
	if p == APIRoot+"/client/logs" {
		s.handleClientLogs(w, r, deviceID)
		return
	}
	for pattern, handler := range s.extra {
		if p == pattern || (strings.HasSuffix(pattern, "/") && strings.HasPrefix(p, pattern)) {
			handler(w, r)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	wantStats := r.URL.Query().Get("stats") == "1" && s.stats != nil
	if wantStats {
		if _, ok := s.auth.Valid(r); !ok && (!isLoopback(r.RemoteAddr) || !s.adminAuthorized(r)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if s.health != nil {
		if err := s.health(); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
	}
	if wantStats {
		writeJSON(w, http.StatusOK, s.stats())
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"api": "v1", "name": s.cfg.ServerName, "serverID": s.identity.Fingerprint,
		"fingerprint": s.identity.Fingerprint, "protocol": config.NativeVersion, "version": config.AppVersion,
		"pairing": s.auth.PairingAvailable(),
	}
	host := strings.ToLower(r.Host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if s.cfg.TunnelHost != "" && host == s.cfg.TunnelHost {
		info["transport"] = "tunnel"
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handlePairRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.auth.AllowPair(r.RemoteAddr) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	var request struct {
		DeviceName string `json:"deviceName"`
		PublicKey  string `json:"publicKey"`
		Code       string `json:"code"`
		QRToken    string `json:"qrToken"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	status, err := s.auth.RequestPair(request.DeviceName, request.PublicKey, r.RemoteAddr, request.Code, request.QRToken)
	if err != nil {
		if errors.Is(err, auth.ErrPairingLocked) {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		if errors.Is(err, auth.ErrPairingClosed) || errors.Is(err, auth.ErrPairingCode) || errors.Is(err, auth.ErrPairingUsed) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if errors.Is(err, auth.ErrAlreadyPaired) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, status)
}

func (s *Server) handlePairCancel(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.auth.RejectPair(id) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePairAck(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.auth.AckPair(id) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePairStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, ok := s.auth.PairStatus(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handlePairConfirm(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.auth.ConfirmPair(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.auth.AllowAuth(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var request struct {
		DeviceID string `json:"deviceID"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	challenge, err := s.auth.NewChallenge(request.DeviceID)
	if err != nil {
		http.Error(w, "unknown device", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, challenge)
}

func (s *Server) handleAuthComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.auth.AllowAuth(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var request struct {
		ChallengeID string `json:"challengeID"`
		Signature   string `json:"signature"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	deviceID, err := s.auth.CompleteChallenge(request.ChallengeID, request.Signature)
	if err != nil {
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}
	s.auth.SetCookie(w, deviceID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	deviceID, ok := s.auth.VerifyWSTicket(query.Get("ticket"))
	if !ok || query.Get("nv") != config.NativeVersion {
		log.Printf("ws rejected: bad ticket or version nv=%q from %s", query.Get("nv"), r.RemoteAddr)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	log.Printf("ws connected device=%s nv=%s from %s", shortID(deviceID), query.Get("nv"), r.RemoteAddr)
	s.hub.ServeHTTPForDevice(w, r, deviceID)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request, deviceID string) {
	clientVersion := r.URL.Query().Get("av")
	clientProtocol := r.URL.Query().Get("nv")
	compatibility := "compatible"
	var update map[string]any
	if clientVersion != "" && s.client != nil && s.client.Protocol == config.NativeVersion && updater.CompareVersions(s.client.Version, clientVersion) > 0 {
		compatibility = "client-update-required"
		update = map[string]any{
			"version": s.client.Version, "protocol": s.client.Protocol,
			"size": len(s.client.Data), "sha256": s.client.SHA256,
			"url": APIRoot + "/updates/client",
		}
	} else if clientProtocol != "" && clientProtocol != config.NativeVersion {
		compatibility = "server-update-required"
	}
	response := map[string]any{
		"ticket": s.auth.WSTicket(deviceID), "vw": s.cfg.ViewW, "vh": s.cfg.ViewH,
		"nv": config.NativeVersion, "version": config.AppVersion, "host": r.Host,
		"caps": config.Caps, "compatibility": compatibility,
	}
	if update != nil {
		response["clientUpdate"] = update
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		http.Error(w, "local access only", http.StatusForbidden)
		return
	}
	if !s.adminAuthorized(r) {
		http.Error(w, "admin authorization required", http.StatusForbidden)
		return
	}
	p := r.URL.Path
	switch {
	case p == APIRoot+"/admin/pairing/session" && r.Method == http.MethodPost:
		var request struct {
			PublicAddress string `json:"publicAddress"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		address := strings.TrimSpace(request.PublicAddress)
		if address == "" {
			address = s.cfg.PublicAddress
		}
		writeJSON(w, http.StatusCreated, s.pairingSessionResponse(s.auth.OpenPairing(), address))
	case p == APIRoot+"/admin/pairing/session" && r.Method == http.MethodGet:
		session, ok := s.auth.PairingSession()
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{
				"active": false, "serverID": s.identity.Fingerprint, "name": s.cfg.ServerName,
				"publicAddress": s.cfg.PublicAddress,
			})
			return
		}
		writeJSON(w, http.StatusOK, s.pairingSessionResponse(session, s.cfg.PublicAddress))
	case p == APIRoot+"/admin/pairing/session" && r.Method == http.MethodDelete:
		s.auth.ClosePairing()
		w.WriteHeader(http.StatusNoContent)
	case p == APIRoot+"/admin/pairing/candidates" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"candidates": s.auth.ListCandidates()})
	case strings.HasPrefix(p, APIRoot+"/admin/pairing/reject/") && r.Method == http.MethodPost:
		if !s.auth.RejectPair(strings.TrimPrefix(p, APIRoot+"/admin/pairing/reject/")) {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case p == APIRoot+"/admin/devices" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"devices": s.auth.ListDevices()})
	case strings.HasPrefix(p, APIRoot+"/admin/devices/revoke/") && r.Method == http.MethodPost:
		if !s.auth.Revoke(strings.TrimPrefix(p, APIRoot+"/admin/devices/revoke/")) {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case p == APIRoot+"/admin/logs/sources" && r.Method == http.MethodGet:
		online := map[string]bool{}
		for _, deviceID := range s.hub.ConnectedDevices() {
			online[deviceID] = true
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": s.auth.ListDevices(), "online": online})
	case p == APIRoot+"/admin/logs" && r.Method == http.MethodGet:
		s.handleAdminLogs(w, r)
	case p == APIRoot+"/admin/logs/stream" && r.Method == http.MethodGet:
		s.handleAdminLogStream(w, r)
	case p == APIRoot+"/admin/logs/refresh" && r.Method == http.MethodPost:
		var request struct {
			DeviceID string `json:"deviceID"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		targets := s.hub.SendDeviceJSON(request.DeviceID, protocol.EmptyEvent{Type: "log-request"})
		writeJSON(w, http.StatusOK, map[string]any{"devices": targets})
	case p == APIRoot+"/admin/clipboard" && r.Method == http.MethodGet:
		s.handleAdminClipboardState(w, r)
	case p == APIRoot+"/admin/clipboard" && r.Method == http.MethodPost:
		s.handleAdminClipboard(w, r)
	case p == APIRoot+"/admin/clipboard/sync" && r.Method == http.MethodPut:
		s.handleAdminClipboardSync(w, r)
	case p == APIRoot+"/admin/shutdown" && r.Method == http.MethodPost:
		if s.shutdown == nil {
			http.Error(w, "shutdown unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		go s.shutdown()
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleClientConnected(client *transport.Client) {
	state := s.hostClipboard.State()
	client.SendJSON(protocol.ClipboardSyncEvent{
		Type: "clipboard-sync", Enabled: state.Enabled, Known: state.Known, Text: state.Text,
	})
}

func (s *Server) handleAuxCommand(client *transport.Client, command protocol.Command) bool {
	switch value := command.(type) {
	case *protocol.ClipboardResultCommand:
		s.clipboardMu.Lock()
		pending := s.clipboard[value.RequestID]
		if pending != nil && pending.expected[client.DeviceID()] && !pending.acked[client.DeviceID()] {
			pending.acked[client.DeviceID()] = value.OK
			select {
			case pending.notify <- struct{}{}:
			default:
			}
		}
		s.clipboardMu.Unlock()
		return true
	case *protocol.ClipboardChangeCommand:
		state, changed, err := s.hostClipboard.SetRemote(context.Background(), value.Text)
		if err != nil && !errors.Is(err, clipboard.ErrUnavailable) {
			log.Printf("clipboard: device-to-host bridge failed system=%s: %v", state.System, err)
		}
		if changed {
			s.hub.BroadcastJSON(protocol.ClipboardEvent{Type: "clipboard", Text: state.Text, Sync: true})
		}
		return true
	case *protocol.LogRecordCommand:
		if err := logstore.AppendDeviceRecord(s.cfg.SurfHome, client.DeviceID(), value.Record); err != nil {
			log.Printf("native log: rejected live record device=%s: %v", shortID(client.DeviceID()), err)
		}
		return true
	default:
		return false
	}
}

func (s *Server) handleAdminClipboard(w http.ResponseWriter, r *http.Request) {
	var request struct {
		DeviceID string `json:"deviceID"`
		Text     string `json:"text"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := clipboard.Validate(request.Text); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	state := s.hostClipboard.State()
	var hostErr error
	if request.DeviceID == "" {
		state, _, hostErr = s.hostClipboard.SetHost(r.Context(), request.Text)
	}
	targets := s.hub.ConnectedDevices()
	if request.DeviceID != "" {
		filtered := targets[:0]
		for _, deviceID := range targets {
			if deviceID == request.DeviceID {
				filtered = append(filtered, deviceID)
			}
		}
		targets = filtered
		if len(targets) == 0 {
			http.Error(w, "no matching paired device is connected", http.StatusConflict)
			return
		}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		http.Error(w, "could not create clipboard request", http.StatusInternalServerError)
		return
	}
	requestID := hex.EncodeToString(random)
	pending := &clipboardRequest{expected: map[string]bool{}, acked: map[string]bool{}, notify: make(chan struct{}, 1)}
	s.clipboardMu.Lock()
	s.clipboard[requestID] = pending
	s.clipboardMu.Unlock()
	defer func() {
		s.clipboardMu.Lock()
		delete(s.clipboard, requestID)
		s.clipboardMu.Unlock()
	}()
	s.clipboardMu.Lock()
	for _, deviceID := range targets {
		pending.expected[deviceID] = true
	}
	s.clipboardMu.Unlock()
	syncValue := request.DeviceID == "" && state.Enabled
	s.hub.SendDeviceJSON(request.DeviceID, protocol.ClipboardEvent{
		Type: "clipboard", RequestID: requestID, Text: request.Text, Sync: syncValue,
	})
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		s.clipboardMu.Lock()
		complete := len(pending.acked) == len(pending.expected)
		s.clipboardMu.Unlock()
		if complete {
			break
		}
		select {
		case <-pending.notify:
		case <-deadline.C:
			goto finished
		}
	}

finished:
	s.clipboardMu.Lock()
	delivered, failed := []string{}, []string{}
	for _, deviceID := range targets {
		if pending.acked[deviceID] {
			delivered = append(delivered, deviceID)
		} else {
			failed = append(failed, deviceID)
		}
	}
	s.clipboardMu.Unlock()
	hostError := ""
	if hostErr != nil {
		hostError = hostErr.Error()
	}
	log.Printf("clipboard: bytes=%d sync=%t targets=%d delivered=%d failed=%d host_ok=%t",
		len([]byte(request.Text)), syncValue, len(targets), len(delivered), len(failed), hostErr == nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": len(failed) == 0, "delivered": delivered, "failed": failed,
		"enabled": state.Enabled, "available": state.Available, "system": state.System,
		"hostOK": hostErr == nil, "hostError": hostError,
	})
}

func clipboardStatePayload(state clipboard.State, err error) map[string]any {
	errorText := ""
	if err != nil {
		errorText = err.Error()
	}
	return map[string]any{
		"enabled": state.Enabled, "available": state.Available, "system": state.System,
		"known": state.Known, "text": state.Text, "revision": state.Revision, "error": errorText,
	}
}

func (s *Server) handleAdminClipboardState(w http.ResponseWriter, r *http.Request) {
	state, err := s.hostClipboard.Read(r.Context())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, clipboardStatePayload(state, err))
}

func (s *Server) handleAdminClipboardSync(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	state, err := s.hostClipboard.SetEnabled(r.Context(), request.Enabled)
	if state.Enabled != request.Enabled {
		message := "could not update clipboard sync preference"
		if err != nil {
			message = err.Error()
		}
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	s.hub.BroadcastJSON(protocol.ClipboardSyncEvent{
		Type: "clipboard-sync", Enabled: state.Enabled, Known: state.Known, Text: state.Text,
	})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, clipboardStatePayload(state, err))
}

func (s *Server) handleClientLogs(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, logstore.DeviceMaxBytes+1))
	if err != nil {
		http.Error(w, "read native log", http.StatusBadRequest)
		return
	}
	if len(data) > logstore.DeviceMaxBytes {
		http.Error(w, "native log is too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := logstore.WriteDeviceSnapshot(s.cfg.SurfHome, deviceID, data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	data, err := s.readAdminLogs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) readAdminLogs(r *http.Request) ([]byte, error) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "server"
	}
	var path string
	var rotated bool
	switch source {
	case "server":
		path, rotated = logstore.ServerPath(s.cfg.SurfHome), true
	case "desktop":
		path, rotated = logstore.DesktopPath(s.cfg.SurfHome), true
	case "device":
		return logstore.ReadDeviceSnapshot(s.cfg.SurfHome, r.URL.Query().Get("deviceID"), logstore.DefaultReadBytes)
	default:
		return nil, errors.New("unknown log source")
	}
	var data []byte
	var err error
	if rotated {
		data, err = logstore.ReadRotated(path, logstore.DefaultReadBytes)
	} else {
		data, err = logstore.ReadTail(path, logstore.DefaultReadBytes)
	}
	return data, err
}

func (s *Server) handleAdminLogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "live logs unavailable", http.StatusInternalServerError)
		return
	}
	current, err := s.readAdminLogs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	controller := http.NewResponseController(w)
	send := func(text []byte, reset bool) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
		data, _ := json.Marshal(map[string]any{"text": string(text), "reset": reset})
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send(current, true) {
		return
	}
	previous := string(current)
	lastWrite := time.Now()
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
		next, err := s.readAdminLogs(r)
		if err != nil {
			return
		}
		text := string(next)
		if text == previous {
			if time.Since(lastWrite) >= 15*time.Second {
				_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
				lastWrite = time.Now()
			}
			continue
		}
		reset := !strings.HasPrefix(text, previous)
		chunk := next
		if !reset {
			chunk = next[len(previous):]
		}
		if !send(chunk, reset) {
			return
		}
		lastWrite = time.Now()
		previous = text
	}
}

func (s *Server) pairingSessionResponse(session auth.PairingSession, address string) map[string]any {
	response := map[string]any{
		"active": true, "id": session.ID, "code": session.Code, "createdAt": session.CreatedAt,
		"attempts": session.Attempts, "candidateID": session.CandidateID,
		"serverID": s.identity.Fingerprint, "name": s.cfg.ServerName,
		"publicAddress": strings.TrimSpace(address),
	}
	if link := pairingLink(address, s.identity.Fingerprint, session.QRToken); link != "" {
		response["pairingURL"] = link
		if png, err := qrcode.Encode(link, qrcode.Medium, 320); err == nil {
			response["qrPNG"] = base64.StdEncoding.EncodeToString(png)
		}
	}
	return response
}

func (s *Server) adminAuthorized(r *http.Request) bool {
	provided := r.Header.Get(control.AdminHeader)
	return s.adminToken != "" && len(provided) == len(s.adminToken) &&
		subtle.ConstantTimeCompare([]byte(provided), []byte(s.adminToken)) == 1
}

func pairingLink(address, serverID, token string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if parsed, err := url.Parse(address); err == nil && parsed.Host != "" {
		address = parsed.Host
	} else {
		address = strings.TrimPrefix(strings.TrimPrefix(address, "https://"), "http://")
	}
	// Keep the visual payload compact: fewer QR modules make each module
	// materially larger on a monitor and much easier for old cameras to read.
	// A 128-bit fingerprint prefix still strongly binds the scanned invitation
	// to the certificate; the client saves the full observed fingerprint.
	if len(serverID) > 32 {
		serverID = serverID[:32]
	}
	query := url.Values{"h": {address}, "i": {serverID}, "t": {token}}
	return "surf://pair?" + query.Encode()
}

func ListenTLS(bindAddr string, port int, ident *identity.Identity, handler http.Handler) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(bindAddr, fmt.Sprint(port)))
	if err != nil {
		return err
	}
	return ServeTLS(listener, ident, handler)
}

func TLSConfig(ident *identity.Identity) *tls.Config {
	return &tls.Config{
		MinVersion:             tls.VersionTLS12,
		Certificates:           []tls.Certificate{ident.Certificate},
		SessionTicketsDisabled: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		},
	}
}

// ServeTLS is split from ListenTLS so transport security can be exercised on
// an ephemeral listener in tests.
func ServeTLS(listener net.Listener, ident *identity.Identity, handler http.Handler) error {
	tlsConfig := TLSConfig(ident)
	server := &http.Server{
		Handler: handler, TLSConfig: tlsConfig,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second,
	}
	return server.Serve(tls.NewListener(listener, tlsConfig))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func isLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
