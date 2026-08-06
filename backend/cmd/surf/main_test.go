package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"surf-backend/internal/control"
)

func TestDesktopConfigRoundTrip(t *testing.T) {
	home := t.TempDir()
	want := desktopConfig{ServerName: "Living Room Surf", PublicAddress: "surf.example:19090", Port: 19090}
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

	want = desktopConfig{ServerName: "Replacement Surf", Port: 18080}
	if err := saveDesktopConfig(home, want); err != nil {
		t.Fatal(err)
	}
	got, err = loadDesktopConfig(home)
	if err != nil || got != want {
		t.Fatalf("replacement config=%+v err=%v", got, err)
	}
}

func TestWindowsInstallerArgumentsForceTakeoverAndRelaunch(t *testing.T) {
	want := []string{"/S"}
	if got := windowsInstallerArguments(); !reflect.DeepEqual(got, want) {
		t.Fatalf("windows installer arguments = %q, want %q", got, want)
	}
}

func TestInvalidDesktopConfigIsPreservedAndReset(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "desktop.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, backup, err := loadDesktopConfigRecovering(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (desktopConfig{}) {
		t.Fatalf("recovered config=%+v", cfg)
	}
	if backup == "" {
		t.Fatal("invalid settings were not preserved")
	}
	if data, err := os.ReadFile(backup); err != nil || string(data) != "{broken" {
		t.Fatalf("backup=%q err=%v", data, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid settings remain: %v", err)
	}
}

func TestDesktopParentGuardExitsOnEOF(t *testing.T) {
	t.Setenv("SURF_PARENT_GUARD", "1")
	reader, writer := io.Pipe()
	exited := make(chan int, 1)
	watchDesktopParent(reader, func(code int) { exited <- code })
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-exited:
		if code != 0 {
			t.Fatalf("exit code=%d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("daemon did not react to desktop parent EOF")
	}
}

func TestInvalidDaemonDescriptorIsPreservedAndIgnored(t *testing.T) {
	home := t.TempDir()
	path := control.Path(home)
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	killed := false
	app := &desktopApp{
		home: home, killProcess: func(int) { killed = true },
		matchesProcess: func(int, string) bool { return false },
	}
	if err := app.takeControlOfExistingBackend(); err != nil {
		t.Fatal(err)
	}
	if killed {
		t.Fatal("invalid descriptor authorized a process kill")
	}
	backups, err := filepath.Glob(path + ".invalid-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("descriptor backups=%v err=%v", backups, err)
	}
}

func TestDesktopTakesControlOfUnresponsiveVerifiedDaemon(t *testing.T) {
	home := t.TempDir()
	server, descriptor := testDaemonDescriptor(t, home, func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusOK)
	})
	server.Close()
	killedPID := 0
	app := &desktopApp{
		home:        home,
		killProcess: func(pid int) { killedPID = pid },
		matchesProcess: func(pid int, executable string) bool {
			return pid == descriptor.PID && executable != ""
		},
	}
	if err := app.takeControlOfExistingBackend(); err != nil {
		t.Fatal(err)
	}
	if killedPID != descriptor.PID {
		t.Fatalf("killed pid=%d, want %d", killedPID, descriptor.PID)
	}
	if _, err := control.Load(home); !errors.Is(err, control.ErrNotRunning) {
		t.Fatalf("daemon descriptor remains after takeover: %v", err)
	}
}

func TestDesktopRemovesDescriptorForExitedDaemon(t *testing.T) {
	home := t.TempDir()
	server, descriptor := testDaemonDescriptor(t, home, func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusOK)
	})
	server.Close()
	descriptor.PID = 1<<30 - 1
	if err := control.Write(home, descriptor); err != nil {
		t.Fatal(err)
	}
	app := &desktopApp{home: home, killProcess: func(int) { t.Fatal("stale PID was killed") }}
	if err := app.takeControlOfExistingBackend(); err != nil {
		t.Fatal(err)
	}
	if _, err := control.Load(home); !errors.Is(err, control.ErrNotRunning) {
		t.Fatalf("stale descriptor remains: %v", err)
	}
}

func TestBackendRestartDelayUsesBoundedBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{-1, time.Second},
		{0, time.Second},
		{1, 2 * time.Second},
		{2, 5 * time.Second},
		{3, 10 * time.Second},
		{4, 30 * time.Second},
		{20, 30 * time.Second},
	}
	for _, test := range tests {
		if got := backendRestartDelay(test.attempt); got != test.want {
			t.Errorf("attempt %d delay=%s, want %s", test.attempt, got, test.want)
		}
	}
}

func TestHealthyExternalDaemonCancelsPendingRestart(t *testing.T) {
	app := &desktopApp{restartAttempt: 3}
	app.scheduleBackendRestart()
	app.mu.Lock()
	if app.restartTimer == nil {
		app.mu.Unlock()
		t.Fatal("restart was not scheduled")
	}
	app.mu.Unlock()
	app.backendHealthy()
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.restartTimer != nil || app.restartAttempt != 0 {
		t.Fatalf("healthy daemon left timer=%v attempt=%d", app.restartTimer != nil, app.restartAttempt)
	}
}

func TestTrayIconHasNativeFormat(t *testing.T) {
	icon := trayIcon()
	if len(icon) < 30 {
		t.Fatalf("icon too short: %d", len(icon))
	}
	if !bytes.Equal(icon[:4], []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("bad PNG header: %x", icon[:min(8, len(icon))])
	}
}

func TestManagementSettingsPersistAndUpdateRuntime(t *testing.T) {
	home := t.TempDir()
	app := &desktopApp{
		home: home, serverName: "Old Surf", baseURL: "https://127.0.0.1:18080",
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("serverName", "New Surf")
	_ = writer.WriteField("publicAddress", "surf.example:19090")
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
	if app.serverName != "New Surf" || app.publicAddress != "surf.example:19090" || app.baseURL != "https://127.0.0.1:19090" {
		t.Fatalf("runtime settings name=%q public=%q base=%q", app.serverName, app.publicAddress, app.baseURL)
	}
	cfg, err := loadDesktopConfig(home)
	if err != nil || cfg.ServerName != "New Surf" || cfg.PublicAddress != "surf.example:19090" || cfg.Port != 19090 {
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
	for _, required := range []string{`id="settings-form"`, `id="lan-address"`, `id="device-list"`, `id="add-device"`, `id="logs"`} {
		if !strings.Contains(body, required) {
			t.Errorf("management page is missing %s", required)
		}
	}
	for _, removed := range []string{`class="sidebar"`, `data-view=`, `/diagnostics/`} {
		if strings.Contains(body, removed) {
			t.Errorf("management page still contains %s", removed)
		}
	}
	if strings.Contains(strings.ToLower(body), "password") {
		t.Error("management page still contains password authentication")
	}
}

func TestManagementPairingUIUsesServerInitiatedInvitation(t *testing.T) {
	app := &desktopApp{}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	app.managementHandler().ServeHTTP(response, request)
	body := response.Body.String()
	for _, required := range []string{
		`>Pair device</button>`,
		`id="pairing-code"`,
		`id="pairing-address"`,
		`id="copy-pairing-address"`,
		`Scan the QR code`,
		`Single use`,
		`/api/v1/admin/pairing/session`,
		`/api/v1/admin/pairing/candidates`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("management pairing UI is missing %q", required)
		}
	}
	lower := strings.ToLower(body)
	for _, removed := range []string{
		`id="copy-pairing-code"`,
		`/admin/pairing/open`,
		`/admin/pairing/close`,
		`/admin/pairing/info`,
		`/admin/pairing/approve`,
		`pairing window`,
		`accepts pairing requests whenever`,
	} {
		if strings.Contains(lower, removed) {
			t.Errorf("management pairing UI still contains removed flow %q", removed)
		}
	}
}

func TestPairCommandAddressPrefersConfiguredPublicAddress(t *testing.T) {
	t.Setenv("SURF_PUBLIC_ADDRESS", "surf.example:18080")
	if got := pairCommandAddress(8080); got != "surf.example:18080" {
		t.Fatalf("pairCommandAddress() = %q", got)
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

func TestManagementAdminProxyRejectsCrossSiteMutation(t *testing.T) {
	app := &desktopApp{}
	request := httptest.NewRequest(http.MethodPost, "/api/backend/api/v1/admin/pairing/reject/request-id", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()
	app.managementHandler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("admin mutation without desktop header = %d", response.Code)
	}
}

func testDaemonDescriptor(t *testing.T, home string, handler func(http.ResponseWriter, *http.Request, string)) (*httptest.Server, control.Descriptor) {
	t.Helper()
	wantToken := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, wantToken)
	}))
	sum := sha256.Sum256(server.Certificate().Raw)
	descriptor, err := control.New(server.URL, hex.EncodeToString(sum[:]), "test-protocol", 18080)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	wantToken = descriptor.AdminToken
	if err := control.Write(home, descriptor); err != nil {
		server.Close()
		t.Fatal(err)
	}
	return server, descriptor
}

func TestLocalAdminDiscoversDaemonWithoutCreatingIdentity(t *testing.T) {
	home := t.TempDir()
	server, descriptor := testDaemonDescriptor(t, home, func(w http.ResponseWriter, r *http.Request, token string) {
		if r.Header.Get(control.AdminHeader) != token {
			t.Errorf("admin token=%q", r.Header.Get(control.AdminHeader))
		}
		if r.URL.Path != "/api/v1/admin/devices" {
			t.Errorf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"devices": []any{}})
	})
	defer server.Close()
	t.Setenv("SURF_HOME", home)
	admin, err := newLocalAdmin()
	if err != nil {
		t.Fatal(err)
	}
	if admin.base != descriptor.ControlURL {
		t.Fatalf("base=%q", admin.base)
	}
	var response struct {
		Devices []any `json:"devices"`
	}
	if err := admin.request(http.MethodGet, "/api/v1/admin/devices", nil, &response); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "identity")); !os.IsNotExist(err) {
		t.Fatalf("CLI created identity state: %v", err)
	}
}

func TestLocalAdminRejectsChangedDaemonIdentity(t *testing.T) {
	home := t.TempDir()
	server, descriptor := testDaemonDescriptor(t, home, func(w http.ResponseWriter, request *http.Request, token string) {
		if request.URL.Path != "/api/v1/admin/devices" {
			t.Errorf("takeover probe path=%q", request.URL.Path)
		}
		if request.Header.Get(control.AdminHeader) != token {
			t.Errorf("takeover admin token=%q", request.Header.Get(control.AdminHeader))
		}
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	descriptor.ServerID = strings.Repeat("0", 64)
	if err := control.Write(home, descriptor); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SURF_HOME", home)
	admin, err := newLocalAdmin()
	if err != nil {
		t.Fatal(err)
	}
	err = admin.request(http.MethodGet, "/api/v1/health", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("identity error=%v", err)
	}
}

func TestDesktopTransportUsesRuntimeControlEndpointAndToken(t *testing.T) {
	home := t.TempDir()
	server, _ := testDaemonDescriptor(t, home, func(w http.ResponseWriter, r *http.Request, token string) {
		if r.Header.Get(control.AdminHeader) != token {
			t.Errorf("admin token=%q", r.Header.Get(control.AdminHeader))
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()
	app := &desktopApp{home: home}
	client, err := app.backendHTTPClient(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get("https://127.0.0.1:1/api/v1/admin/devices")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d", response.StatusCode)
	}
}

func TestDesktopTakesControlOfAuthenticatedExistingDaemon(t *testing.T) {
	home := t.TempDir()
	server, descriptor := testDaemonDescriptor(t, home, func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	killedPID := 0
	app := &desktopApp{
		home: home,
		killProcess: func(pid int) {
			killedPID = pid
			server.CloseClientConnections()
			server.Close()
		},
	}
	if err := app.takeControlOfExistingBackend(); err != nil {
		t.Fatal(err)
	}
	if killedPID != descriptor.PID {
		t.Fatalf("killed pid=%d, want %d", killedPID, descriptor.PID)
	}
	if _, err := control.Load(home); !errors.Is(err, control.ErrNotRunning) {
		t.Fatalf("daemon descriptor remains after takeover: %v", err)
	}
}

func TestDesktopDoesNotKillUnverifiedDaemon(t *testing.T) {
	home := t.TempDir()
	server, descriptor := testDaemonDescriptor(t, home, func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	descriptor.ServerID = strings.Repeat("0", 64)
	if err := control.Write(home, descriptor); err != nil {
		t.Fatal(err)
	}
	killed := false
	app := &desktopApp{home: home, killProcess: func(int) { killed = true }}
	if err := app.takeControlOfExistingBackend(); err == nil {
		t.Fatal("takeover accepted a changed daemon identity")
	}
	if killed {
		t.Fatal("takeover killed an unverified process")
	}
}

func TestTerminalTextRemovesControlCharacters(t *testing.T) {
	if got := terminalText("iPad\n\x1b[31m\tmini"); got != "iPad [31m mini" {
		t.Fatalf("terminal text=%q", got)
	}
}

func TestApplyHomeFlag(t *testing.T) {
	t.Setenv("SURF_HOME", "old")
	args, err := applyHomeFlag([]string{"--home", "custom-home", "status"})
	if err != nil || len(args) != 1 || args[0] != "status" || os.Getenv("SURF_HOME") != "custom-home" {
		t.Fatalf("args=%v home=%q err=%v", args, os.Getenv("SURF_HOME"), err)
	}
	if _, err := applyHomeFlag([]string{"--home"}); err == nil {
		t.Fatal("missing --home path was accepted")
	}
}
