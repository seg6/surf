package browser

import (
	"testing"
	"time"
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
