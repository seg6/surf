package browser

import "testing"

func TestNormalizeNavURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a":  "https://example.com/a",
		"http://example.com":     "http://example.com",
		"about:blank":            "about:blank",
		"example.com":            "https://example.com",
		"news.ycombinator.com/x": "https://news.ycombinator.com/x",
		"cats and dogs":          "https://duckduckgo.com/?q=cats+and+dogs",
		"what is .net":           "https://duckduckgo.com/?q=what+is+.net",
		".hidden":                "https://duckduckgo.com/?q=.hidden",
		"golang":                 "https://duckduckgo.com/?q=golang",
		"  example.com  ":        "https://example.com",
		"":                       "",
	}
	for in, want := range cases {
		if got := NormalizeNavURL(in); got != want {
			t.Errorf("NormalizeNavURL(%q) = %q, want %q", in, got, want)
		}
	}
}
