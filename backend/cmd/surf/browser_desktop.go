package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"surf-backend/internal/protocol"
	"surf-backend/internal/web"
)

func (a *desktopApp) browserStatus() (protocol.BrowserModeEvent, error) {
	var mode protocol.BrowserModeEvent
	client, err := a.backendHTTPClient(time.Second)
	if err != nil {
		return mode, err
	}
	response, err := client.Get("https://127.0.0.1" + web.APIRoot + "/admin/browser")
	if err != nil {
		return mode, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return mode, fmt.Errorf("browser status unavailable")
	}
	err = json.NewDecoder(response.Body).Decode(&mode)
	return mode, err
}

func (a *desktopApp) updateBrowserItems(mode protocol.BrowserModeEvent) {
	status := map[string]string{"streaming": "Surf is running", "setup": "Browser setup active", "opening": "Opening browser setup…", "resuming": "Resuming Surf…", "failed": "Browser needs attention", "starting": "Surf is starting…"}[mode.State]
	if mode.Standalone {
		status = "Standalone browser setup — close it to start Surf"
	}
	a.setStatus(status)
	if a.browserItem != nil {
		title := "Browser setup…"
		if mode.State == "setup" {
			title = "Show setup browser"
		}
		a.browserItem.SetTitle(title)
		a.browserItem.SetDisabled(mode.CanForce || (mode.State != "streaming" && mode.State != "setup"))
	}
	if a.resumeItem != nil {
		a.resumeItem.SetDisabled(mode.Standalone || (mode.State != "setup" && mode.State != "failed"))
	}
}

func (a *desktopApp) openBrowserSetup() {
	mode, err := a.browserStatus()
	if err != nil || mode.State != "setup" {
		a.openManagement("#browser-setup-open")
		return
	}
	data, _ := json.Marshal(map[string]any{"action": "open", "revision": mode.Revision})
	client, err := a.backendHTTPClient(time.Second)
	if err != nil {
		return
	}
	response, err := client.Post("https://127.0.0.1"+web.APIRoot+"/admin/browser", "application/json", bytes.NewReader(data))
	if err != nil {
		a.openManagement("#browser-setup-open")
		return
	}
	if response.StatusCode != http.StatusAccepted {
		a.openManagement("#browser-setup-open")
	}
	response.Body.Close()
}

func (a *desktopApp) confirmBrowserInterruption(w http.ResponseWriter, r *http.Request) bool {
	mode, err := a.browserStatus()
	if err != nil || mode.State == "streaming" {
		return true
	}
	if mode.Standalone {
		http.Error(w, "Close standalone browser setup before starting or updating Surf", http.StatusConflict)
		return false
	}
	if r.Header.Get("X-Surf-Browser-Revision") != strconv.FormatUint(mode.Revision, 10) {
		http.Error(w, "Browser setup is active; confirm closing its windows before restarting or updating Surf", http.StatusConflict)
		return false
	}
	return true
}
