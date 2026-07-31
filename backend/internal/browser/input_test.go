package browser

import (
	"testing"
	"time"

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
		if got := NormalizeNavURL(in); got != want {
			t.Errorf("NormalizeNavURL(%q) = %q, want %q", in, got, want)
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

func TestScrollEventParamsArePrecisePixels(t *testing.T) {
	params := scrollEventParams(&protocol.ScrollCommand{
		X:  1.2,
		Y:  -0.1,
		DX: -0.25,
		DY: 0.1,
	}, 768, 950)
	if params["type"] != "mouseWheel" {
		t.Fatalf("unexpected wheel envelope: %#v", params)
	}
	if params["x"] != float64(768) || params["y"] != float64(0) {
		t.Fatalf("coordinates were not clamped and scaled: %#v", params)
	}
	if params["deltaX"] != float64(-192) || params["deltaY"] != float64(95) {
		t.Fatalf("deltas were not scaled to CSS pixels: %#v", params)
	}
}

func TestIsActiveSession(t *testing.T) {
	b := &Controller{
		tabs:     map[int]*Tab{},
		activeID: 1,
	}
	b.tabs[1] = &Tab{ID: 1, Session: "active"}
	b.tabs[2] = &Tab{ID: 2, Session: "inactive"}

	if !b.isActiveSession("active") {
		t.Fatal("active session was not recognized")
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
