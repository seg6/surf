package browser

import "testing"

func TestTabCaptureCodecLevel(t *testing.T) {
	if got := encoderCodec(640, 480, 60); got != "avc1.42E01F" {
		t.Fatalf("640x480 source-paced codec = %q", got)
	}
	if got := encoderCodec(768, 950, 60); got != "avc1.42E029" {
		t.Fatalf("768x950 source-paced codec = %q", got)
	}
	if got := encoderCodec(768, 950, 30); got != "avc1.42E01F" {
		t.Fatalf("768x950 adaptive codec = %q", got)
	}
	if got := encoderCodec(768, 950, 40); got != "avc1.42E029" {
		t.Fatalf("768x950 40fps codec = %q", got)
	}
	if got := encoderCodec(1920, 1080, 60); got != "avc1.42E029" {
		t.Fatalf("1920x1080 source-paced codec = %q", got)
	}
}
