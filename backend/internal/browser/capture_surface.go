package browser

import (
	"encoding/json"
	"log"
)

func (b *Controller) resizeCaptureSurface(targetID string, width, height int) {
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
