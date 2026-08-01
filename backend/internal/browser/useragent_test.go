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
	native := testChromeMetadata()
	params := userAgentOverrideParams(desktop, native, true)
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
	if !metadataHasBrand(metadata, "Google Chrome", "151") {
		t.Fatalf("native Chrome brand missing from mobile metadata: %#v", metadata)
	}
	if native["platform"] != "Linux" || native["mobile"] != false {
		t.Fatalf("native metadata was mutated: %#v", native)
	}

	desktopParams := userAgentOverrideParams(desktop, native, false)
	desktopMetadata, _ := desktopParams["userAgentMetadata"].(map[string]any)
	if desktopParams["userAgent"] != desktop ||
		desktopMetadata["platform"] != "Linux" ||
		desktopMetadata["architecture"] != "x86" ||
		desktopMetadata["bitness"] != "64" ||
		desktopMetadata["mobile"] != false {
		t.Fatalf("desktop override = %#v", desktopParams)
	}
}

func TestDesktopUserAgentOverridePreservesNativeClientHints(t *testing.T) {
	const edge = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36 Edg/150.0.4078.105"
	native := map[string]any{
		"brands": []any{
			map[string]any{"brand": "Not=A?Brand", "version": "99"},
			map[string]any{"brand": "Chromium", "version": "150"},
			map[string]any{"brand": "Microsoft Edge", "version": "150"},
		},
		"fullVersion": "150.0.4078.105", "platform": "Windows",
		"platformVersion": "10.0.0", "architecture": "x86", "model": "",
		"mobile": false, "bitness": "64", "wow64": false,
		"formFactors": []any{"Desktop"},
	}
	params := userAgentOverrideParams(edge, native, false)
	metadata, _ := params["userAgentMetadata"].(map[string]any)
	if metadata["platform"] != "Windows" ||
		metadata["platformVersion"] != "10.0.0" ||
		metadata["fullVersion"] != "150.0.4078.105" {
		t.Fatalf("Edge metadata = %#v", metadata)
	}
	if !metadataHasBrand(metadata, "Microsoft Edge", "150") {
		t.Fatalf("Edge brand missing from %#v", metadata["brands"])
	}
}

func TestUserAgentOverrideRequiresNativeMetadata(t *testing.T) {
	if got := userAgentOverrideParams("Chrome/151.0.0.0", nil, false); got != nil {
		t.Fatalf("partial override = %#v, want nil", got)
	}
}

func testChromeMetadata() map[string]any {
	return map[string]any{
		"brands": []any{
			map[string]any{"brand": "Not=A?Brand", "version": "99"},
			map[string]any{"brand": "Google Chrome", "version": "151"},
			map[string]any{"brand": "Chromium", "version": "151"},
		},
		"fullVersionList": []any{
			map[string]any{"brand": "Not=A?Brand", "version": "99.0.0.0"},
			map[string]any{"brand": "Google Chrome", "version": "151.0.7922.47"},
			map[string]any{"brand": "Chromium", "version": "151.0.7922.47"},
		},
		"fullVersion": "151.0.7922.47", "platform": "Linux",
		"platformVersion": "", "architecture": "x86", "model": "",
		"mobile": false, "bitness": "64", "wow64": false,
		"formFactors": []any{"Desktop"},
	}
}

func metadataHasBrand(metadata map[string]any, wantName, wantVersion string) bool {
	brands, _ := metadata["brands"].([]any)
	for _, item := range brands {
		brand, _ := item.(map[string]any)
		if brand["brand"] == wantName && brand["version"] == wantVersion {
			return true
		}
	}
	return false
}
