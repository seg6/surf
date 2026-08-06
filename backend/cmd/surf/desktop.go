package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	_ "embed"
	"encoding/hex"
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

	"surf-backend/internal/app"
	"surf-backend/internal/atomicfile"
	"surf-backend/internal/config"
	"surf-backend/internal/control"
	"surf-backend/internal/process"
	"surf-backend/internal/statefile"
	"surf-backend/internal/tray"
	"surf-backend/internal/updater"
	"surf-backend/internal/web"
)

type desktopConfig struct {
	ServerName        string `json:"serverName"`
	PublicAddress     string `json:"publicAddress,omitempty"`
	Port              int    `json:"port"`
	StartAtLogin      bool   `json:"startAtLogin"`
	StartupChoiceMade bool   `json:"startupChoiceMade"`
}

type desktopApp struct {
	mu sync.Mutex

	home          string
	manageChild   bool
	baseURL       string
	serverName    string
	publicAddress string
	logPath       string
	logFile       *os.File
	manageURL     string
	manageHTTP    *http.Server

	cmd         *process.Started
	done        chan struct{}
	parentGuard io.WriteCloser

	closing        bool
	restartTimer   *time.Timer
	restartAttempt int
	killProcess    func(int)
	matchesProcess func(int, string) bool

	tray       *tray.App
	statusItem *tray.Item
	updateItem *tray.Item

	updateRelease *updater.Release
	updateState   string
	updateError   string
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
	if runtime.GOOS == "linux" && os.Getenv("APPIMAGE") != "" && release.Package.URL != "" {
		home, err := surfHome()
		if err != nil {
			return err
		}
		staged := filepath.Join(home, "updates", release.Manifest.Version, release.Package.Name)
		if err := updateClient().DownloadAsset(ctx, release.Package, staged); err != nil {
			return err
		}
		if err := updater.ReplaceExecutable(staged, os.Getenv("APPIMAGE")); err != nil {
			return err
		}
		fmt.Printf("Updated Surf AppImage to %s.\n", release.Manifest.Version)
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
	home, err := surfHome()
	if err != nil {
		return err
	}
	lock, primary, err := acquireDesktopInstance(home)
	if err != nil {
		return err
	}
	if !primary {
		return activateDesktopInstance(home)
	}
	defer lock.Close()
	app, err := newDesktopApp()
	if err != nil {
		return err
	}
	if err := writeDesktopInstance(home, app.manageURL); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(home, "desktop-instance.json"))
	trayApp := tray.New()
	app.tray = trayApp
	app.onReady()
	defer app.onExit()
	return trayApp.Run(trayIcon(), "Surf remote browser backend")
}

func doctor() error {
	cfg, err := config.LoadForDoctor()
	if err != nil {
		return err
	}
	if err := app.Prepare(cfg); err != nil {
		return err
	}
	if _, err := exec.LookPath(cfg.ChromePath); err != nil {
		log.Printf("doctor: missing chromium=%s: %v", cfg.ChromePath, err)
		return fmt.Errorf("doctor failed")
	}
	log.Printf("doctor: ok chromium=%s", cfg.ChromePath)
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
	cfg, recoveredConfig, err := loadDesktopConfigRecovering(home)
	if err != nil {
		return nil, err
	}
	if envName := os.Getenv("SURF_SERVER_NAME"); envName != "" {
		cfg.ServerName = envName
	}
	if cfg.ServerName == "" {
		hostname, _ := os.Hostname()
		cfg.ServerName = "Surf"
		if hostname != "" {
			cfg.ServerName = "Surf on " + hostname
		}
	}
	if envAddress := os.Getenv("SURF_PUBLIC_ADDRESS"); envAddress != "" {
		cfg.PublicAddress = envAddress
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
		baseURL:    "https://127.0.0.1:" + strconv.Itoa(cfg.Port),
		serverName: cfg.ServerName, publicAddress: cfg.PublicAddress,
		logPath: logPath, logFile: logFile,
		killProcess: process.Kill, matchesProcess: process.MatchesExecutable,
	}
	if err := app.startManagementServer(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	fmt.Fprintln(logFile, "surf: management UI", app.manageURL)
	if recoveredConfig != "" {
		fmt.Fprintln(logFile, "surf: recovered invalid desktop settings; backup:", recoveredConfig)
	}
	return app, nil
}

func (a *desktopApp) onReady() {
	a.statusItem = a.tray.AddItem("Starting Surf…", nil)
	a.statusItem.SetDisabled(true)
	a.tray.AddItem("Settings…", func() { a.openManagement("") })
	a.tray.AddItem("Restart Backend", func() {
		a.setStatus("Restarting Surf…")
		a.stopBackend()
		_ = a.startBackend()
	})
	a.updateItem = a.tray.AddItem("Check for Updates…", func() {
		a.startUpdateCheck()
		a.openManagement("")
	})
	a.tray.AddSeparator()
	a.tray.AddItem("Quit Surf", a.tray.Quit)

	go func() {
		if err := a.takeControlOfExistingBackend(); err != nil {
			a.logf("surf: take control: %v\n", err)
		}
		if err := a.startBackend(); err != nil {
			a.logf("surf: start backend: %v\n", err)
		}
	}()
	go a.monitorHealth()
	if cfg, err := loadDesktopConfig(a.home); err == nil && !cfg.StartupChoiceMade {
		go a.openManagement("")
	}
	go a.automaticUpdateChecks()
}

func (a *desktopApp) onExit() {
	a.mu.Lock()
	a.closing = true
	a.cancelBackendRestartLocked()
	a.mu.Unlock()
	a.stopBackend()
	if a.manageHTTP != nil {
		_ = a.manageHTTP.Close()
	}
	if a.logFile != nil {
		_ = a.logFile.Close()
	}
}

func (a *desktopApp) takeControlOfExistingBackend() error {
	descriptor, err := control.Load(a.home)
	if errors.Is(err, control.ErrNotRunning) {
		return nil
	}
	if err != nil {
		backup, backupErr := statefile.Quarantine(control.Path(a.home), "invalid")
		if backupErr != nil {
			return fmt.Errorf("discard invalid daemon descriptor: %w (original error: %v)", backupErr, err)
		}
		a.logf("surf: ignored invalid daemon descriptor; backup: %s (%v)\n", backup, err)
		return nil
	}
	client, err := a.backendHTTPClient(time.Second)
	if err != nil {
		return err
	}
	probeURL := "https://127.0.0.1" + web.APIRoot + "/admin/devices"
	response, err := client.Get(probeURL)
	if err != nil {
		if !process.Running(descriptor.PID) {
			a.logf("surf: removed stale daemon descriptor for exited pid=%d\n", descriptor.PID)
			return control.RemoveOwned(a.home, descriptor.AdminToken)
		}
		self, selfErr := os.Executable()
		if selfErr == nil && a.matchesProcess != nil && a.matchesProcess(descriptor.PID, self) {
			a.logf("surf: taking control from unresponsive backend pid=%d after executable verification (%v)\n", descriptor.PID, err)
			kill := a.killProcess
			if kill == nil {
				kill = process.Kill
			}
			kill(descriptor.PID)
			return control.RemoveOwned(a.home, descriptor.AdminToken)
		}
		return fmt.Errorf("verify existing daemon pid=%d: %w", descriptor.PID, err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("verify existing daemon pid=%d: status %d", descriptor.PID, response.StatusCode)
	}

	a.logf("surf: taking control from existing backend pid=%d\n", descriptor.PID)
	kill := a.killProcess
	if kill == nil {
		kill = process.Kill
	}
	kill(descriptor.PID)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		request, requestErr := client.Get(probeURL)
		if requestErr != nil {
			return control.RemoveOwned(a.home, descriptor.AdminToken)
		}
		_, _ = io.Copy(io.Discard, request.Body)
		_ = request.Body.Close()
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("existing backend pid=%d did not stop", descriptor.PID)
}

func (a *desktopApp) startBackend() error {
	a.mu.Lock()
	if a.closing || a.cmd != nil {
		a.mu.Unlock()
		return nil
	}
	a.cancelBackendRestartLocked()
	baseURL, serverName, publicAddress := a.baseURL, a.serverName, a.publicAddress
	port, _ := strconv.Atoi(strings.TrimPrefix(baseURL, "https://127.0.0.1:"))
	if publicAddress == "" {
		if urls := localLANURLs(port); len(urls) != 0 {
			publicAddress = urls[0]
		}
	}
	self, err := os.Executable()
	if err != nil {
		a.mu.Unlock()
		return err
	}
	env := append(filteredEnv(os.Environ(), "SURF_HOME", "SURF_SERVER_NAME", "SURF_PUBLIC_ADDRESS", "BIND_ADDR", "PORT", "SURF_PARENT_GUARD"),
		"SURF_HOME="+a.home,
		"SURF_SERVER_NAME="+serverName,
		"SURF_PUBLIC_ADDRESS="+publicAddress,
		"BIND_ADDR=0.0.0.0",
		"PORT="+strings.TrimPrefix(baseURL, "https://127.0.0.1:"),
		"SURF_PARENT_GUARD=1",
	)
	done := make(chan struct{})
	cmd, err := process.Start(self, []string{"daemon"}, process.Options{
		Env: env, Stdin: true, StdoutWriter: a.logFile, StderrWriter: a.logFile,
	})
	if err != nil {
		a.mu.Unlock()
		a.setStatus("Surf failed to start")
		a.scheduleBackendRestart()
		return err
	}
	a.cmd, a.done, a.parentGuard = cmd, done, cmd.Stdin
	a.mu.Unlock()
	a.logf("surf: backend started pid=%d\n", cmd.Process.Pid)
	a.setStatus("Surf is starting…")
	go func() {
		err := <-cmd.Done
		a.mu.Lock()
		unexpected := a.cmd == cmd && !a.closing
		if a.cmd == cmd {
			a.cmd = nil
			a.done = nil
			if a.parentGuard != nil {
				_ = a.parentGuard.Close()
				a.parentGuard = nil
			}
		}
		a.mu.Unlock()
		close(done)
		if !unexpected {
			return
		}
		if err != nil {
			a.logf("surf: backend pid=%d exited: %v\n", cmd.Process.Pid, err)
		} else {
			a.logf("surf: backend pid=%d exited unexpectedly\n", cmd.Process.Pid)
		}
		a.setStatus("Surf is restarting…")
		a.scheduleBackendRestart()
	}()
	return nil
}

func (a *desktopApp) stopBackend() {
	a.mu.Lock()
	a.cancelBackendRestartLocked()
	a.restartAttempt = 0
	cmd, done, parentGuard := a.cmd, a.done, a.parentGuard
	if cmd != nil {
		a.cmd = nil
		a.done = nil
		a.parentGuard = nil
	}
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
	if parentGuard != nil {
		_ = parentGuard.Close()
	}
}

func backendRestartDelay(attempt int) time.Duration {
	delays := [...]time.Duration{
		time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(delays) {
		attempt = len(delays) - 1
	}
	return delays[attempt]
}

func (a *desktopApp) cancelBackendRestartLocked() {
	if a.restartTimer != nil {
		a.restartTimer.Stop()
		a.restartTimer = nil
	}
}

func (a *desktopApp) scheduleBackendRestart() {
	a.mu.Lock()
	if a.closing || a.cmd != nil || a.restartTimer != nil {
		a.mu.Unlock()
		return
	}
	delay := backendRestartDelay(a.restartAttempt)
	if a.restartAttempt < 4 {
		a.restartAttempt++
	}
	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		a.mu.Lock()
		if a.closing || a.restartTimer != timer {
			a.mu.Unlock()
			return
		}
		a.restartTimer = nil
		a.mu.Unlock()
		a.setStatus("Surf is restarting…")
		if err := a.startBackend(); err != nil {
			a.logf("surf: restart backend: %v\n", err)
		}
	})
	a.restartTimer = timer
	a.mu.Unlock()
	a.logf("surf: retrying backend in %s\n", delay)
}

func (a *desktopApp) backendHealthy() {
	a.mu.Lock()
	a.restartAttempt = 0
	if a.cmd == nil {
		a.cancelBackendRestartLocked()
	}
	a.mu.Unlock()
}

func (a *desktopApp) logf(format string, args ...any) {
	if a.logFile != nil {
		_, _ = fmt.Fprintf(a.logFile, format, args...)
	}
}

func (a *desktopApp) monitorHealth() {
	client, err := a.backendHTTPClient(time.Second)
	if err != nil {
		fmt.Fprintln(a.logFile, "surf: TLS identity:", err)
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		baseURL := a.baseURL
		a.mu.Unlock()
		response, err := client.Get(baseURL + web.APIRoot + "/health")
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			a.backendHealthy()
			a.setStatus("Surf is running")
		} else {
			a.mu.Lock()
			owned := a.cmd != nil
			a.mu.Unlock()
			if owned {
				a.setStatus("Surf is starting…")
			} else {
				a.setStatus("Surf is restarting…")
				a.scheduleBackendRestart()
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
	mux.HandleFunc("/api/activate", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		go a.openManagement("")
	})
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
				request.URL.Path = web.APIRoot + "/health"
				request.URL.RawQuery = "stats=1"
			} else if strings.HasPrefix(request.URL.Path, "/api/backend/") {
				request.URL.Path = strings.TrimPrefix(request.URL.Path, "/api/backend")
			}
			request.Host = target.Host
		},
		Transport: a.backendTransport(),
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, "Surf backend is not ready: "+err.Error(), http.StatusBadGateway)
		},
	}
	mux.Handle("/api/status", proxy)
	mux.Handle("/api/backend/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !validDesktopMutation(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
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
	if !a.startUpdateCheck() {
		http.Error(w, "update already in progress", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *desktopApp) startUpdateCheck() bool {
	a.mu.Lock()
	if a.updateState == "checking" || a.updateState == "downloading" || a.updateState == "applying" {
		a.mu.Unlock()
		return false
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
			if a.updateItem != nil {
				a.updateItem.SetTitle("Update available: " + release.Manifest.Version + "…")
			}
		} else {
			a.updateState = "current"
			if a.updateItem != nil {
				a.updateItem.SetTitle("Check for Updates…")
			}
		}
	}()
	return true
}

func (a *desktopApp) automaticUpdateChecks() {
	a.startUpdateCheck()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		a.startUpdateCheck()
	}
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
	return (r.Method == http.MethodPost || r.Method == http.MethodDelete) && r.Header.Get("X-Surf-Desktop") == "1"
}

func (a *desktopApp) applyDesktopUpdate(release updater.Release) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if runtime.GOOS == "windows" && release.Package.URL != "" {
		installer := filepath.Join(a.home, "updates", release.Manifest.Version, release.Package.Name)
		if err := updateClient().DownloadAsset(ctx, release.Package, installer); err != nil {
			a.setUpdateFailure(err)
			return
		}
		a.mu.Lock()
		a.updateState = "applying"
		a.mu.Unlock()
		a.stopBackend()
		command := exec.Command(installer, windowsInstallerArguments()...)
		if err := command.Start(); err != nil {
			a.setUpdateFailure(err)
			_ = a.startBackend()
			return
		}
		a.quitTray()
		return
	}
	if runtime.GOOS == "linux" && os.Getenv("APPIMAGE") != "" && release.Package.URL != "" {
		staged := filepath.Join(a.home, "updates", release.Manifest.Version, release.Package.Name)
		if err := updateClient().DownloadAsset(ctx, release.Package, staged); err != nil {
			a.setUpdateFailure(err)
			return
		}
		a.mu.Lock()
		a.updateState = "applying"
		a.mu.Unlock()
		a.stopBackend()
		target := os.Getenv("APPIMAGE")
		if err := updater.ReplaceExecutable(staged, target); err != nil {
			a.setUpdateFailure(err)
			_ = a.startBackend()
			return
		}
		if err := exec.Command(target).Start(); err != nil {
			a.setUpdateFailure(err)
			return
		}
		a.quitTray()
		return
	}
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
	a.quitTray()
}

func (a *desktopApp) quitTray() {
	if a.tray != nil {
		a.tray.Quit()
	}
}

func windowsInstallerArguments() []string {
	return []string{"/S"}
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
	baseURL, serverName, publicAddress := a.baseURL, a.serverName, a.publicAddress
	running := a.cmd != nil
	a.mu.Unlock()
	port, _ := strconv.Atoi(strings.TrimPrefix(baseURL, "https://127.0.0.1:"))
	lanURLs := localLANURLs(port)
	lanURL := ""
	if len(lanURLs) != 0 {
		lanURL = lanURLs[0]
	}
	cfg, _ := loadDesktopConfig(a.home)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"serverName": serverName, "publicAddress": publicAddress, "baseURL": baseURL, "lanURL": lanURL, "lanURLs": lanURLs,
		"port": port, "running": running, "startAtLogin": cfg.StartAtLogin,
		"startupChoiceMade": cfg.StartupChoiceMade,
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
			value := "https://" + net.JoinHostPort(ip4.String(), strconv.Itoa(port))
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
	serverName := strings.TrimSpace(r.FormValue("serverName"))
	publicAddress := strings.TrimSpace(r.FormValue("publicAddress"))
	port, err := strconv.Atoi(r.FormValue("port"))
	if serverName == "" || len(serverName) > 80 {
		http.Error(w, "Server name must contain 1 to 80 characters.", http.StatusBadRequest)
		return
	}
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "Port must be between 1 and 65535.", http.StatusBadRequest)
		return
	}
	startAtLogin := r.FormValue("startAtLogin") == "on"
	cfg := desktopConfig{
		ServerName: serverName, PublicAddress: publicAddress, Port: port,
		StartAtLogin: startAtLogin, StartupChoiceMade: true,
	}
	if a.manageChild {
		if err := setStartAtLogin(startAtLogin); err != nil {
			http.Error(w, "Could not update start-at-login: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := saveDesktopConfig(a.home, cfg); err != nil {
		http.Error(w, "Could not save settings.", http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.serverName = serverName
	a.publicAddress = publicAddress
	a.baseURL = "https://127.0.0.1:" + strconv.Itoa(port)
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

func (a *desktopApp) backendTransport() http.RoundTripper {
	a.mu.Lock()
	home := a.home
	a.mu.Unlock()
	if home == "" {
		return &errorTransport{err: fmt.Errorf("Surf home is not configured")}
	}
	return &daemonControlTransport{home: home}
}

func (a *desktopApp) backendHTTPClient(timeout time.Duration) (*http.Client, error) {
	transport := a.backendTransport()
	if failed, ok := transport.(*errorTransport); ok {
		return nil, failed.err
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

type errorTransport struct{ err error }

func (t *errorTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

type daemonControlTransport struct {
	home      string
	mu        sync.Mutex
	token     string
	target    *url.URL
	transport *http.Transport
}

func (t *daemonControlTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	descriptor, err := control.Load(t.home)
	if err != nil {
		return nil, fmt.Errorf("Surf daemon control: %w", err)
	}
	t.mu.Lock()
	if t.transport == nil || t.token != descriptor.AdminToken {
		target, err := url.Parse(descriptor.ControlURL)
		if err != nil {
			t.mu.Unlock()
			return nil, err
		}
		want := descriptor.ServerID
		transport := &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12, InsecureSkipVerify: true,
			VerifyConnection: func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) == 0 {
					return fmt.Errorf("backend presented no certificate")
				}
				sum := sha256.Sum256(state.PeerCertificates[0].Raw)
				if got := hex.EncodeToString(sum[:]); got != want {
					return fmt.Errorf("backend identity mismatch")
				}
				return nil
			},
		}}
		if t.transport != nil {
			t.transport.CloseIdleConnections()
		}
		t.token, t.target, t.transport = descriptor.AdminToken, target, transport
	}
	token, target, transport := t.token, t.target, t.transport
	t.mu.Unlock()

	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
	clone.Host = target.Host
	clone.Header.Set(control.AdminHeader, token)
	return transport.RoundTrip(clone)
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

func loadDesktopConfigRecovering(home string) (desktopConfig, string, error) {
	cfg, err := loadDesktopConfig(home)
	if err == nil {
		return cfg, "", nil
	}
	path := filepath.Join(home, "desktop.json")
	backup, backupErr := statefile.Quarantine(path, "invalid")
	if backupErr != nil {
		return desktopConfig{}, "", fmt.Errorf("recover desktop config: %w (original error: %v)", backupErr, err)
	}
	return desktopConfig{}, backup, nil
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
	if err := atomicfile.Replace(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
	return trayIconPNG
}
