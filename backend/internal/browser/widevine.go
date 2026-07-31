package browser

import (
	"encoding/json"
	"log"
	"net/url"
	"strings"
)

const widevineProbeExpression = `(() => {
  if (!navigator.requestMediaKeySystemAccess) {
    return Promise.resolve({available: false, detail: "Encrypted Media Extensions unavailable"});
  }
  return navigator.requestMediaKeySystemAccess("com.widevine.alpha", [{
    initDataTypes: ["cenc"],
    audioCapabilities: [{contentType: 'audio/mp4; codecs="mp4a.40.2"'}],
    videoCapabilities: [{contentType: 'video/mp4; codecs="avc1.42E01E"'}]
  }]).then(
    () => ({available: true, detail: ""}),
    error => ({available: false, detail: error.name || "key system unavailable"})
  );
})()`

func isTrustedMediaOrigin(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "http" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// probeWidevine asks the browser's EME implementation instead of guessing
// from the operating system or executable name. A browser that bundles or has
// installed Widevine can therefore expose it on every host platform Surf runs
// on; Surf never copies or redistributes the proprietary CDM itself.
func (b *Controller) probeWidevine(session string) {
	b.mu.Lock()
	if b.widevineState != "unknown" || b.widevineProbing {
		b.mu.Unlock()
		return
	}
	tab := b.bySession[session]
	if tab == nil || !isTrustedMediaOrigin(tab.URL) {
		b.mu.Unlock()
		return
	}
	b.widevineProbing = true
	b.mu.Unlock()

	raw, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression":    widevineProbeExpression,
		"awaitPromise":  true,
		"returnByValue": true,
	})
	state, detail := "unavailable", ""
	if err != nil {
		detail = err.Error()
	} else {
		var response struct {
			Result struct {
				Value struct {
					Available bool   `json:"available"`
					Detail    string `json:"detail"`
				} `json:"value"`
			} `json:"result"`
			Exception json.RawMessage `json:"exceptionDetails"`
		}
		if json.Unmarshal(raw, &response) != nil || len(response.Exception) != 0 {
			detail = "probe failed"
		} else {
			detail = response.Result.Value.Detail
			if response.Result.Value.Available {
				state = "available"
			}
		}
	}
	b.mu.Lock()
	b.widevineState = state
	b.widevineDetail = detail
	b.widevineProbing = false
	b.mu.Unlock()
	if detail == "" {
		log.Printf("browser DRM: Widevine %s", state)
	} else {
		log.Printf("browser DRM: Widevine %s (%s)", state, detail)
	}
}
