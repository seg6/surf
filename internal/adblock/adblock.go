// Package adblock answers "should this request be blocked?" against an
// embedded ad/tracker domain list (StevenBlack hosts, domains only, baked in
// at build time). Matching is by domain suffix: ads.example.com is blocked if
// either itself or example.com is listed.
package adblock

import (
	_ "embed"
	"net/url"
	"strings"
	"sync"
)

//go:embed hosts.txt
var rawList string

var (
	once sync.Once
	set  map[string]struct{}
)

func load() {
	set = make(map[string]struct{}, 90000)
	for _, line := range strings.Split(rawList, "\n") {
		d := strings.TrimSpace(line)
		if d != "" {
			set[d] = struct{}{}
		}
	}
}

// BlockedURL reports whether the URL's host (or any parent domain) is listed.
func BlockedURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return BlockedHost(u.Hostname())
}

func BlockedHost(host string) bool {
	once.Do(load)
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	for h != "" {
		if _, ok := set[h]; ok {
			return true
		}
		i := strings.IndexByte(h, '.')
		if i < 0 {
			return false
		}
		h = h[i+1:]
	}
	return false
}
