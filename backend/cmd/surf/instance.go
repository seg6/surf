package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type desktopInstance struct {
	URL string `json:"url"`
}

func writeDesktopInstance(home, url string) error {
	data, err := json.Marshal(desktopInstance{URL: url})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(home, "desktop-instance.json"), data, 0o600)
}

func activateDesktopInstance(home string) error {
	return requestDesktopInstance(home, "/api/activate")
}

func quitDesktopInstance(home string) error {
	return requestDesktopInstance(home, "/api/quit")
}

func requestDesktopInstance(home, path string) error {
	data, err := os.ReadFile(filepath.Join(home, "desktop-instance.json"))
	if err != nil {
		return fmt.Errorf("Surf is already running")
	}
	var instance desktopInstance
	if json.Unmarshal(data, &instance) != nil || instance.URL == "" {
		return errors.New("Surf is already running but its Settings address is unavailable")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	request, err := http.NewRequest(http.MethodPost, instance.URL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Surf-Desktop", "1")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("contact running Surf: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("contact running Surf: HTTP %d", response.StatusCode)
	}
	return nil
}
