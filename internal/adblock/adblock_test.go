package adblock

import "testing"

func TestBlockedHost(t *testing.T) {
	// doubleclick.net is on every ad list that has ever existed.
	if !BlockedHost("doubleclick.net") {
		t.Fatal("doubleclick.net should be blocked")
	}
	if !BlockedHost("ads.doubleclick.net") {
		t.Fatal("subdomain of listed domain should be blocked")
	}
	if BlockedHost("example.com") || BlockedHost("duckduckgo.com") {
		t.Fatal("innocent domains blocked")
	}
	if BlockedHost("") {
		t.Fatal("empty host blocked")
	}
}

func TestBlockedURL(t *testing.T) {
	if !BlockedURL("https://ads.doubleclick.net/x.js?y=1") {
		t.Fatal("url on listed domain should be blocked")
	}
	if BlockedURL("https://example.com/ads.js") {
		t.Fatal("path must not influence matching")
	}
}
