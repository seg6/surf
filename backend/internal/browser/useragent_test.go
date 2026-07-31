package browser

import (
	"strings"
	"testing"
)

func TestNormalizeHeadlessUserAgent(t *testing.T) {
	const in = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/150.0.0.0 Safari/537.36"
	const want = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	if got := NormalizeHeadlessUserAgent(in); got != want {
		t.Fatalf("NormalizeHeadlessUserAgent() = %q, want %q", got, want)
	}
	if got := NormalizeHeadlessUserAgent(want); got != want {
		t.Fatalf("ordinary Chrome UA changed to %q", got)
	}
}

func TestMobileUserAgentOverrideIsCoherent(t *testing.T) {
	const desktop = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.7922.47 Safari/537.36"
	params := userAgentOverrideParams(desktop, true)
	userAgent, _ := params["userAgent"].(string)
	if !strings.Contains(userAgent, "Android 13") ||
		!strings.Contains(userAgent, "Chrome/151.0.7922.47 Mobile") ||
		strings.Contains(userAgent, "X11") {
		t.Fatalf("mobile user agent = %q", userAgent)
	}
	metadata, _ := params["userAgentMetadata"].(map[string]any)
	if metadata["platform"] != "Android" || metadata["mobile"] != true {
		t.Fatalf("mobile metadata = %#v", metadata)
	}

	desktopParams := userAgentOverrideParams(desktop, false)
	desktopMetadata, _ := desktopParams["userAgentMetadata"].(map[string]any)
	if desktopParams["userAgent"] != desktop ||
		desktopMetadata["platform"] != "Linux" ||
		desktopMetadata["architecture"] != "x86" ||
		desktopMetadata["bitness"] != "64" ||
		desktopMetadata["mobile"] != false {
		t.Fatalf("desktop override = %#v", desktopParams)
	}
}

func TestEdgeUserAgentOverridePreservesClientHints(t *testing.T) {
	const edge = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36 Edg/150.0.4078.105"
	params := userAgentOverrideParams(edge, false)
	metadata, _ := params["userAgentMetadata"].(map[string]any)
	if metadata["platform"] != "Windows" ||
		metadata["platformVersion"] != "10.0.0" ||
		metadata["fullVersion"] != "150.0.4078.105" {
		t.Fatalf("Edge metadata = %#v", metadata)
	}
	brands, _ := metadata["brands"].([]any)
	foundEdge := false
	for _, item := range brands {
		brand, _ := item.(map[string]any)
		if brand["brand"] == "Microsoft Edge" && brand["version"] == "150" {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("Edge brand missing from %#v", brands)
	}
}
