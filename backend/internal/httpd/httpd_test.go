package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"surf-backend/internal/auth"
	"surf-backend/internal/config"
)

func TestHealthStatsRequiresAuth(t *testing.T) {
	a := auth.New(t.TempDir(), "unused", 1)
	srv, err := New(&config.Config{}, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetStats(func() map[string]any { return map[string]any{"activeURL": "https://example.com"} })

	public := httptest.NewRecorder()
	srv.Handler().ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/health", nil))
	if public.Code != http.StatusOK {
		t.Fatalf("public health code = %d, want %d", public.Code, http.StatusOK)
	}

	unauth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/health?stats=1", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth stats code = %d, want %d", unauth.Code, http.StatusUnauthorized)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/health?stats=1", nil)
	w := httptest.NewRecorder()
	a.SetCookie(w)
	authReq.AddCookie(w.Result().Cookies()[0])
	authResp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authResp, authReq)
	if authResp.Code != http.StatusOK {
		t.Fatalf("auth stats code = %d, want %d", authResp.Code, http.StatusOK)
	}
}

func TestNativeConfigReturnsWSTicket(t *testing.T) {
	a := auth.New(t.TempDir(), "unused", 1)
	srv, err := New(&config.Config{ViewW: 1024, ViewH: 768}, a, nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/native-config", nil)
	cookieResp := httptest.NewRecorder()
	a.SetCookie(cookieResp)
	req.AddCookie(cookieResp.Result().Cookies()[0])
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("native-config code = %d, want %d", resp.Code, http.StatusOK)
	}
	var body struct {
		Ticket string `json:"ticket"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(resp.Result().Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Token != "" {
		t.Fatal("native-config returned legacy token")
	}
	if !a.VerifyWSTicket(body.Ticket) {
		t.Fatal("native-config returned invalid ws ticket")
	}
}
