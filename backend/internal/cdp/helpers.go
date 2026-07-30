package cdp

import "encoding/json"

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

func (c *Client) NavigationHistory(sessionID string) (NavigationHistory, error) {
	var h NavigationHistory
	err := c.CallInto(sessionID, "Page.getNavigationHistory", nil, &h)
	return h, err
}

func (c *Client) NavigateToHistoryEntry(sessionID string, id int) error {
	_, err := c.Call(sessionID, "Page.navigateToHistoryEntry", map[string]any{"entryId": id})
	return err
}
