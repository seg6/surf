// Surf is a backend and tray supervisor. Tray mode relaunches the same
// executable in serve mode to retain process isolation.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
	"surf-backend/internal/backendapp"
	"surf-backend/internal/config"
	"surf-backend/internal/runenv"
	"surf-backend/internal/updater"
)

type desktopConfig struct {
	Password string `json:"password"`
	Port     int    `json:"port"`
}

type desktopApp struct {
	mu sync.Mutex

	home        string
	manageChild bool
	baseURL     string
	password    string
	logPath     string
	logFile     *os.File
	manageURL   string
	manageHTTP  *http.Server
	authCookie  *http.Cookie

	cmd  *exec.Cmd
	done chan struct{}

	statusItem *systray.MenuItem

	updateRelease *updater.Release
	updateState   string
	updateError   string
}

func main() {
	prepareConsole(len(os.Args) > 1 && os.Args[1] != "update-helper")
	if len(os.Args) >= 2 && os.Args[1] == "update-helper" {
		if err := runUpdateHelper(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "surf update:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "Usage: surf [serve|doctor|update|version]")
		os.Exit(2)
	}
	if len(os.Args) == 1 {
		if err := runTray(); err != nil {
			fmt.Fprintln(os.Stderr, "surf:", err)
			os.Exit(1)
		}
		return
	}
	command := os.Args[1]
	var err error
	switch command {
	case "serve":
		err = backendapp.Serve()
	case "doctor":
		err = doctor()
	case "update":
		err = runCommandUpdate()
	case "version":
		fmt.Printf("surf %s\nprotocol %s\n", config.AppVersion, config.NativeVersion)
		return
	default:
		fmt.Fprintln(os.Stderr, "Usage: surf [serve|doctor|update|version]")
		err = fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "surf:", err)
		os.Exit(1)
	}
}

func updateClient() updater.Client {
	return updater.Client{ManifestURL: os.Getenv("SURF_UPDATE_MANIFEST")}
}

func checkUpdate(ctx context.Context) (updater.Release, error) {
	return updateClient().Check(ctx, config.AppVersion)
}

func stageUpdate(ctx context.Context, release updater.Release, home string) (string, error) {
	directory := filepath.Join(home, "updates", release.Manifest.Version)
	return updateClient().DownloadExecutable(ctx, release, directory)
}

func runCommandUpdate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	release, err := checkUpdate(ctx)
	if err != nil {
		return err
	}
	if !release.Newer {
		fmt.Printf("Surf %s is current.\n", config.AppVersion)
		return nil
	}
	home, err := surfHome()
	if err != nil {
		return err
	}
	staged, err := stageUpdate(ctx, release, home)
	if err != nil {
		return err
	}
	target, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := launchUpdateHelper(staged, target, false); err != nil {
			return err
		}
		fmt.Printf("Surf %s will be installed after this process exits.\n", release.Manifest.Version)
		return nil
	}
	if err := updater.ReplaceExecutable(staged, target); err != nil {
		return err
	}
	fmt.Printf("Updated Surf to %s.\n", release.Manifest.Version)
	return nil
}

func runUpdateHelper(args []string) error {
	hideConsole()
	if len(args) != 3 {
		return errors.New("invalid update helper invocation")
	}
	staged, target := args[0], args[1]
	restart := args[2] == "restart"
	deadline := time.Now().Add(45 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		err = updater.ReplaceExecutableCopy(staged, target)
		if err == nil {
			if restart {
				return exec.Command(target).Start()
			}
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("replace executable: %w", err)
}

func runTray() error {
	hideConsole()
	app, err := newDesktopApp()
	if err != nil {
		return err
	}
	systray.Run(app.onReady, app.onExit)
	return nil
}

func doctor() error {
	cfg, err := config.LoadForDoctor()
	if err != nil {
		return err
	}
	if err := backendapp.EnsureRuntime(cfg); err != nil {
		return err
	}
	failed := false
	for _, check := range runenv.Doctor(cfg) {
		if check.OK {
			log.Printf("doctor: ok %s=%s", check.Name, check.Path)
		} else if check.Required {
			failed = true
			log.Printf("doctor: missing %s=%s: %v", check.Name, check.Path, check.Err)
		} else {
			log.Printf("doctor: optional missing %s=%s: %v", check.Name, check.Path, check.Err)
		}
	}
	if failed {
		return fmt.Errorf("doctor failed")
	}
	return nil
}

func newDesktopApp() (*desktopApp, error) {
	home, err := surfHome()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	cfg, err := loadDesktopConfig(home)
	if err != nil {
		return nil, err
	}
	if envPassword := os.Getenv("SURF_PASSWORD"); envPassword != "" {
		cfg.Password = envPassword
	}
	if cfg.Password == "" {
		cfg.Password, err = randomPassword()
		if err != nil {
			return nil, err
		}
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = 18080
	}
	if err := saveDesktopConfig(home, cfg); err != nil {
		return nil, err
	}
	logPath := filepath.Join(home, "desktop.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	app := &desktopApp{
		home: home, manageChild: true,
		baseURL:  "http://127.0.0.1:" + strconv.Itoa(cfg.Port),
		password: cfg.Password, logPath: logPath, logFile: logFile,
	}
	if err := app.startManagementServer(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	fmt.Fprintln(logFile, "surf: management UI", app.manageURL)
	return app, nil
}

func (a *desktopApp) onReady() {
	icon := trayIcon()
	systray.SetIcon(icon)
	systray.SetTitle("Surf")
	systray.SetTooltip("Surf remote browser backend")

	a.statusItem = systray.AddMenuItem("Starting Surf…", "Backend status")
	a.statusItem.Disable()
	settings := systray.AddMenuItem("Settings…", "Open Surf settings and status")
	restart := systray.AddMenuItem("Restart Backend", "Restart the managed backend process")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit Surf", "Stop the backend and quit")

	go a.startBackend()
	go a.monitorHealth()
	go menuLoop(settings, func() { a.openManagement("") })
	go menuLoop(restart, func() {
		a.setStatus("Restarting Surf…")
		a.stopBackend()
		_ = a.startBackend()
	})
	go menuLoop(quit, systray.Quit)
}

func menuLoop(item *systray.MenuItem, action func()) {
	for range item.ClickedCh {
		action()
	}
}

func (a *desktopApp) onExit() {
	a.stopBackend()
	if a.manageHTTP != nil {
		_ = a.manageHTTP.Close()
	}
	if a.logFile != nil {
		_ = a.logFile.Close()
	}
}

func (a *desktopApp) startBackend() error {
	a.mu.Lock()
	if a.cmd != nil {
		a.mu.Unlock()
		return nil
	}
	password, baseURL := a.password, a.baseURL
	self, err := os.Executable()
	if err != nil {
		a.mu.Unlock()
		return err
	}
	cmd := exec.Command(self, "serve")
	cmd.Env = append(filteredEnv(os.Environ(), "SURF_HOME", "SURF_PASSWORD", "BIND_ADDR", "PORT"),
		"SURF_HOME="+a.home,
		"SURF_PASSWORD="+password,
		"BIND_ADDR=0.0.0.0",
		"PORT="+strings.TrimPrefix(baseURL, "http://127.0.0.1:"),
	)
	cmd.Stdout, cmd.Stderr = a.logFile, a.logFile
	done := make(chan struct{})
	if err := cmd.Start(); err != nil {
		a.mu.Unlock()
		a.setStatus("Surf failed to start")
		return err
	}
	a.cmd, a.done = cmd, done
	a.mu.Unlock()
	a.setStatus("Surf is starting…")
	go func() {
		err := cmd.Wait()
		a.mu.Lock()
		if a.cmd == cmd {
			a.cmd = nil
			a.done = nil
		}
		a.mu.Unlock()
		if err != nil {
			fmt.Fprintln(a.logFile, "surf: backend exited:", err)
		}
		close(done)
	}()
	return nil
}

func (a *desktopApp) stopBackend() {
	a.mu.Lock()
	cmd, done := a.cmd, a.done
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func (a *desktopApp) monitorHealth() {
	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		baseURL := a.baseURL
		a.mu.Unlock()
		response, err := client.Get(baseURL + "/health")
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			a.setStatus("Surf is running")
		} else {
			a.mu.Lock()
			owned := a.cmd != nil
			a.mu.Unlock()
			if owned {
				a.setStatus("Surf is starting…")
			} else {
				a.setStatus("Surf is stopped")
			}
		}
		<-ticker.C
	}
}

func (a *desktopApp) setStatus(status string) {
	if a.statusItem != nil {
		a.statusItem.SetTitle(status)
	}
}

func (a *desktopApp) currentPassword() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.password
}

func filteredEnv(env []string, names ...string) []string {
	blocked := make(map[string]bool, len(names))
	for _, name := range names {
		blocked[name] = true
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			out = append(out, entry)
		}
	}
	return out
}

func (a *desktopApp) startManagementServer() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("desktop management listener: %w", err)
	}
	a.manageURL = "http://" + listener.Addr().String()
	a.manageHTTP = &http.Server{
		Handler:           a.managementHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := a.manageHTTP.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(a.logFile, "surf: management server:", err)
		}
	}()
	return nil
}

func (a *desktopApp) openManagement(path string) {
	if err := openExternal(a.manageURL + path); err != nil {
		a.setStatus("Could not open browser")
		fmt.Fprintln(a.logFile, "surf: open browser:", err)
	}
}

func (a *desktopApp) managementHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.managementHome)
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(surfIconPNG)
	})
	mux.HandleFunc("/api/config", a.managementConfig)
	mux.HandleFunc("/api/restart", a.managementRestart)
	mux.HandleFunc("/api/update/check", a.managementUpdateCheck)
	mux.HandleFunc("/api/update/apply", a.managementUpdateApply)
	mux.HandleFunc("/api/update/status", a.managementUpdateStatus)
	mux.HandleFunc("/settings", a.managementSettings)
	mux.HandleFunc("/logs", a.managementLogs)
	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			a.mu.Lock()
			baseURL := a.baseURL
			a.mu.Unlock()
			target, err := url.Parse(baseURL)
			if err != nil {
				return
			}
			request.URL.Scheme = target.Scheme
			request.URL.Host = target.Host
			if request.URL.Path == "/api/status" {
				request.URL.Path = "/health"
				request.URL.RawQuery = "stats=1"
			}
			request.Host = target.Host
		},
		Transport: &desktopTransport{app: a, base: http.DefaultTransport},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, "Surf backend is not ready: "+err.Error(), http.StatusBadGateway)
		},
	}
	mux.Handle("/api/status", proxy)
	return localOnly(mux)
}

func (a *desktopApp) managementUpdateStatus(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	state, message := a.updateState, a.updateError
	version := ""
	if a.updateRelease != nil {
		version = a.updateRelease.Manifest.Version
	}
	a.mu.Unlock()
	if state == "" {
		state = "idle"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"state": state, "version": version, "message": message, "current": config.AppVersion,
	})
}

func (a *desktopApp) managementUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if !validDesktopMutation(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	a.mu.Lock()
	if a.updateState == "checking" || a.updateState == "downloading" || a.updateState == "applying" {
		a.mu.Unlock()
		http.Error(w, "update already in progress", http.StatusConflict)
		return
	}
	a.updateState, a.updateError = "checking", ""
	a.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		release, err := checkUpdate(ctx)
		a.mu.Lock()
		defer a.mu.Unlock()
		if err != nil {
			a.updateState, a.updateError = "error", err.Error()
			return
		}
		a.updateRelease = &release
		if release.Newer {
			a.updateState = "available"
		} else {
			a.updateState = "current"
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

func (a *desktopApp) managementUpdateApply(w http.ResponseWriter, r *http.Request) {
	if !validDesktopMutation(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	a.mu.Lock()
	if a.updateState != "available" || a.updateRelease == nil {
		a.mu.Unlock()
		http.Error(w, "no update is available", http.StatusConflict)
		return
	}
	release := *a.updateRelease
	a.updateState, a.updateError = "downloading", ""
	a.mu.Unlock()
	go a.applyDesktopUpdate(release)
	w.WriteHeader(http.StatusAccepted)
}

func validDesktopMutation(r *http.Request) bool {
	return r.Method == http.MethodPost && r.Header.Get("X-Surf-Desktop") == "1"
}

func (a *desktopApp) applyDesktopUpdate(release updater.Release) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	staged, err := stageUpdate(ctx, release, a.home)
	if err != nil {
		a.setUpdateFailure(err)
		return
	}
	target, err := os.Executable()
	if err != nil {
		a.setUpdateFailure(err)
		return
	}
	a.mu.Lock()
	a.updateState = "applying"
	a.mu.Unlock()
	a.stopBackend()
	if runtime.GOOS == "windows" {
		err = launchUpdateHelper(staged, target, true)
	} else {
		err = updater.ReplaceExecutable(staged, target)
		if err == nil {
			err = exec.Command(target).Start()
		}
	}
	if err != nil {
		a.setUpdateFailure(err)
		_ = a.startBackend()
		return
	}
	systray.Quit()
}

func (a *desktopApp) setUpdateFailure(err error) {
	a.mu.Lock()
	a.updateState, a.updateError = "error", err.Error()
	a.mu.Unlock()
}

func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			http.Error(w, "local access only", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

//go:embed management.html
var managementHTML []byte

func (a *desktopApp) managementHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(managementHTML)
}

func (a *desktopApp) managementConfig(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	password, baseURL := a.password, a.baseURL
	running := a.cmd != nil
	a.mu.Unlock()
	port, _ := strconv.Atoi(strings.TrimPrefix(baseURL, "http://127.0.0.1:"))
	lanURLs := localLANURLs(port)
	lanURL := ""
	if len(lanURLs) != 0 {
		lanURL = lanURLs[0]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"password": password, "baseURL": baseURL, "lanURL": lanURL, "lanURLs": lanURLs,
		"port": port, "running": running,
	})
}

func localLANURLs(port int) []string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	urls := make([]string, 0, len(addresses))
	preferred := preferredLANIP()
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			value := "http://" + net.JoinHostPort(ip4.String(), strconv.Itoa(port))
			if preferred != nil && ip4.Equal(preferred) {
				urls = append([]string{value}, urls...)
			} else {
				urls = append(urls, value)
			}
		}
	}
	if len(urls) > 1 {
		slices.Sort(urls[1:])
	}
	return slices.Compact(urls)
}

func preferredLANIP() net.IP {
	connection, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return nil
	}
	defer connection.Close()
	address, _ := connection.LocalAddr().(*net.UDPAddr)
	if address == nil {
		return nil
	}
	return address.IP.To4()
}

func (a *desktopApp) managementRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("X-Surf-Desktop") != "1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	go func() {
		a.setStatus("Restarting Surf…")
		a.stopBackend()
		if err := a.startBackend(); err != nil {
			fmt.Fprintln(a.logFile, "surf: restart:", err)
		}
	}()
}

func (a *desktopApp) managementSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("X-Surf-Desktop") != "1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseMultipartForm(64 << 10); err != nil {
		http.Error(w, "invalid settings", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	port, err := strconv.Atoi(r.FormValue("port"))
	if len(password) < 8 {
		http.Error(w, "Password must contain at least 8 characters.", http.StatusBadRequest)
		return
	}
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "Port must be between 1 and 65535.", http.StatusBadRequest)
		return
	}
	cfg := desktopConfig{Password: password, Port: port}
	if err := saveDesktopConfig(a.home, cfg); err != nil {
		http.Error(w, "Could not save settings.", http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.password = password
	a.baseURL = "http://127.0.0.1:" + strconv.Itoa(port)
	a.authCookie = nil
	a.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
	if !a.manageChild { // unit tests exercise settings without a child process
		return
	}
	go func() {
		a.setStatus("Restarting Surf…")
		a.stopBackend()
		if err := a.startBackend(); err != nil {
			fmt.Fprintln(a.logFile, "surf: restart after settings:", err)
		}
	}()
}

func (a *desktopApp) managementLogs(w http.ResponseWriter, _ *http.Request) {
	data, err := os.ReadFile(a.logPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(data) > 1<<20 {
		data = data[len(data)-(1<<20):]
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

type desktopTransport struct {
	app  *desktopApp
	base http.RoundTripper
}

func (t *desktopTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cookie, err := t.app.backendAuthCookie()
	if err != nil {
		return nil, err
	}
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.AddCookie(cookie)
	response, err := t.base.RoundTrip(clone)
	if err == nil && response.StatusCode == http.StatusUnauthorized {
		t.app.mu.Lock()
		t.app.authCookie = nil
		t.app.mu.Unlock()
	}
	return response, err
}

func (a *desktopApp) backendAuthCookie() (*http.Cookie, error) {
	a.mu.Lock()
	if a.authCookie != nil {
		cookie := *a.authCookie
		a.mu.Unlock()
		return &cookie, nil
	}
	password, baseURL := a.password, a.baseURL
	a.mu.Unlock()

	form := url.Values{"password": {password}}
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("backend login returned HTTP %d", response.StatusCode)
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "surf_auth" {
			a.mu.Lock()
			a.authCookie = cookie
			a.mu.Unlock()
			copy := *cookie
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("backend login did not return an auth cookie")
}

func surfHome() (string, error) {
	if home := os.Getenv("SURF_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".surf"), nil
}

func loadDesktopConfig(home string) (desktopConfig, error) {
	var cfg desktopConfig
	data, err := os.ReadFile(filepath.Join(home, "desktop.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("read desktop config: %w", err)
	}
	return cfg, nil
}

func saveDesktopConfig(home string, cfg desktopConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(home, "desktop.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	// Windows does not let Rename replace an existing file. The config is
	// deliberately tiny, so remove the old copy before installing the complete
	// temporary file on every platform.
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func randomPassword() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func openExternal(target string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

//go:embed surf-icon.png
var surfIconPNG []byte

//go:embed tray-icon.png
var trayIconPNG []byte

func trayIcon() []byte {
	if runtime.GOOS != "windows" {
		return trayIconPNG
	}
	// Windows requires an ICO container and accepts embedded PNG image entries.
	var icon bytes.Buffer
	_ = binary.Write(&icon, binary.LittleEndian, []uint16{0, 1, 1})
	icon.Write([]byte{64, 64, 0, 0})
	_ = binary.Write(&icon, binary.LittleEndian, uint16(1))
	_ = binary.Write(&icon, binary.LittleEndian, uint16(32))
	_ = binary.Write(&icon, binary.LittleEndian, uint32(len(trayIconPNG)))
	_ = binary.Write(&icon, binary.LittleEndian, uint32(22))
	icon.Write(trayIconPNG)
	return icon.Bytes()
}
