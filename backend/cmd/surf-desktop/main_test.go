package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopConfigRoundTrip(t *testing.T) {
	home := t.TempDir()
	want := desktopConfig{Password: "test-secret", Port: 19090}
	if err := saveDesktopConfig(home, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadDesktopConfig(home)
	if err != nil || got != want {
		t.Fatalf("config=%+v err=%v", got, err)
	}
	info, err := os.Stat(filepath.Join(home, "desktop.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not expose Unix permission bits through os.FileMode.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("mode=%v", info.Mode())
	}

	want = desktopConfig{Password: "replacement-secret", Port: 18080}
	if err := saveDesktopConfig(home, want); err != nil {
		t.Fatal(err)
	}
	got, err = loadDesktopConfig(home)
	if err != nil || got != want {
		t.Fatalf("replacement config=%+v err=%v", got, err)
	}
}

func TestRandomPassword(t *testing.T) {
	first, err := randomPassword()
	if err != nil || len(first) < 20 {
		t.Fatalf("password length=%d err=%v", len(first), err)
	}
	second, err := randomPassword()
	if err != nil || first == second {
		t.Fatalf("password collision or error: %v", err)
	}
}

func TestTrayIconHasNativeFormat(t *testing.T) {
	icon := trayIcon()
	if len(icon) < 30 {
		t.Fatalf("icon too short: %d", len(icon))
	}
	if runtime.GOOS != "windows" {
		if !bytes.Equal(icon[:4], []byte{0x89, 'P', 'N', 'G'}) {
			t.Fatalf("bad PNG header: %x", icon[:min(8, len(icon))])
		}
		return
	}
	if !bytes.Equal(icon[:4], []byte{0, 0, 1, 0}) {
		t.Fatalf("bad ICO header: %x", icon[:min(8, len(icon))])
	}
}

func TestManagementSettingsPersistAndUpdateRuntime(t *testing.T) {
	home := t.TempDir()
	app := &desktopApp{
		home: home, password: "old-password", baseURL: "http://127.0.0.1:18080",
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("password", "new-password")
	_ = writer.WriteField("port", "19090")
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/settings", &body)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Surf-Desktop", "1")
	response := httptest.NewRecorder()
	app.managementHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("settings code=%d body=%s", response.Code, response.Body.String())
	}
	if app.currentPassword() != "new-password" || app.baseURL != "http://127.0.0.1:19090" {
		t.Fatalf("runtime settings password=%q base=%q", app.currentPassword(), app.baseURL)
	}
	cfg, err := loadDesktopConfig(home)
	if err != nil || cfg.Password != "new-password" || cfg.Port != 19090 {
		t.Fatalf("saved config=%+v err=%v", cfg, err)
	}
}

func TestManagementRejectsNonLoopback(t *testing.T) {
	app := &desktopApp{}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:12345"
	response := httptest.NewRecorder()
	app.managementHandler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("code=%d, want forbidden", response.Code)
	}
}

func TestManagementHomeIsSinglePageUtility(t *testing.T) {
	app := &desktopApp{}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	app.managementHandler().ServeHTTP(response, request)
	body := response.Body.String()
	for _, required := range []string{`id="settings-form"`, `id="lan-address"`, `id="logs"`} {
		if !strings.Contains(body, required) {
			t.Errorf("management page is missing %s", required)
		}
	}
	for _, removed := range []string{`class="sidebar"`, `data-view=`, `/diagnostics/`} {
		if strings.Contains(body, removed) {
			t.Errorf("management page still contains %s", removed)
		}
	}
}

func TestDiagnosticsRouteRemoved(t *testing.T) {
	app := &desktopApp{}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/diagnostics", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	app.managementHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("code=%d, want not found", response.Code)
	}
}
