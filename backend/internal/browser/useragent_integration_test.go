package browser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"surf-backend/internal/cdp"
)

// This opt-in test exercises the identity round trip against a real browser.
// It stays out of ordinary unit runs because CI targets several platforms and
// does not always have Chrome installed.
func TestBrowserIdentityRoundTrip(t *testing.T) {
	path := os.Getenv("SURF_TEST_BROWSER_IDENTITY")
	if path == "" {
		t.Skip("set SURF_TEST_BROWSER_IDENTITY to a Chrome/Chromium executable")
	}

	requests := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- r.Header.Clone():
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, browserProcess, err := cdp.Launch(cdp.LaunchConfig{
		ChromePath: path,
		Profile:    t.TempDir(),
		W:          800,
		H:          600,
		NoSandbox:  os.Geteuid() == 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer browserProcess.Kill()

	versionRaw, err := client.Call("", "Browser.getVersion", nil)
	if err != nil {
		t.Fatal(err)
	}
	var version struct {
		UserAgent string `json:"userAgent"`
	}
	if err := json.Unmarshal(versionRaw, &version); err != nil {
		t.Fatal(err)
	}
	metadata, err := nativeUserAgentMetadata(client)
	if err != nil {
		t.Fatal(err)
	}
	targetsRaw, err := client.Call("", "Target.getTargets", nil)
	if err != nil {
		t.Fatal(err)
	}
	var targets struct {
		Infos []struct {
			URL string `json:"url"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(targetsRaw, &targets); err != nil {
		t.Fatal(err)
	}
	for _, info := range targets.Infos {
		if strings.Contains(info.URL, "<title>Surf</title>") || strings.HasPrefix(info.URL, "http://127.0.0.1:") {
			t.Fatalf("browser identity target leaked into tab discovery: %q", info.URL)
		}
	}

	targetRaw, err := client.Call("", "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil {
		t.Fatal(err)
	}
	var target struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(targetRaw, &target); err != nil {
		t.Fatal(err)
	}
	attachRaw, err := client.Call("", "Target.attachToTarget", map[string]any{
		"targetId": target.TargetID,
		"flatten":  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachRaw, &attached); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(attached.SessionID, "Network.enable", nil); err != nil {
		t.Fatal(err)
	}
	wantUserAgent := NormalizeHeadlessUserAgent(version.UserAgent)
	params := userAgentOverrideParams(wantUserAgent, metadata, false)
	if _, err := client.Call(attached.SessionID, "Network.setUserAgentOverride", params); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(attached.SessionID, "Page.navigate", map[string]any{"url": server.URL}); err != nil {
		t.Fatal(err)
	}

	var desktopClientHints string
	select {
	case headers := <-requests:
		if got := headers.Get("User-Agent"); got != wantUserAgent || strings.Contains(got, "HeadlessChrome/") {
			t.Fatalf("User-Agent=%q want %q", got, wantUserAgent)
		}
		desktopClientHints = headers.Get("Sec-CH-UA")
		if desktopClientHints == "" {
			t.Fatal("native Sec-CH-UA was suppressed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("browser did not reach identity test server")
	}

	mobileParams := userAgentOverrideParams(wantUserAgent, metadata, true)
	if _, err := client.Call(attached.SessionID, "Network.setUserAgentOverride", mobileParams); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(attached.SessionID, "Page.navigate", map[string]any{"url": server.URL + "?mobile"}); err != nil {
		t.Fatal(err)
	}
	select {
	case headers := <-requests:
		if got := headers.Get("User-Agent"); !strings.Contains(got, "Android 13; Pixel 7") ||
			!strings.Contains(got, " Mobile Safari/") {
			t.Fatalf("mobile User-Agent=%q", got)
		}
		if got := headers.Get("Sec-CH-UA"); got != desktopClientHints {
			t.Fatalf("mobile brands=%q want native brands %q", got, desktopClientHints)
		}
		if got := headers.Get("Sec-CH-UA-Mobile"); got != "?1" {
			t.Fatalf("Sec-CH-UA-Mobile=%q want ?1", got)
		}
		if got := headers.Get("Sec-CH-UA-Platform"); got != `"Android"` {
			t.Fatalf("Sec-CH-UA-Platform=%q want Android", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("browser did not reach mobile identity test server")
	}
}
