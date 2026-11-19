package browser

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/protocol"
)

func TestNormalizeNavURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a":  "https://example.com/a",
		"http://example.com":     "http://example.com",
		"about:blank":            "about:blank",
		"example.com":            "https://example.com",
		"news.ycombinator.com/x": "https://news.ycombinator.com/x",
		"cats and dogs":          "https://www.google.com/search?q=cats+and+dogs",
		"what is .net":           "https://www.google.com/search?q=what+is+.net",
		".hidden":                "https://www.google.com/search?q=.hidden",
		"golang":                 "https://www.google.com/search?q=golang",
		"  example.com  ":        "https://example.com",
		"":                       "",
	}
	for in, want := range cases {
		if got := NormalizeNavURL(in, defaultSearchURL); got != want {
			t.Errorf("NormalizeNavURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeNavURLCustomSearch(t *testing.T) {
	got := NormalizeNavURL("cats & dogs", "https://duckduckgo.com/?q={searchTerms}")
	if want := "https://duckduckgo.com/?q=cats+%26+dogs"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := NormalizeNavURL("example.com", "https://duckduckgo.com/?q={searchTerms}"); got != "https://example.com" {
		t.Fatalf("host input searched: %q", got)
	}
}

func TestParseSearchTemplate(t *testing.T) {
	for in, want := range map[string]string{
		"https://duckduckgo.com/?q={searchTerms}":                      "https://duckduckgo.com/?q={searchTerms}",
		"https://example.com/s?q=%s":                                   "https://example.com/s?q={searchTerms}",
		"https://www.bing.com/search?q={searchTerms}&{inputEncoding?}": "https://www.bing.com/search?q={searchTerms}&",
		" http://localhost:8080/?q={searchTerms} ":                     "http://localhost:8080/?q={searchTerms}",
		"https://google.com/{google:baseURL}search?q={searchTerms}":    "",
		"https://example.com/":                                         "",
		"ftp://example.com/?q={searchTerms}":                           "",
		"javascript:{searchTerms}":                                     "",
		"https://example.com/{searchTerms}{searchTerms}":               "",
		"": "",
	} {
		got, ok := parseSearchTemplate(in)
		if got != want || ok != (want != "") {
			t.Errorf("parseSearchTemplate(%q) = %q, %t; want %q", in, got, ok, want)
		}
	}
}

func TestSearchTemplateOrder(t *testing.T) {
	profile := t.TempDir()
	b := &Controller{cfg: &config.Config{Profile: profile}}
	if got := b.searchTemplate(); got != defaultSearchURL {
		t.Fatalf("no profile: %q", got)
	}
	if err := os.MkdirAll(filepath.Join(profile, "Default"), 0o700); err != nil {
		t.Fatal(err)
	}
	prefs := filepath.Join(profile, "Default", "Preferences")
	chromium := `{"default_search_provider_data":{"template_url_data":{"url":"https://duckduckgo.com/?q={searchTerms}"}}}`
	if err := os.WriteFile(prefs, []byte(chromium), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := b.searchTemplate(); got != "https://duckduckgo.com/?q={searchTerms}" {
		t.Fatalf("chromium choice: %q", got)
	}
	b.cfg.SearchURL = "https://www.startpage.com/do/search?q=%s"
	if got := b.searchTemplate(); got != "https://www.startpage.com/do/search?q={searchTerms}" {
		t.Fatalf("host setting: %q", got)
	}
	b.cfg.SearchURL = "not a template"
	if got := b.searchTemplate(); got != "https://duckduckgo.com/?q={searchTerms}" {
		t.Fatalf("invalid host setting: %q", got)
	}
	for _, bad := range []string{"{", `{"default_search_provider_data":{"template_url_data":{"url":"https://x.test/?q={searchTerms}","post_params":"q={searchTerms}"}}}`} {
		if err := os.WriteFile(prefs, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		b.cfg.SearchURL = ""
		if got := b.searchTemplate(); got != defaultSearchURL {
			t.Fatalf("bad prefs %q: %q", bad, got)
		}
	}
}

func TestMotionStallAccounting(t *testing.T) {
	controller := &Controller{}
	controller.noteMotionPhase("begin")
	controller.perfMu.Lock()
	controller.motionLastAUAt = time.Now().Add(-100 * time.Millisecond)
	controller.perfMu.Unlock()
	controller.checkMotionStall(time.Now())
	controller.perfMu.Lock()
	defer controller.perfMu.Unlock()
	if controller.motionStalls != 1 || !controller.motionStallLogged {
		t.Fatalf("motion stalls=%d logged=%t", controller.motionStalls, controller.motionStallLogged)
	}
}

func TestIsActiveSession(t *testing.T) {
	active := &Tab{ID: 1, Session: "active"}
	inactive := &Tab{ID: 2, Session: "inactive"}
	b := &Controller{
		tabs:      map[int]*Tab{1: active, 2: inactive},
		bySession: map[string]*Tab{"active": active, "child": active, "inactive": inactive},
		activeID:  1,
	}

	if !b.isActiveSession("active") {
		t.Fatal("active session was not recognized")
	}
	if !b.isActiveSession("child") {
		t.Fatal("active tab child session was not recognized")
	}
	if b.isActiveSession("inactive") {
		t.Fatal("inactive session was treated as active")
	}
	if b.isActiveSession("") {
		t.Fatal("empty session was treated as active")
	}
}

func TestNormalizeViewportPreservesClientDerivedSurfaces(t *testing.T) {
	// These are representative live stream surfaces, not profiles used by
	// production code. They cover compact phones/iPods, modern phones, iPads,
	// iPad Pro, both orientations, and an arbitrary non-device-specific size.
	surfaces := [][2]int{
		{320, 392}, {480, 232}, {320, 480}, {374, 578}, {414, 648}, {428, 838},
		{768, 950}, {1024, 694}, {834, 1120}, {1366, 950}, {1024, 1292},
		{1200, 700},
	}
	for _, surface := range surfaces {
		w, h := normalizeViewportSize(surface[0], surface[1], 768, 950)
		if w != surface[0] || h != surface[1] {
			t.Errorf("surface %dx%d normalized to %dx%d", surface[0], surface[1], w, h)
		}
	}
}

func TestNormalizeViewportUsesEvenEncoderBounds(t *testing.T) {
	w, h := normalizeViewportSize(31, 2001, 768, 950)
	if w != minViewportDimension || h != maxViewportDimension {
		t.Fatalf("normalized bounds = %dx%d, want %dx%d", w, h, minViewportDimension, maxViewportDimension)
	}
}

func TestNormalizeViewportPreservesAspectRatioAboveEncoderBounds(t *testing.T) {
	w, h := normalizeViewportSize(2560, 1408, 768, 950)
	if w != 1600 || h != 880 {
		t.Fatalf("wide fullscreen normalized to %dx%d, want 1600x880", w, h)
	}
	w, h = normalizeViewportSize(1408, 2560, 768, 950)
	if w != 880 || h != 1600 {
		t.Fatalf("tall fullscreen normalized to %dx%d, want 880x1600", w, h)
	}
}

func TestPageOrigin(t *testing.T) {
	cases := map[string]string{
		"https://example.com/path":      "https://example.com",
		"http://example.com:8080/path":  "http://example.com:8080",
		"about:blank":                   "",
		"chrome-error://chromewebdata/": "",
		"not a url":                     "",
	}
	for in, want := range cases {
		if got := pageOrigin(in); got != want {
			t.Errorf("pageOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveClientSizeWithoutScaleKeepsPointBehaviour(t *testing.T) {
	w, h, scale := resolveClientSize(&protocol.SizeCommand{W: 2560, H: 1408}, 768, 950)
	if w != 1600 || h != 880 || scale != 1 {
		t.Fatalf("legacy size = %dx%d@%v, want 1600x880@1", w, h, scale)
	}
	w, h, scale = resolveClientSize(&protocol.SizeCommand{W: 321, H: 481}, 768, 950)
	if w != 320 || h != 480 || scale != 1 {
		t.Fatalf("legacy odd size = %dx%d@%v", w, h, scale)
	}
}

func TestResolveClientSizeWithScaleIsPhysical(t *testing.T) {
	w, h, scale := resolveClientSize(&protocol.SizeCommand{W: 640, H: 960, Scale: 2}, 768, 950)
	if w != 640 || h != 960 || scale != 2 {
		t.Fatalf("2x size = %dx%d@%v", w, h, scale)
	}
	// A 1x client that sends a scale may use the larger hidpi bound.
	w, h, scale = resolveClientSize(&protocol.SizeCommand{W: 2560, H: 1408, Scale: 1}, 768, 950)
	if w != 2560 || h != 1408 || scale != 1 {
		t.Fatalf("1x wide size = %dx%d@%v", w, h, scale)
	}
	// Oversized surfaces shrink together with the scale: same CSS viewport.
	w, h, scale = resolveClientSize(&protocol.SizeCommand{W: 2880, H: 1800, Scale: 2}, 768, 950)
	if w > maxHiDPIViewportDimension || w*h > maxHiDPIViewportArea {
		t.Fatalf("oversized surface not clamped: %dx%d", w, h)
	}
	if cssW := float64(w) / scale; math.Abs(cssW-1440) > 2 {
		t.Fatalf("css width %v after clamp (%dx%d@%v), want about 1440", cssW, w, h, scale)
	}
	// Clamping never pushes the scale below one.
	_, _, scale = resolveClientSize(&protocol.SizeCommand{W: 4000, H: 2400, Scale: 1}, 768, 950)
	if scale != 1 {
		t.Fatalf("scale after clamp = %v, want 1", scale)
	}
}
