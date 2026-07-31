package browser

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/transport"
)

func TestTrustedMediaOrigin(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/watch", true},
		{"http://localhost:18080/test", true},
		{"http://127.0.0.1/test", true},
		{"http://[::1]/test", true},
		{"http://example.com/watch", false},
		{"about:blank", false},
	}
	for _, test := range tests {
		if got := isTrustedMediaOrigin(test.url); got != test.want {
			t.Errorf("isTrustedMediaOrigin(%q)=%v want %v", test.url, got, test.want)
		}
	}
}

func TestWidevineBrowserIntegration(t *testing.T) {
	path := os.Getenv("SURF_TEST_WIDEVINE_BROWSER")
	if path == "" {
		t.Skip("set SURF_TEST_WIDEVINE_BROWSER to a Widevine-capable browser")
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><title>Widevine probe</title>"))
	}))
	defer origin.Close()
	home := t.TempDir()
	controller, err := New(&config.Config{
		SurfHome:       home,
		ChromePath:     path,
		Profile:        filepath.Join(home, "profile"),
		StartURL:       origin.URL,
		ViewW:          800,
		ViewH:          600,
		ChromeGPU:      true,
		StreamBitrateK: 12000,
	}, transport.New())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer controller.Shutdown()
	if err := controller.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state, _ := controller.Stats()["widevine"].(string)
		if state == "available" {
			return
		}
		if state == "unavailable" {
			t.Fatalf("Widevine unavailable: %v", controller.Stats()["widevineDetail"])
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Widevine probe timed out: %v", controller.Stats())
}
