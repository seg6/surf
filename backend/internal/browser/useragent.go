package browser

import (
	"encoding/json"
	"fmt"
	"strings"
)

const nativeUserAgentMetadataExpression = `(async () => {
	if (!navigator.userAgentData) return null;
	return navigator.userAgentData.getHighEntropyValues([
		"architecture", "bitness", "formFactors", "fullVersionList",
		"model", "platform", "platformVersion", "uaFullVersion", "wow64"
	]);
})()`

type userAgentMetadataCaller interface {
	Call(sessionID, method string, params any) (json.RawMessage, error)
}

// nativeUserAgentMetadata asks the browser itself for the client hints it
// would expose without automation. chrome://version is a trustworthy local
// origin, so the high-entropy API is available without a network request.
// The target is created before discovery is enabled and never becomes a Surf
// tab.
func nativeUserAgentMetadata(client userAgentMetadataCaller) (map[string]any, error) {
	contextRaw, err := client.Call("", "Target.createBrowserContext", nil)
	if err != nil {
		return nil, fmt.Errorf("create browser identity context: %w", err)
	}
	var browserContext struct {
		ID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(contextRaw, &browserContext); err != nil {
		return nil, fmt.Errorf("decode browser identity context: %w", err)
	}
	if browserContext.ID == "" {
		return nil, fmt.Errorf("decode browser identity context: missing browserContextId")
	}
	defer func() {
		_, _ = client.Call("", "Target.disposeBrowserContext", map[string]any{
			"browserContextId": browserContext.ID,
		})
	}()

	targetRaw, err := client.Call("", "Target.createTarget", map[string]any{
		"url":              "chrome://version",
		"browserContextId": browserContext.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("create browser identity target: %w", err)
	}
	var target struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(targetRaw, &target); err != nil {
		return nil, fmt.Errorf("decode browser identity target: %w", err)
	}
	if target.TargetID == "" {
		return nil, fmt.Errorf("decode browser identity target: missing targetId")
	}
	attachRaw, err := client.Call("", "Target.attachToTarget", map[string]any{
		"targetId": target.TargetID,
		"flatten":  true,
	})
	if err != nil {
		return nil, fmt.Errorf("attach browser identity target: %w", err)
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachRaw, &attached); err != nil {
		return nil, fmt.Errorf("decode browser identity session: %w", err)
	}
	if attached.SessionID == "" {
		return nil, fmt.Errorf("decode browser identity session: missing sessionId")
	}

	raw, err := client.Call(attached.SessionID, "Runtime.evaluate", map[string]any{
		"expression":    nativeUserAgentMetadataExpression,
		"awaitPromise":  true,
		"returnByValue": true,
	})
	if err != nil {
		return nil, fmt.Errorf("read native browser identity: %w", err)
	}
	var evaluated struct {
		Result struct {
			Value map[string]any `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &evaluated); err != nil {
		return nil, fmt.Errorf("decode native browser identity: %w", err)
	}
	if len(evaluated.ExceptionDetails) != 0 && string(evaluated.ExceptionDetails) != "null" {
		return nil, fmt.Errorf("native browser identity evaluation failed")
	}
	metadata := evaluated.Result.Value
	if metadata == nil {
		return nil, fmt.Errorf("browser did not expose user-agent metadata")
	}
	if fullVersion, ok := metadata["uaFullVersion"]; ok {
		metadata["fullVersion"] = fullVersion
		delete(metadata, "uaFullVersion")
	}
	if _, ok := metadata["platform"].(string); !ok {
		return nil, fmt.Errorf("browser user-agent metadata has no platform")
	}
	if _, ok := metadata["mobile"].(bool); !ok {
		return nil, fmt.Errorf("browser user-agent metadata has no mobile state")
	}
	if brands, ok := metadata["brands"].([]any); !ok || len(brands) == 0 {
		return nil, fmt.Errorf("browser user-agent metadata has no brands")
	}
	return metadata, nil
}

// NormalizeHeadlessUserAgent performs Surf's only desktop identity change.
// Everything else, including Chrome's real brand list and platform details,
// comes from the browser unchanged.
func NormalizeHeadlessUserAgent(userAgent string) string {
	return strings.Replace(userAgent, "HeadlessChrome/", "Chrome/", 1)
}

func chromeVersionFromUserAgent(userAgent string) string {
	const marker = "Chrome/"
	start := strings.Index(userAgent, marker)
	if start < 0 {
		return ""
	}
	version := userAgent[start+len(marker):]
	if end := strings.IndexByte(version, ' '); end >= 0 {
		version = version[:end]
	}
	return version
}

func cloneUserAgentMetadata(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return nil
	}
	var clone map[string]any
	if json.Unmarshal(encoded, &clone) != nil {
		return nil
	}
	return clone
}

func userAgentOverrideParams(desktopUserAgent string, nativeMetadata map[string]any, mobile bool) map[string]any {
	if desktopUserAgent == "" || nativeMetadata == nil {
		return nil
	}
	metadata := cloneUserAgentMetadata(nativeMetadata)
	if metadata == nil {
		return nil
	}
	if !mobile {
		return map[string]any{
			"userAgent":         desktopUserAgent,
			"userAgentMetadata": metadata,
		}
	}

	version := chromeVersionFromUserAgent(desktopUserAgent)
	if version == "" {
		return nil
	}
	metadata["platform"] = "Android"
	metadata["platformVersion"] = "13.0.0"
	metadata["architecture"] = ""
	metadata["model"] = "Pixel 7"
	metadata["mobile"] = true
	metadata["bitness"] = ""
	metadata["wow64"] = false
	metadata["formFactors"] = []any{"Mobile"}
	return map[string]any{
		"userAgent": fmt.Sprintf(
			"Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 "+
				"(KHTML, like Gecko) Chrome/%s Mobile Safari/537.36", version),
		"platform":          "Linux armv8l",
		"userAgentMetadata": metadata,
	}
}
