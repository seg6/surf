package cdp

import (
	"encoding/base64"
	"encoding/json"
)

type NavigationEntry struct {
	ID int `json:"id"`
}

type NavigationHistory struct {
	CurrentIndex int               `json:"currentIndex"`
	Entries      []NavigationEntry `json:"entries"`
}

func (h NavigationHistory) CanGoBack() bool {
	return h.CurrentIndex > 0
}

func (h NavigationHistory) CanGoForward() bool {
	return h.CurrentIndex < len(h.Entries)-1
}

func (c *Client) CallInto(sessionID, method string, params any, out any) error {
	raw, err := c.Call(sessionID, method, params)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) EvaluateString(sessionID, expression string) (string, error) {
	var p struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	err := c.CallInto(sessionID, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true,
	}, &p)
	return p.Result.Value, err
}

func (c *Client) EvaluateBool(sessionID, expression string) (bool, error) {
	var p struct {
		Result struct {
			Value bool `json:"value"`
		} `json:"result"`
	}
	err := c.CallInto(sessionID, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true,
	}, &p)
	return p.Result.Value, err
}

func (c *Client) CaptureJPEG(sessionID string, quality int) ([]byte, error) {
	var p struct {
		Data string `json:"data"`
	}
	if err := c.CallInto(sessionID, "Page.captureScreenshot", map[string]any{
		"format": "jpeg", "quality": quality,
		"fromSurface": true, "captureBeyondViewport": false, "optimizeForSpeed": true,
	}, &p); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(p.Data)
}

func (c *Client) NavigationHistory(sessionID string) (NavigationHistory, error) {
	var h NavigationHistory
	err := c.CallInto(sessionID, "Page.getNavigationHistory", nil, &h)
	return h, err
}

func (c *Client) NavigateToHistoryEntry(sessionID string, id int) error {
	_, err := c.Call(sessionID, "Page.navigateToHistoryEntry", map[string]any{"entryId": id})
	return err
}

func (c *Client) SetDeviceMetrics(sessionID string, width, height int, deviceScaleFactor float64) error {
	_, err := c.Call(sessionID, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width": width, "height": height, "deviceScaleFactor": deviceScaleFactor, "mobile": false,
	})
	return err
}

// SetContentsSize resizes full Chrome's real (hidden in headless mode)
// platform window content area. Page emulation alone changes CSS metrics but
// does not necessarily resize that compositor surface.
func (c *Client) SetContentsSize(targetID string, width, height int) error {
	var window struct {
		WindowID int `json:"windowId"`
	}
	if err := c.CallInto("", "Browser.getWindowForTarget", map[string]any{
		"targetId": targetID,
	}, &window); err != nil {
		return err
	}
	if window.WindowID == 0 {
		return nil
	}
	_, err := c.Call("", "Browser.setContentsSize", map[string]any{
		"windowId": window.WindowID,
		"width":    width,
		"height":   height,
	})
	return err
}
