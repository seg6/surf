// Package httpd serves the Surf native client's API surface: password login,
// /native-config, feature routes (downloads, tabicons), and the WebSocket
// endpoint. There is intentionally no browser UI.
package httpd

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"surf-backend/internal/auth"
	"surf-backend/internal/clientupdate"
	"surf-backend/internal/config"
	"surf-backend/internal/updater"
	"surf-backend/internal/ws"
)

type Server struct {
	cfg    *config.Config
	auth   *auth.Auth
	hub    *ws.Hub
	extra  map[string]http.HandlerFunc // feature routes (downloads, tabicons)
	health func() error
	stats  func() map[string]any
	client *clientupdate.Bundle
}

func New(cfg *config.Config, a *auth.Auth, hub *ws.Hub) (*Server, error) {
	return &Server{
		cfg: cfg, auth: a, hub: hub,
		extra: map[string]http.HandlerFunc{},
	}, nil
}

func (s *Server) SetHealthCheck(fn func() error) { s.health = fn }

// SetStats wires the runtime snapshot served at /health?stats=1.
func (s *Server) SetStats(fn func() map[string]any) { s.stats = fn }

func (s *Server) SetClientUpdate(bundle *clientupdate.Bundle) {
	s.client = bundle
	if bundle != nil {
		s.extra["/updates/v1/client"] = bundle.ServeHTTP
	}
}

// Gated registers an auth-required feature route (/downloads/, /tabicon/).
// Prefix match when the pattern ends with '/'.
func (s *Server) Gated(pattern string, h http.HandlerFunc) {
	s.extra[pattern] = h
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.route)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path

	switch {
	case p == "/health":
		wantStats := r.URL.Query().Get("stats") == "1" && s.stats != nil
		if wantStats && !s.auth.Valid(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if s.health != nil {
			if err := s.health(); err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		if wantStats {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(s.stats())
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
		return
	case p == "/favicon.ico":
		w.WriteHeader(http.StatusNoContent)
		return
	case p == "/login":
		s.handleLogin(w, r)
		return
	case p == "/logout":
		s.auth.ClearCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	case p == "/ws":
		s.handleWS(w, r)
		return
	}

	if !s.auth.Valid(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if p == "/native-config" {
		s.handleNativeConfig(w, r)
		return
	}
	for pattern, h := range s.extra {
		if p == pattern || (strings.HasSuffix(pattern, "/") && strings.HasPrefix(p, pattern)) {
			h(w, r)
			return
		}
	}
	http.NotFound(w, r)
}

// handleWS verifies the ticket and native protocol version, then hands the
// connection to the hub.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if !s.auth.VerifyWSTicket(q.Get("ticket")) || q.Get("nv") != config.NativeVersion {
		log.Printf("ws rejected: bad ticket or version nv=%q (want %s) from %s", q.Get("nv"), config.NativeVersion, r.RemoteAddr)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	log.Printf("ws connected nv=%s from %s", q.Get("nv"), r.RemoteAddr)
	s.hub.Serve(w, r)
}

func (s *Server) handleNativeConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	clientVersion := r.URL.Query().Get("av")
	clientProtocol := r.URL.Query().Get("nv")
	compatibility := "compatible"
	var update map[string]any
	if clientProtocol != "" && clientProtocol != config.NativeVersion {
		compatibility = "server-update-required"
		if s.client != nil && s.client.Protocol == config.NativeVersion &&
			updater.CompareVersions(s.client.Version, clientVersion) > 0 {
			compatibility = "client-update-required"
			update = map[string]any{
				"version": s.client.Version, "protocol": s.client.Protocol,
				"size": len(s.client.Data), "sha256": s.client.SHA256,
				"url": "/updates/v1/client",
			}
		}
	}
	response := map[string]any{
		"ticket": s.auth.WSTicket(), "vw": s.cfg.ViewW, "vh": s.cfg.ViewH,
		"nv": config.NativeVersion, "version": config.AppVersion, "host": r.Host,
		"caps": config.Caps, "compatibility": compatibility,
	}
	if update != nil {
		response["clientUpdate"] = update
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.auth.Allow(r.RemoteAddr) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if s.auth.CheckPassword(r.PostForm.Get("password")) {
		s.auth.SetCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func Listen(bindAddr string, port int, h http.Handler) error {
	srv := &http.Server{
		Addr:              net.JoinHostPort(bindAddr, fmt.Sprint(port)),
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServe()
}
