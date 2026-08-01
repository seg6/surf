package browser

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
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
// would expose without automation. A temporary loopback origin is used because
// userAgentData is deliberately restricted to trustworthy contexts and some
// Chrome builds do not expose it on chrome:// pages. No request leaves the
// machine. The target is created before discovery is enabled and never becomes
// a Surf tab.
func nativeUserAgentMetadata(client userAgentMetadataCaller) (map[string]any, error) {
	origin, closeOrigin, err := browserIdentityOrigin()
	if err != nil {
		return nil, err
	}
	defer closeOrigin()

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
		"url":              origin,
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

	metadata, err := awaitNativeUserAgentMetadata(client, attached.SessionID, 3*time.Second)
	if err != nil {
		return nil, err
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

func browserIdentityOrigin() (string, func(), error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen for browser identity probe: %w", err)
	}
	server := &http.Server{
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!doctype html><title>Surf</title>"))
		}),
	}
	go func() { _ = server.Serve(listener) }()
	closeOrigin := func() { _ = server.Close() }
	return "http://" + listener.Addr().String() + "/", closeOrigin, nil
}

func awaitNativeUserAgentMetadata(client userAgentMetadataCaller, sessionID string, timeout time.Duration) (map[string]any, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		raw, err := client.Call(sessionID, "Runtime.evaluate", map[string]any{
			"expression":    nativeUserAgentMetadataExpression,
			"awaitPromise":  true,
			"returnByValue": true,
		})
		if err != nil {
			lastErr = fmt.Errorf("read native browser identity: %w", err)
		} else {
			var evaluated struct {
				Result struct {
					Value map[string]any `json:"value"`
				} `json:"result"`
				ExceptionDetails json.RawMessage `json:"exceptionDetails"`
			}
			if err := json.Unmarshal(raw, &evaluated); err != nil {
				lastErr = fmt.Errorf("decode native browser identity: %w", err)
			} else if len(evaluated.ExceptionDetails) != 0 && string(evaluated.ExceptionDetails) != "null" {
				lastErr = fmt.Errorf("native browser identity evaluation failed")
			} else if evaluated.Result.Value != nil {
				return evaluated.Result.Value, nil
			} else {
				lastErr = fmt.Errorf("browser did not expose user-agent metadata")
			}
		}
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		time.Sleep(25 * time.Millisecond)
	}
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
