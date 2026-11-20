package browser

import (
	"encoding/json"
	"log"
)

func (b *Controller) resizeCaptureSurface(targetID, session string, width, height int) {
	if targetID == "" || width < 1 || height < 1 {
		return
	}
	raw, err := b.cdp.Call("", "Browser.getWindowForTarget", map[string]any{
		"targetId": targetID,
	})
	if err != nil {
		log.Printf("tab capture: get capture surface: %v", err)
		return
	}
	var window struct {
		ID     int `json:"windowId"`
		Bounds struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"bounds"`
	}
	if err := json.Unmarshal(raw, &window); err != nil || window.ID == 0 {
		log.Printf("tab capture: decode capture surface: %v", err)
		return
	}
	// Size the page directly when Chromium supports it. This does not
	// inspect the website or briefly lift its device emulation.
	if _, err := b.cdp.Call("", "Browser.setContentsSize", map[string]any{
		"windowId": window.ID, "width": width, "height": height,
	}); err == nil {
		return
	}
	// Older Chromium builds only expose outer-window sizing.
	extraW, extraH := b.browserChrome(session)
	width, height = width+extraW, height+extraH
	if window.Bounds.Width == width && window.Bounds.Height == height {
		return
	}
	if _, err := b.cdp.Call("", "Browser.setWindowBounds", map[string]any{
		"windowId": window.ID,
		"bounds": map[string]any{
			"width": width, "height": height,
		},
	}); err != nil {
		log.Printf("tab capture: resize capture surface to %dx%d: %v", width, height, err)
		return
	}
	log.Printf("tab capture: capture surface resized to %dx%d", width, height)
}

// captureWorld returns an isolated execution context: the page cannot
// replace the browser's viewport properties in this world.
func (b *Controller) captureWorld(session string) int {
	if session == "" {
		return 0
	}
	var tree struct {
		FrameTree struct {
			Frame struct {
				ID string `json:"id"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	if err := b.cdp.CallInto(session, "Page.getFrameTree", nil, &tree); err != nil || tree.FrameTree.Frame.ID == "" {
		return 0
	}
	var world struct {
		ID int `json:"executionContextId"`
	}
	if err := b.cdp.CallInto(session, "Page.createIsolatedWorld", map[string]any{
		"frameId": tree.FrameTree.Frame.ID, "worldName": "surf-capture-size",
	}, &world); err != nil {
		return 0
	}
	return world.ID
}

// browserChrome is how much of the window Chromium's own UI takes: the
// window's outer size minus the page's inner size, measured with the device
// emulation lifted (applyView sets it again right after).
func (b *Controller) browserChrome(session string) (int, int) {
	contextID := b.captureWorld(session)
	if contextID == 0 {
		return 0, 0
	}
	if _, err := b.cdp.Call(session, "Emulation.clearDeviceMetricsOverride", nil); err != nil {
		return 0, 0
	}
	raw, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression":    "[outerWidth - innerWidth, outerHeight - innerHeight]",
		"returnByValue": true,
		"contextId":     contextID,
	})
	if err != nil {
		return 0, 0
	}
	var res struct {
		Result struct {
			Value []int `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &res) != nil || len(res.Result.Value) != 2 {
		return 0, 0
	}
	w, h := res.Result.Value[0], res.Result.Value[1]
	if w < 0 || w > 400 || h < 0 || h > 400 {
		return 0, 0
	}
	return w, h
}
