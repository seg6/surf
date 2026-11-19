package browser

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"surf-backend/internal/config"
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

func TestAwaitNativeUserAgentMetadataSurvivesInitialDocument(t *testing.T) {
	caller := &identitySequenceCaller{}
	metadata, err := awaitNativeUserAgentMetadata(caller, "session", 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 2 {
		t.Fatalf("Runtime.evaluate calls = %d, want 2", caller.calls)
	}
	if metadata["platform"] != "Linux" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

type identitySequenceCaller struct {
	calls int
}

func (c *identitySequenceCaller) Call(_, method string, _ any) (json.RawMessage, error) {
	if method != "Runtime.evaluate" {
		return nil, nil
	}
	c.calls++
	if c.calls == 1 {
		return json.RawMessage(`{"result":{"value":null}}`), nil
	}
	return json.RawMessage(`{"result":{"value":{"platform":"Linux"}}}`), nil
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

func TestCustomUserAgentOverridesSurfIdentity(t *testing.T) {
	b := &Controller{
		cfg:               &config.Config{},
		userAgent:         "Mozilla/5.0 Chrome/150.0.0.0 Safari/537.36",
		userAgentMetadata: map[string]any{"platform": "Linux", "mobile": false, "brands": []any{"x"}},
	}
	if params := b.userAgentParams(true); params["userAgent"] == "Custom/1.0" || params["userAgentMetadata"] == nil {
		t.Fatalf("default identity changed: %v", params)
	}
	b.cfg.UserAgent = "Custom/1.0"
	for _, mobile := range []bool{false, true} {
		params := b.userAgentParams(mobile)
		if params["userAgent"] != "Custom/1.0" || len(params) != 1 {
			t.Fatalf("mobile=%t params=%v", mobile, params)
		}
	}
	// Without native metadata Surf applies no override, but a custom agent still does.
	b.userAgentMetadata = nil
	if params := b.userAgentParams(false); params["userAgent"] != "Custom/1.0" {
		t.Fatalf("params=%v", params)
	}
}

func TestScaledViewport(t *testing.T) {
	for _, test := range []struct {
		w, h        int
		scale       float64
		cssW, cssH  int
		deviceScale float64
	}{
		{640, 960, 0, 640, 960, 1},
		{640, 960, 1, 640, 960, 1},
		{640, 960, 0.5, 640, 960, 1},
		{640, 960, 1.5, 960, 1440, 2.0 / 3},
		{640, 960, 2, 1280, 1920, 0.5},
		{1024, 768, 1.25, 1280, 960, 0.8},
	} {
		w, h, ds := scaledViewport(test.w, test.h, test.scale)
		if w != test.cssW || h != test.cssH || math.Abs(ds-test.deviceScale) > 1e-9 {
			t.Errorf("scaledViewport(%d,%d,%v) = %d,%d,%v; want %d,%d,%v",
				test.w, test.h, test.scale, w, h, ds, test.cssW, test.cssH, test.deviceScale)
		}
		// The device pixels rendered must equal the client's surface.
		if math.Abs(float64(w)*ds-float64(test.w)) > 1e-9 {
			t.Errorf("scale %v: %d css px * %v != %d device px", test.scale, w, ds, test.w)
		}
	}
}
