// Package httpd serves the gated client page, the login flow, static assets
// and the WebSocket endpoint. Everything except /health, /login and the app
// icons requires the auth cookie; /ws is instead gated by the token that only
// ships inside the authenticated page (iOS 6 Safari can't be trusted to send
// cookies/Basic-Auth on the upgrade through every proxy).
package httpd

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	rbrowser "rbrowser"
	"rbrowser/internal/auth"
	"rbrowser/internal/config"
	"rbrowser/internal/ws"
)

type Server struct {
	cfg    *config.Config
	auth   *auth.Auth
	hub    *ws.Hub
	pub    fs.FS
	index  []byte
	extra  map[string]http.HandlerFunc // feature routes (downloads, tabicons)
}

func New(cfg *config.Config, a *auth.Auth, hub *ws.Hub) (*Server, error) {
	pub, err := fs.Sub(rbrowser.Public, "public")
	if err != nil {
		return nil, err
	}
	idx, err := fs.ReadFile(pub, "index.html")
	if err != nil {
		return nil, err
	}
	html := strings.Replace(string(idx), "__WS_TOKEN__", a.Token, 1)
	html = strings.ReplaceAll(html, "__CLIENT_VERSION__", config.ClientVersion)
	return &Server{
		cfg: cfg, auth: a, hub: hub, pub: pub,
		index: []byte(html),
		extra: map[string]http.HandlerFunc{},
	}, nil
}

// Gated registers an auth-required feature route (M3: /downloads/, /tabicon/).
// Prefix match when the pattern ends with '/'.
func (s *Server) Gated(pattern string, h http.HandlerFunc) {
	s.extra[pattern] = h
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.route)
}

var iconPaths = map[string]string{
	"/icon.png":                        "icon.png",
	"/apple-touch-icon.png":            "icon.png",
	"/apple-touch-icon-precomposed.png": "icon.png",
	"/icon-57.png":                     "icon-57.png",
	"/icon-72.png":                     "icon-72.png",
	"/icon-114.png":                    "icon-114.png",
	"/icon-144.png":                    "icon-144.png",
}

var staticAssets = map[string]string{
	"/app.js":    "application/javascript; charset=utf-8",
	"/style.css": "text/css; charset=utf-8",
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path

	switch {
	case p == "/health":
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
		return
	case p == "/favicon.ico":
		w.WriteHeader(http.StatusNoContent)
		return
	case iconPaths[p] != "":
		s.serveEmbedded(w, iconPaths[p], "image/png", "public, max-age=86400")
		return
	case p == "/login":
		s.handleLogin(w, r)
		return
	case p == "/logout":
		s.auth.ClearCookie(w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	case p == "/ws":
		s.handleWS(w, r)
		return
	}

	if !s.auth.Valid(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if ct, ok := staticAssets[p]; ok {
		s.serveEmbedded(w, strings.TrimPrefix(p, "/"), ct, "public, max-age=31536000")
		return
	}
	for pattern, h := range s.extra {
		if p == pattern || (strings.HasSuffix(pattern, "/") && strings.HasPrefix(p, pattern)) {
			h(w, r)
			return
		}
	}
	if p == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(s.index)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) serveEmbedded(w http.ResponseWriter, name, contentType, cache string) {
	b, err := fs.ReadFile(s.pub, name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cache)
	_, _ = w.Write(b)
}

// handleWS verifies the in-page token and protocol version, then hands the
// connection to the hub.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("k") != s.auth.Token || q.Get("v") != config.ClientVersion {
		log.Printf("ws rejected: bad token or version %q (want %s) from %s", q.Get("v"), config.ClientVersion, r.RemoteAddr)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	log.Printf("ws connected from %s", r.RemoteAddr)
	s.hub.Serve(w, r)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendLogin(w, "")
		return
	}
	if !s.auth.Allow(r.RemoteAddr) {
		s.sendLogin(w, "Too many attempts — wait a minute.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		s.sendLogin(w, "Wrong password.")
		return
	}
	if s.auth.CheckPassword(r.PostForm.Get("password")) {
		s.auth.SetCookie(w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.sendLogin(w, "Wrong password.")
}

func (s *Server) sendLogin(w http.ResponseWriter, errMsg string) {
	msg := ""
	if errMsg != "" {
		msg = "<p>" + errMsg + "</p>"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, strings.Replace(loginHTML, "__ERR__", msg, 1))
}

// loginHTML shares the app's design language: graphite chassis, Avenir Next
// (ships on iOS 6), one gold accent.
const loginHTML = `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no">
<meta name="apple-mobile-web-app-capable" content="yes">
<link rel="apple-touch-icon-precomposed" href="/icon-72.png">
<title>wrp</title>
<style>
html,body{margin:0;height:100%;background:#0e0f11;color:#eceded;font-family:"Avenir Next","Helvetica Neue",Helvetica,Arial,sans-serif;}
body{display:-webkit-box;display:flex;-webkit-box-align:center;align-items:center;-webkit-box-pack:center;justify-content:center;}
form{width:300px;padding:30px 26px 26px;background:#191b1e;border:1px solid #2c2e33;border-radius:12px;}
.mark{width:44px;height:44px;margin:0 0 14px;border-radius:10px;}
h1{margin:0 0 2px;font-size:26px;font-weight:600;letter-spacing:0.5px;}
.sub{margin:0 0 22px;color:#8f939a;font-size:13px;letter-spacing:0.3px;}
input,button{display:block;width:100%;height:44px;border:0;border-radius:9px;font-size:17px;font-family:inherit;-webkit-box-sizing:border-box;box-sizing:border-box;}
input{margin:0 0 12px;padding:0 12px;background:#101215;color:#eceded;border:1px solid #2c2e33;-webkit-appearance:none;}
input:focus{border-color:#d9a441;outline:none;}
button{background:#d9a441;color:#17140c;font-weight:600;}
button:active{background:#c2913a;}
p{margin:14px 0 0;color:#c4554d;font-size:14px;}
</style></head><body>
<form method="post" action="/login"><img class="mark" src="/icon-72.png" alt="">
<h1>wrp</h1><div class="sub">Your browser, elsewhere</div>
<input type="password" name="password" placeholder="Password" autofocus>
<button type="submit">Log in</button>__ERR__</form></body></html>`

func Listen(port int, h http.Handler) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), h)
}
