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
	// 1920x1080 is 8160 macroblocks: 30 fps fits level 4.1 (244800 MB/s),
	// 60 fps needs level 4.2 (489600 MB/s).
	if got := encoderCodec(1920, 1080, 30); got != "avc1.42E029" {
		t.Fatalf("1920x1080@30 codec = %q", got)
	}
	if got := encoderCodec(1920, 1080, 60); got != "avc1.42E02A" {
		t.Fatalf("1920x1080@60 codec = %q", got)
	}
}

func TestEncoderLevelsForHiDPISurfaces(t *testing.T) {
	for _, test := range []struct {
		w, h, fps int
		codec     string
	}{
		{640, 1136, 60, "avc1.42E029"},  // iPhone 5s at 2x: 2840 MBs, 170400 MB/s
		{1536, 1152, 30, "avc1.42E029"}, // iPad 3 at 1.5x: 6912 MBs
		{1536, 1152, 60, "avc1.42E02A"}, // 414720 MB/s needs 4.2
		{2520, 1574, 60, "avc1.42E033"}, // desktop 1.75x: 15642 MBs needs 5.1
		{2560, 1600, 30, "avc1.42E033"}, // frame larger than 4.2 allows
	} {
		if got := encoderCodec(test.w, test.h, test.fps); got != test.codec {
			t.Errorf("encoderCodec(%d,%d,%d) = %s, want %s", test.w, test.h, test.fps, got, test.codec)
		}
		// The declared level must actually cover the (capped) stream.
		fps := levelFrameRate(test.w, test.h, test.fps)
		for _, level := range h264Levels {
			if level.codec != encoderCodec(test.w, test.h, fps) {
				continue
			}
			if mbs := macroblocks(test.w, test.h); mbs > level.maxFS || mbs*fps > level.maxMBPS {
				t.Errorf("%dx%d@%d exceeds %s", test.w, test.h, fps, level.codec)
			}
		}
	}
}

func TestLevelFrameRateCapsOversizedStreams(t *testing.T) {
	for _, test := range []struct{ w, h, want int }{
		{640, 1136, 60},  // well inside level 4.2
		{1920, 1080, 60}, // 489600 <= 522240
		{1920, 1088, 60}, // 8160 MBs
		{2560, 1600, 61}, // 16000 MBs: 983040/16000 = 61, target 60 stays
		{3840, 2160, 30}, // outside what Surf encodes; still bounded
	} {
		got := levelFrameRate(test.w, test.h, 60)
		want := min(60, test.want)
		if got != want {
			t.Errorf("levelFrameRate(%d,%d,60) = %d, want %d", test.w, test.h, got, want)
		}
	}
	if got := levelFrameRate(2560, 2560, 60); got != 983040/(160*160) {
		t.Errorf("2560x2560 fps = %d", got)
	}
}
