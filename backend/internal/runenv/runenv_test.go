package runenv

import "testing"

func TestDisplaySocket(t *testing.T) {
	if got := displaySocket(":123.0"); got != "/tmp/.X11-unix/X123" {
		t.Fatalf("displaySocket=%q", got)
	}
}
