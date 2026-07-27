package browser

import "testing"

func TestNormalizeHeadlessUserAgent(t *testing.T) {
	const in = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/150.0.0.0 Safari/537.36"
	const want = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	if got := normalizeHeadlessUserAgent(in); got != want {
		t.Fatalf("normalizeHeadlessUserAgent() = %q, want %q", got, want)
	}
	if got := normalizeHeadlessUserAgent(want); got != want {
		t.Fatalf("ordinary Chrome UA changed to %q", got)
	}
}
