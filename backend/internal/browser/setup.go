package browser

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/chromium"
	"surf-backend/internal/config"
	"surf-backend/internal/process"
)

//go:embed setup_extension/*
var setupFiles embed.FS

type setupBrowser struct {
	client    *cdp.Client
	child     *process.Started
	sessionID string
	home      string
	mu        sync.Mutex
	pollMu    sync.Mutex
	closed    bool // guarded by pollMu
	snapshot  browserSession
	windows   int
	ready     bool
}

func startSetup(cfg *config.Config) (result *setupBrowser, resultErr error) {
	if err := chromium.CheckProfileAvailable(cfg.Profile); err != nil {
		return nil, err
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return nil, fmt.Errorf("browser setup needs a desktop session; run Surf from a logged-in desktop")
	}
	path := filepath.Join(cfg.SurfHome, "runtime", "browser-setup-extension")
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	for _, name := range []string{"manifest.json", "background.js"} {
		data, err := setupFiles.ReadFile("setup_extension/" + name)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(path, name), data, 0600); err != nil {
			return nil, err
		}
	}
	extensions := []string{path}
	if cfg.ContentBlockerPath != "" {
		extensions = append(extensions, cfg.ContentBlockerPath)
	}
	client, child, err := cdp.Launch(cdp.LaunchConfig{ChromePath: cfg.ChromePath, Profile: cfg.Profile,
		W: 1100, H: 800, NoSandbox: cfg.ChromeNoSandbox, Visible: true, ContinueSession: true, ExtensionPaths: extensions})
	if err != nil {
		return nil, err
	}
	setup := &setupBrowser{client: client, child: child, home: cfg.SurfHome, snapshot: loadBrowserSession(cfg.SurfHome)}
	ok := false
	defer func() {
		if !ok {
			if closeErr := cdp.CloseBrowser(client, child, true, 5*time.Second); closeErr != nil {
				// Retain ownership: never launch a replacement while this browser
				// might still hold the profile. It has not been offered to the user.
				result, resultErr = setup, fmt.Errorf("%v; cleanup: %w", resultErr, closeErr)
				return
			}
			client.Close()
		}
	}()
	downloadPath := cfg.DownloadsDir
	if downloadPath == "" {
		downloadPath = filepath.Join(cfg.SurfHome, "downloads")
	}
	if err := os.MkdirAll(downloadPath, 0700); err != nil {
		return nil, err
	}
	if _, err := client.Call("", "Browser.setDownloadBehavior", map[string]any{"behavior": "allow", "downloadPath": downloadPath}); err != nil {
		return nil, err
	}
	if err := restoreHandoff(client, setup.snapshot); err != nil {
		return nil, err
	}
	// Attach only to our extension's worker, identified by its installed path.
	raw, err := client.Call("", "Extensions.getExtensions", nil)
	if err != nil {
		return nil, fmt.Errorf("setup session observer: %w", err)
	}
	var extensionsResult struct {
		Extensions []struct{ ID, Path string } `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &extensionsResult); err != nil {
		return nil, err
	}
	extensionID := ""
	for _, e := range extensionsResult.Extensions {
		if filepath.Clean(e.Path) == filepath.Clean(path) {
			extensionID = e.ID
		}
	}
	if extensionID == "" {
		return nil, fmt.Errorf("setup session observer was not loaded")
	}
	deadline := time.Now().Add(5 * time.Second)
	for setup.sessionID == "" && time.Now().Before(deadline) {
		raw, err := client.Call("", "Target.getTargets", nil)
		if err != nil {
			return nil, err
		}
		var targets struct {
			Targets []targetInfo `json:"targetInfos"`
		}
		if err := json.Unmarshal(raw, &targets); err != nil {
			return nil, err
		}
		for _, target := range targets.Targets {
			if target.Type != "service_worker" || !strings.HasPrefix(target.URL, "chrome-extension://"+extensionID+"/") {
				continue
			}
			raw, err = client.Call("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
			if err != nil {
				return nil, err
			}
			var attached struct {
				ID string `json:"sessionId"`
			}
			if err := json.Unmarshal(raw, &attached); err != nil {
				return nil, err
			}
			setup.sessionID = attached.ID
		}
		if setup.sessionID == "" {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if setup.sessionID == "" {
		return nil, fmt.Errorf("setup session observer did not start")
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		if err := setup.poll(); err != nil {
			return nil, err
		}
		if setup.ready {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("setup session observer did not become ready")
		}
		time.Sleep(25 * time.Millisecond)
	}
	ok = true
	return setup, nil
}

func (s *setupBrowser) poll() error {
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	if s.closed {
		return nil
	}
	raw, err := s.client.CallTimeout(s.sessionID, "Runtime.evaluate", map[string]any{
		"expression": "typeof globalThis.surfSetupSnapshot === 'function' ? globalThis.surfSetupSnapshot() : null", "awaitPromise": true, "returnByValue": true}, time.Second)
	if err != nil {
		return err
	}
	var result struct {
		Result struct {
			Value struct {
				Tabs    []string `json:"tabs"`
				Active  int      `json:"active"`
				Windows int      `json:"windows"`
				Ready   bool     `json:"ready"`
			} `json:"value"`
		} `json:"result"`
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if len(result.Exception) > 0 {
		return fmt.Errorf("setup session observer failed")
	}
	value := result.Result.Value
	if !value.Ready {
		return nil
	}
	s.mu.Lock()
	next := filteredSetupSession(value.Tabs, value.Active, s.snapshot)
	s.windows, s.ready = value.Windows, true
	changed := !sameSession(s.snapshot, next)
	s.mu.Unlock()
	if changed {
		if err := writeBrowserSession(s.home, next); err != nil {
			return err
		}
		// Commit only persisted snapshots, so a temporary disk error is retried
		// by the next poll even if no tab has changed in the meantime.
		s.mu.Lock()
		s.snapshot = next
		s.mu.Unlock()
	}
	return nil
}

func (s *setupBrowser) close(force bool, timeout time.Duration) error {
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	if s.closed {
		return nil
	}
	if err := cdp.CloseBrowser(s.client, s.child, force, timeout); err != nil {
		return err
	}
	s.closed = true
	return nil
}

func filteredSetupSession(urls []string, active int, previous browserSession) browserSession {
	next := browserSession{Version: browserSessionVersion, Mobile: previous.Mobile, Dark: previous.Dark, FromSetup: true}
	for i, url := range urls {
		if url == "chrome://newtab/" || url == "edge://newtab/" || url == "brave://newtab/" {
			url = "about:blank#surf-new"
		}
		if !restorableURL(url) {
			continue
		}
		if i == active {
			next.Active = len(next.Tabs)
		}
		next.Tabs = append(next.Tabs, url)
	}
	if len(next.Tabs) == 0 {
		next.Tabs = []string{"about:blank#surf-new"}
	}
	return next
}

func sameSession(a, b browserSession) bool {
	if a.Active != b.Active || a.Mobile != b.Mobile || a.Dark != b.Dark || a.FromSetup != b.FromSetup || len(a.Tabs) != len(b.Tabs) {
		return false
	}
	for i := range a.Tabs {
		if a.Tabs[i] != b.Tabs[i] {
			return false
		}
	}
	return true
}

func (s *setupBrowser) focus() error {
	_, err := s.client.Call(s.sessionID, "Runtime.evaluate", map[string]any{
		"expression":   `(async()=>{const ws=await chrome.windows.getAll({windowTypes:['normal']});const w=ws.find(w=>w.focused)||ws[ws.length-1];if(w)await chrome.windows.update(w.id,{focused:true,state:'normal'});})()`,
		"awaitPromise": true, "returnByValue": true})
	return err
}
