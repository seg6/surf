package browser

import "testing"

func TestVideoResizeMailboxKeepsLatestRequest(t *testing.T) {
	b := &Controller{videoResizeWake: make(chan struct{}, 1)}
	b.requestVideoResize(800, 600, []byte{1}, "old")
	b.requestVideoResize(1024, 768, []byte{2}, "new")

	b.videoResizeMu.Lock()
	job := b.videoResizePending
	seq := b.videoResizeSeq
	b.videoResizeMu.Unlock()
	if job.w != 1024 || job.h != 768 || len(job.jpeg) != 1 || job.jpeg[0] != 2 || job.session != "new" {
		t.Fatalf("mailbox retained stale resize: %+v", job)
	}
	if seq != 2 || len(b.videoResizeWake) != 1 {
		t.Fatalf("mailbox seq/wake = %d/%d, want 2/1", seq, len(b.videoResizeWake))
	}
}

// buildTestJPEG constructs a minimal-but-marker-correct JPEG byte sequence
// (SOI, a skippable APP0, a SOF0 declaring w x h, EOI) — not a real
// decodable image, just enough structure for jpegSize's marker scan, so
// this test doesn't need an external fixture or to shell out to a real
// encoder.
func buildTestJPEG(w, h int) []byte {
	var b []byte
	b = append(b, 0xFF, 0xD8) // SOI

	// APP0 (JFIF header): marker scanning must skip over this correctly
	// before reaching SOF0.
	app0 := []byte{'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}
	b = append(b, 0xFF, 0xE0, 0x00, byte(len(app0)+2))
	b = append(b, app0...)

	// SOF0 (baseline): precision(1) height(2) width(2) numComponents(1) +
	// 3 bytes/component for 3 components.
	sof := []byte{
		8,                     // precision
		byte(h >> 8), byte(h), // height
		byte(w >> 8), byte(w), // width
		3,          // num components
		1, 0x22, 0, // Y
		2, 0x11, 1, // Cb
		3, 0x11, 1, // Cr
	}
	b = append(b, 0xFF, 0xC0, byte(len(sof)+2>>8), byte(len(sof)+2))
	b = append(b, sof...)

	b = append(b, 0xFF, 0xD9) // EOI
	return b
}

func TestJPEGSizeReadsSOF0Dimensions(t *testing.T) {
	for _, dims := range [][2]int{{800, 600}, {1024, 1366}, {768, 934}, {1, 1}} {
		data := buildTestJPEG(dims[0], dims[1])
		w, h, ok := jpegSize(data)
		if !ok {
			t.Fatalf("%v: jpegSize failed to find SOF0", dims)
		}
		if w != dims[0] || h != dims[1] {
			t.Fatalf("%v: jpegSize=%dx%d, want %dx%d", dims, w, h, dims[0], dims[1])
		}
	}
}

func TestJPEGSizeRejectsGarbage(t *testing.T) {
	for _, data := range [][]byte{
		nil,
		{0xFF, 0xD8},                   // SOI only, no SOF
		{0x00, 0x01, 0x02, 0x03},       // not even a JPEG
		{0xFF, 0xD8, 0xFF, 0xE0, 0x00}, // truncated segment
	} {
		if _, _, ok := jpegSize(data); ok {
			t.Fatalf("jpegSize accepted garbage: %v", data)
		}
	}
}
