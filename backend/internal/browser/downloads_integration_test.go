package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/config"
	"surf-backend/internal/process"
)

// Part of the normal go test ./... suite. Uses a private profile and a local
// fixture: no user browser or internet. SURF_TEST_BROWSER can override discovery.
func TestChromiumDownloads(t *testing.T) {
	path := os.Getenv("SURF_TEST_BROWSER")
	if path != "" {
		resolved, err := exec.LookPath(path)
		if err != nil {
			t.Fatalf("SURF_TEST_BROWSER: %v", err)
		}
		path = resolved
	} else {
		for _, candidate := range []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser"} {
			if resolved, err := exec.LookPath(candidate); err == nil {
				path = resolved
				break
			}
		}
	}
	if path == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("Chromium is required in CI; install it or set SURF_TEST_BROWSER")
		}
		t.Skip("Chromium not installed; install it or set SURF_TEST_BROWSER")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\"test book.txt\"")
		if r.URL.Path == "/slow" {
			w.Header().Set("Content-Length", "1000000")
			// Enough bytes for Chromium's MIME sniffing before we stall.
			_, _ = w.Write(make([]byte, 64*1024))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		_, _ = fmt.Fprint(w, "download fixture")
	}))
	defer server.Close()
	client, proc, err := cdp.Launch(cdp.LaunchConfig{ChromePath: path, Profile: t.TempDir(), W: 800, H: 600, NoSandbox: os.Geteuid() == 0})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill(proc.Pid)
	defer client.Close()
	b := &Controller{cfg: &config.Config{DownloadsDir: t.TempDir()}, cdp: client}
	if err := b.setupDownloads(); err != nil {
		t.Fatal(err)
	}
	completed := make(chan string, 4)
	started := make(chan string, 4)
	client.OnEvent(func(ev cdp.Event) {
		switch ev.Method {
		case "Browser.downloadWillBegin":
			b.onEvent(ev)
			var p struct{ GUID string }
			_ = json.Unmarshal(ev.Params, &p)
			started <- p.GUID
		case "Browser.downloadProgress":
			b.onEvent(ev)
			var p struct{ State string }
			_ = json.Unmarshal(ev.Params, &p)
			if p.State == "completed" || p.State == "canceled" {
				completed <- p.State
			}
		}
	})
	wait := func(ch <-chan string) string {
		t.Helper()
		select {
		case value := <-ch:
			return value
		case <-time.After(15 * time.Second):
			t.Fatal("timed out waiting for Chromium download event")
			return ""
		}
	}
	navigate := func(url string) {
		t.Helper()
		if _, err := client.Call("", "Target.createTarget", map[string]any{"url": url}); err != nil {
			t.Fatal(err)
		}
	}
	navigate(server.URL + "/file")
	navigate(server.URL + "/file")
	for i := 0; i < 2; i++ {
		wait(started)
		if state := wait(completed); state != "completed" {
			t.Fatal(state)
		}
	}
	items, err := b.downloads.list()
	if err != nil || len(items) != 2 {
		t.Fatalf("downloads: %v %v", items, err)
	}
	for _, item := range items {
		data, err := os.ReadFile(filepath.Join(b.downloads.dir, item.Name))
		if err != nil || string(data) != "download fixture" {
			t.Fatalf("file: %q %v", data, err)
		}
	}
	navigate(server.URL + "/slow")
	guid := wait(started)
	if _, err := client.Call("", "Browser.cancelDownload", map[string]any{"guid": guid}); err != nil {
		t.Fatal(err)
	}
	if state := wait(completed); state != "canceled" {
		t.Fatal(state)
	}
	items, err = b.downloads.list()
	if err != nil || len(items) != 2 {
		t.Fatalf("canceled file leaked: %v %v", items, err)
	}
}
