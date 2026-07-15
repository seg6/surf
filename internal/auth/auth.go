// Package auth implements the password gate: bcrypt-checked login, an
// HMAC-signed expiry cookie (v1.<exp>.<hex hmac>), and the WS token that
// only ships inside the gated page (iOS 6 Safari can't be trusted to send
// auth headers on the WS upgrade).
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const CookieName = "rbrowser_auth"

type Auth struct {
	hash     string
	secret   []byte
	Token    string // WS handshake token
	days     int
	mu       sync.Mutex
	attempts map[string][]time.Time // login rate limit per IP
}

// readOrCreateSecret keeps secrets stable across restarts by persisting them
// in the profile dir (same files server.js used: .wstoken / .authsecret).
func readOrCreateSecret(file string, bytes int) string {
	if b, err := os.ReadFile(file); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			return v
		}
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	v := hex.EncodeToString(buf)
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	_ = os.WriteFile(file, []byte(v), 0o600)
	return v
}

func New(profile, bcryptHash string, days int) *Auth {
	return &Auth{
		hash:     bcryptHash,
		secret:   []byte(readOrCreateSecret(filepath.Join(profile, ".authsecret"), 32)),
		Token:    readOrCreateSecret(filepath.Join(profile, ".wstoken"), 16),
		days:     max(1, days),
		attempts: map[string][]time.Time{},
	}
}

func (a *Auth) sign(exp int64) string {
	m := hmac.New(sha256.New, a.secret)
	fmt.Fprintf(m, "%d", exp)
	return hex.EncodeToString(m.Sum(nil))
}

func (a *Auth) cookieValue() string {
	exp := time.Now().Unix() + int64(a.days)*86400
	return fmt.Sprintf("v1.%d.%s", exp, a.sign(exp))
}

// Valid reports whether the request carries an unexpired, correctly signed cookie.
func (a *Auth) Valid(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	p := strings.Split(c.Value, ".")
	if len(p) != 3 || p[0] != "v1" {
		return false
	}
	exp, err := strconv.ParseInt(p[1], 10, 64)
	if err != nil || exp < time.Now().Unix() {
		return false
	}
	return hmac.Equal([]byte(p[2]), []byte(a.sign(exp)))
}

func (a *Auth) SetCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: a.cookieValue(), Path: "/",
		MaxAge: a.days * 86400, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func (a *Auth) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func (a *Auth) CheckPassword(pass string) bool {
	return bcrypt.CompareHashAndPassword([]byte(a.hash), []byte(pass)) == nil
}

// Allow implements a small sliding-window rate limit: 5 login attempts per
// minute per IP. Memory is bounded by pruning empty windows.
func (a *Auth) Allow(remoteAddr string) bool {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	a.mu.Lock()
	defer a.mu.Unlock()
	kept := a.attempts[ip][:0]
	for _, t := range a.attempts[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= 5 {
		a.attempts[ip] = kept
		return false
	}
	a.attempts[ip] = append(kept, now)
	for k, v := range a.attempts {
		if len(v) == 0 || !v[len(v)-1].After(cutoff) {
			delete(a.attempts, k)
		}
	}
	return true
}
