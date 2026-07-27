package browser

import "testing"

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
