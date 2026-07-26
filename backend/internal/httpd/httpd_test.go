package httpd

import (
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
