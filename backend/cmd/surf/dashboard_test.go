package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
)

func serveManagement(t *testing.T, method, target, remote string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, nil)
	request.RemoteAddr = remote
	response := httptest.NewRecorder()
	(&desktopApp{}).managementHandler().ServeHTTP(response, request)
	return response
}

func TestDashboardIndexIsServedAtRoot(t *testing.T) {
	response := serveManagement(t, http.MethodGet, "/", "127.0.0.1:12345")
	if response.Code != http.StatusOK {
		t.Fatalf("code=%d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content type %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("cache control %q, want no-store", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("nosniff header %q", got)
	}
	body := response.Body.String()
	for _, required := range []string{
		`<div id="app">`, `rel="icon" type="image/png" sizes="1024x1024" href="/icon.png"`,
		`rel="apple-touch-icon" href="/icon.png"`, `surf-dashboard-appearance`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("index.html is missing %s", required)
		}
	}
	if strings.Contains(body, "fonts.googleapis.com") || strings.Contains(body, "fonts.gstatic.com") {
		t.Error("the dashboard must not request remote fonts")
	}
}

func TestDashboardAssetsAreServedWithContentTypes(t *testing.T) {
	index := serveManagement(t, http.MethodGet, "/", "127.0.0.1:12345").Body.String()
	script := regexp.MustCompile(`src="(/assets/[^"]+\.js)"`).FindStringSubmatch(index)
	style := regexp.MustCompile(`href="(/assets/[^"]+\.css)"`).FindStringSubmatch(index)
	if script == nil || style == nil {
		t.Fatalf("index.html does not reference a script and a stylesheet:\n%s", index)
	}
	want := map[string]string{
		script[1]: "text/javascript; charset=utf-8",
		style[1]:  "text/css; charset=utf-8",
	}
	fonts, err := fs.Glob(dashboardFS, "assets/*.woff2")
	if err != nil || len(fonts) == 0 {
		t.Fatalf("no bundled fonts: %v", err)
	}
	for _, font := range fonts {
		want["/"+font] = "font/woff2"
	}
	for target, contentType := range want {
		response := serveManagement(t, http.MethodGet, target, "127.0.0.1:12345")
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("%s: code=%d bytes=%d", target, response.Code, response.Body.Len())
			continue
		}
		if got := response.Header().Get("Content-Type"); got != contentType {
			t.Errorf("%s: content type %q, want %q", target, got, contentType)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: cache control %q", target, got)
		}
	}
}

func TestDashboardServesOnlyIndexAndAssets(t *testing.T) {
	for _, target := range []string{
		"/index.html", "/assets/missing.js", "/assets/", "/dashboard/dist/index.html", "/diagnostics",
	} {
		if code := serveManagement(t, http.MethodGet, target, "127.0.0.1:12345").Code; code != http.StatusNotFound {
			t.Errorf("%s: code=%d, want 404", target, code)
		}
	}
	if code := serveManagement(t, http.MethodPost, "/", "127.0.0.1:12345").Code; code != http.StatusMethodNotAllowed {
		t.Errorf("POST /: code=%d, want 405", code)
	}
	if code := serveManagement(t, http.MethodGet, "/", "192.0.2.10:12345").Code; code != http.StatusForbidden {
		t.Errorf("non-loopback /: code=%d, want 403", code)
	}
	if code := serveManagement(t, http.MethodGet, "/assets/app.js", "192.0.2.10:12345").Code; code != http.StatusForbidden {
		t.Errorf("non-loopback asset: code=%d, want 403", code)
	}
}

func TestDashboardContentTypes(t *testing.T) {
	for name, want := range map[string]string{
		"a.js": "text/javascript; charset=utf-8", "a.css": "text/css; charset=utf-8",
		"a.woff2": "font/woff2", "a.html": "text/html; charset=utf-8", "a.unknown-ext": "application/octet-stream",
	} {
		if got := dashboardContentType(path.Join("assets", name)); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}
