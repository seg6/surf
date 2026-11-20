package browser

import (
	"encoding/json"
	"testing"
	"time"

	"surf-backend/internal/config"
)

func TestCaptureWindowMatchesPageArea(t *testing.T) {
	client, session := launchInputTestBrowser(t)
	raw, err := client.Call(session, "Target.getTargetInfo", nil)
	if err != nil {
		t.Fatal(err)
	}
	var target struct {
		TargetInfo struct {
			TargetID string `json:"targetId"`
		} `json:"targetInfo"`
	}
	if err := json.Unmarshal(raw, &target); err != nil || target.TargetInfo.TargetID == "" {
		t.Fatalf("Target.getTargetInfo = %s, %v", raw, err)
	}
	b := &Controller{cfg: &config.Config{}, cdp: client, viewW: 960, viewH: 544, viewScale: 1}
	var overridden struct {
		Result struct {
			Value int `json:"value"`
		} `json:"result"`
	}
	if err := client.CallInto(session, "Runtime.evaluate", map[string]any{
		"expression":    `Object.defineProperty(window, 'innerHeight', {get: () => 999}); innerHeight`,
		"returnByValue": true,
	}, &overridden); err != nil || overridden.Result.Value != 999 {
		t.Fatalf("overridden innerHeight = %d, %v", overridden.Result.Value, err)
	}
	b.applyView(&Tab{TargetID: target.TargetInfo.TargetID, Session: session})
	if _, err := client.Call(session, "Emulation.clearDeviceMetricsOverride", nil); err != nil {
		t.Fatal(err)
	}
	contextID := b.captureWorld(session)
	if contextID == 0 {
		t.Fatal("could not create an isolated world to read the real page size")
	}
	var got []int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		raw, err := client.Call(session, "Runtime.evaluate", map[string]any{
			"expression": "[innerWidth, innerHeight]", "returnByValue": true, "contextId": contextID,
		})
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Result struct {
				Value []int `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &page); err != nil || len(page.Result.Value) != 2 {
			t.Fatalf("Runtime.evaluate = %s, %v", raw, err)
		}
		got = page.Result.Value
		if got[0] == 960 && got[1] == 544 {
			return
		}
	}
	t.Fatalf("page area = %v, want 960x544", got)
}
