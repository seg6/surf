package browser

import "testing"

func TestDarkModeMediaParams(t *testing.T) {
	for dark, want := range map[bool]string{false: "light", true: "dark"} {
		params := darkModeMediaParams(dark)
		features, ok := params["features"].([]map[string]any)
		if !ok || len(features) != 1 || features[0]["name"] != "prefers-color-scheme" ||
			features[0]["value"] != want {
			t.Fatalf("dark=%t params=%#v", dark, params)
		}
	}
}

func TestDisplayTabTitlePrefersDocumentTitleAndFallsBackToHost(t *testing.T) {
	cases := []struct {
		title string
		url   string
		want  string
	}{
		{"A real page title", "https://example.com/path", "A real page title"},
		{"https://example.com/path", "https://example.com/path", "example.com"},
		{"", "https://example.com:8443/path", "example.com:8443"},
		{"", "about:blank#surf-new", "New Tab"},
	}
	for _, tc := range cases {
		if got := displayTabTitle(tc.title, tc.url); got != tc.want {
			t.Errorf("displayTabTitle(%q, %q)=%q want %q", tc.title, tc.url, got, tc.want)
		}
	}
}
