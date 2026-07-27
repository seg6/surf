// Command browsersmoke verifies that Chromium can navigate through Surf's
// actual CDP launch and user-agent normalization path.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"surf-backend/internal/browser"
	"surf-backend/internal/cdp"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: browsersmoke BROWSER [URL]")
		os.Exit(2)
	}
	targetURL := "https://example.com/"
	if len(os.Args) == 3 {
		targetURL = os.Args[2]
	}
	profile, err := os.MkdirTemp("", "surf-browser-smoke-")
	check(err)
	defer os.RemoveAll(profile)
	client, process, err := cdp.Launch(cdp.LaunchConfig{
		ChromePath: os.Args[1], Profile: profile, W: 1024, H: 768,
	})
	check(err)
	defer process.Kill()
	defer client.Close()

	var version struct {
		UserAgent string `json:"userAgent"`
	}
	raw, err := client.Call("", "Browser.getVersion", nil)
	check(err)
	check(json.Unmarshal(raw, &version))

	var targets struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	raw, err = client.Call("", "Target.getTargets", nil)
	check(err)
	check(json.Unmarshal(raw, &targets))
	targetID := ""
	for _, target := range targets.TargetInfos {
		if target.Type == "page" {
			targetID = target.TargetID
			break
		}
	}
	if targetID == "" {
		check(fmt.Errorf("browser created no page target"))
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	raw, err = client.Call("", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true})
	check(err)
	check(json.Unmarshal(raw, &attached))
	_, err = client.Call(attached.SessionID, "Network.enable", nil)
	check(err)
	_, err = client.Call(attached.SessionID, "Network.setUserAgentOverride", map[string]any{
		"userAgent": browser.NormalizeHeadlessUserAgent(version.UserAgent),
	})
	check(err)
	_, err = client.Call(attached.SessionID, "Page.enable", nil)
	check(err)
	_, err = client.Call(attached.SessionID, "Page.navigate", map[string]any{"url": targetURL})
	check(err)
	time.Sleep(15 * time.Second)
	var evaluated struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	raw, err = client.Call(attached.SessionID, "Runtime.evaluate", map[string]any{
		"expression":    "document.title + '\\n' + document.body.innerText.slice(0, 1000)",
		"returnByValue": true,
	})
	check(err)
	check(json.Unmarshal(raw, &evaluated))
	fmt.Println(evaluated.Result.Value)
	lower := strings.ToLower(evaluated.Result.Value)
	if strings.Contains(lower, "just a moment") || strings.Contains(lower, "verifying you are human") {
		check(fmt.Errorf("browser remained on an interstitial"))
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "browsersmoke:", err)
		os.Exit(1)
	}
}
