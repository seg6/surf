package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func newTestAuth(t *testing.T) *Auth {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return New(t.TempDir(), string(hash), 180)
}

func requestWithCookie(v string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: v})
	return r
}

func TestCookieRoundTrip(t *testing.T) {
	a := newTestAuth(t)
	w := httptest.NewRecorder()
	a.SetCookie(w)
	c := w.Result().Cookies()[0]
	if !a.Valid(requestWithCookie(c.Value)) {
		t.Fatal("freshly set cookie should validate")
	}
}

func TestCookieTamperAndExpiry(t *testing.T) {
	a := newTestAuth(t)
	exp := time.Now().Unix() + 3600
	good := fmt.Sprintf("v1.%d.%s", exp, a.sign(exp))
	if !a.Valid(requestWithCookie(good)) {
		t.Fatal("signed cookie should validate")
	}
	// Changing the expiry without re-signing must fail.
	forged := fmt.Sprintf("v1.%d.%s", exp+9999, a.sign(exp))
	if a.Valid(requestWithCookie(forged)) {
		t.Fatal("tampered expiry validated")
	}
	past := time.Now().Unix() - 10
	expired := fmt.Sprintf("v1.%d.%s", past, a.sign(past))
	if a.Valid(requestWithCookie(expired)) {
		t.Fatal("expired cookie validated")
	}
	if a.Valid(requestWithCookie("garbage")) || a.Valid(requestWithCookie("v2.1.2")) {
		t.Fatal("malformed cookie validated")
	}
}

func TestSecretsPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	a1 := New(dir, "x", 1)
	a2 := New(dir, "x", 1)
	if a1.sign(42) != a2.sign(42) {
		t.Fatal("auth secret not persistent")
	}
	if !a2.VerifyWSTicket(a1.WSTicket()) {
		t.Fatal("ws ticket did not validate after restart")
	}
}

func TestWSTicket(t *testing.T) {
	a := newTestAuth(t)
	ticket := a.WSTicket()
	if !a.VerifyWSTicket(ticket) {
		t.Fatal("fresh ws ticket should validate")
	}
	if a.VerifyWSTicket(strings.Replace(ticket, "v1.", "v2.", 1)) {
		t.Fatal("tampered ws ticket version validated")
	}
	past := time.Now().Add(-time.Minute).Unix()
	expired := fmt.Sprintf("v1.%d.%s.%s", past, "nonce", a.signWSTicket(past, "nonce"))
	if a.VerifyWSTicket(expired) {
		t.Fatal("expired ws ticket validated")
	}
}

func TestPassword(t *testing.T) {
	a := newTestAuth(t)
	if !a.CheckPassword("hunter2") {
		t.Fatal("correct password rejected")
	}
	if a.CheckPassword("wrong") {
		t.Fatal("wrong password accepted")
	}
}

func TestRateLimit(t *testing.T) {
	a := newTestAuth(t)
	for i := 0; i < 5; i++ {
		if !a.Allow("1.2.3.4:5678") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if a.Allow("1.2.3.4:9999") {
		t.Fatal("6th attempt from same IP should be limited")
	}
	if !a.Allow("5.6.7.8:1111") {
		t.Fatal("other IP should be unaffected")
	}
}
