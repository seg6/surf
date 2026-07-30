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
	data, err := os.ReadFile(filepath.Join(home, "desktop-instance.json"))
	if err != nil {
		return fmt.Errorf("Surf is already running")
	}
	var instance desktopInstance
	if json.Unmarshal(data, &instance) != nil || instance.URL == "" {
		return errors.New("Surf is already running but its Settings address is unavailable")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Post(instance.URL+"/api/activate", "text/plain", nil)
	if err != nil {
		return fmt.Errorf("activate running Surf: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("activate running Surf: HTTP %d", response.StatusCode)
	}
	return nil
}
