// surf-desktop is a small tray supervisor for the standalone surf-backend
// executable. Keeping the server in its own process preserves the same CLI,
// service, crash containment, and diagnostics behavior used on headless hosts.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
)

type desktopConfig struct {
	Password string `json:"password"`
	Port     int    `json:"port"`
}

type desktopApp struct {
	mu sync.Mutex

	home        string
	backendPath string
	baseURL     string
	password    string
	logPath     string
	logFile     *os.File

	cmd  *exec.Cmd
	done chan struct{}

	statusItem *systray.MenuItem
}

func main() {
	app, err := newDesktopApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "surf-desktop:", err)
		os.Exit(1)
	}
	systray.Run(app.onReady, app.onExit)
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
	backendPath, err := locateBackend()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(home, "desktop.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &desktopApp{
		home: home, backendPath: backendPath,
		baseURL:  "http://127.0.0.1:" + strconv.Itoa(cfg.Port),
		password: cfg.Password, logPath: logPath, logFile: logFile,
	}, nil
}

func (a *desktopApp) onReady() {
	icon := trayIcon()
	systray.SetIcon(icon)
	systray.SetTemplateIcon(icon, icon)
	systray.SetTitle("Surf")
	systray.SetTooltip("Surf remote browser backend")

	a.statusItem = systray.AddMenuItem("Starting Surf…", "Backend status")
	a.statusItem.Disable()
	openSurf := systray.AddMenuItem("Open Surf", "Open the backend in your browser")
	openDiagnostics := systray.AddMenuItem("Open Diagnostics", "Open performance diagnostics")
	password := systray.AddMenuItem("Password: "+a.password, "Password used by the iPad")
	password.Disable()
	copyPassword := systray.AddMenuItem("Copy Password", "Copy the generated Surf password")
	openLogs := systray.AddMenuItem("Open Logs", "Open Surf's desktop log")
	restart := systray.AddMenuItem("Restart Backend", "Restart the managed backend process")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit Surf", "Stop the backend and quit")

	go a.startBackend()
	go a.monitorHealth()
	go menuLoop(openSurf, func() { _ = openExternal(a.baseURL) })
	go menuLoop(openDiagnostics, func() { _ = openExternal(a.baseURL + "/diagnostics/") })
	go menuLoop(copyPassword, func() {
		if err := copyText(a.password); err != nil {
			a.setStatus("Could not copy password")
			return
		}
		a.setStatus("Password copied")
	})
	go menuLoop(openLogs, func() { _ = openFile(a.logPath) })
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
	cmd := exec.Command(a.backendPath, "serve")
	cmd.Env = append(os.Environ(),
		"SURF_HOME="+a.home,
		"SURF_PASSWORD="+a.password,
		"BIND_ADDR=0.0.0.0",
		"PORT="+strings.TrimPrefix(a.baseURL, "http://127.0.0.1:"),
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
			fmt.Fprintln(a.logFile, "surf-desktop: backend exited:", err)
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
		response, err := client.Get(a.baseURL + "/health")
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

func locateBackend() (string, error) {
	if path := os.Getenv("SURF_BACKEND"); path != "" {
		return path, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "surf-backend"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(filepath.Dir(self), name)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, nil
	}
	return "", fmt.Errorf("surf-backend was not found beside %s; set SURF_BACKEND", self)
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

func openFile(path string) error {
	target := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	return openExternal(target)
}

func copyText(value string) error {
	var commands [][]string
	switch runtime.GOOS {
	case "windows":
		commands = [][]string{{"cmd", "/c", "clip"}}
	case "darwin":
		commands = [][]string{{"pbcopy"}}
	default:
		commands = [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		}
	}
	var lastErr error
	for _, command := range commands {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdin = strings.NewReader(value)
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func trayIcon() []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	blue := color.RGBA{R: 25, G: 116, B: 210, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := x-size/2, y-size/2
			if dx*dx+dy*dy <= 15*15 {
				img.Set(x, y, blue)
			}
		}
	}
	for y := 8; y < 13; y++ {
		for x := 8; x < 24; x++ {
			img.Set(x, y, white)
		}
	}
	for y := 13; y < 19; y++ {
		for x := 8; x < 13; x++ {
			img.Set(x, y, white)
		}
	}
	for y := 18; y < 23; y++ {
		for x := 8; x < 24; x++ {
			img.Set(x, y, white)
		}
	}
	var pngData bytes.Buffer
	_ = png.Encode(&pngData, img)
	var icon bytes.Buffer
	_ = binary.Write(&icon, binary.LittleEndian, []uint16{0, 1, 1})
	icon.Write([]byte{size, size, 0, 0})
	_ = binary.Write(&icon, binary.LittleEndian, uint16(1))
	_ = binary.Write(&icon, binary.LittleEndian, uint16(32))
	_ = binary.Write(&icon, binary.LittleEndian, uint32(pngData.Len()))
	_ = binary.Write(&icon, binary.LittleEndian, uint32(22))
	icon.Write(pngData.Bytes())
	return icon.Bytes()
}
