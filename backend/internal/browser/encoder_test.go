package browser

import "testing"

func TestTabCaptureCodecLevel(t *testing.T) {
	if got := encoderCodec(768, 950, 30); got != "avc1.42E01F" {
		t.Fatalf("768x950@30 codec = %q", got)
	}
	if got := encoderCodec(1024, 1024, 30); got != "avc1.42E029" {
		t.Fatalf("1024x1024@30 codec = %q", got)
	}
}
