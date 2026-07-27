package stream

import (
	"bytes"
	"testing"
	"time"
)

func nal(start4 bool, typ byte, payload ...byte) []byte {
	var b []byte
	if start4 {
		b = append(b, 0, 0, 0, 1)
	} else {
		b = append(b, 0, 0, 1)
	}
	b = append(b, typ&0x1F|0x60) // nal_ref_idc bits set, type in low 5
	if typ == 5 || typ == 9 {
		b[len(b)-1] = typ // AUD/IDR exactly as x264 emits (ref idc varies; type bits are what we scan)
	}
	return append(b, payload...)
}

func TestSplitterCutsOnAUD(t *testing.T) {
	// AU1: AUD SPS PPS IDR — AU2: AUD P — AU3 (incomplete): AUD
	au1 := bytes.Join([][]byte{
		nal(true, 9, 0x10),
		nal(true, 7, 0x42, 0x00, 0x1f),
		nal(true, 8, 0xce),
		nal(false, 5, 0x88, 0x84, 0x00),
	}, nil)
	au2 := bytes.Join([][]byte{
		nal(true, 9, 0x30),
		nal(false, 1, 0x9a, 0x00),
	}, nil)
	tail := nal(true, 9, 0x10)

	full := bytes.Join([][]byte{au1, au2, tail}, nil)

	// Feed in awkward chunk sizes to exercise split start codes.
	for _, chunk := range []int{1, 3, 7, 1000} {
		sp := newAUSplitter()
		var got []AU
		for i := 0; i < len(full); i += chunk {
			end := min(i+chunk, len(full))
			aus, err := sp.feed(full[i:end])
			if err != nil {
				t.Fatalf("chunk=%d: feed error: %v", chunk, err)
			}
			got = append(got, aus...)
		}
		if len(got) != 2 {
			t.Fatalf("chunk=%d: got %d AUs, want 2", chunk, len(got))
		}
		if !bytes.Equal(got[0].Data, au1) {
			t.Errorf("chunk=%d: AU1 bytes mismatch", chunk)
		}
		if !bytes.Equal(got[1].Data, au2) {
			t.Errorf("chunk=%d: AU2 bytes mismatch", chunk)
		}
		if !got[0].IDR {
			t.Errorf("chunk=%d: AU1 should be IDR", chunk)
		}
		if got[1].IDR {
			t.Errorf("chunk=%d: AU2 should not be IDR", chunk)
		}
	}
}

func TestSubBackpressureResyncsOnIDR(t *testing.T) {
	s := New(Config{W: 64, H: 64, FPS: 15, BitrateK: 100, MaxrateK: 100, BufsizeK: 50})
	sub := &Sub{C: make(chan AU, 2), s: s, fresh: true, dropped: true, gen: 1}

	// Fresh sub: P-frames before the first IDR are skipped.
	sub.offer(AU{Seq: 1, IDR: false}, 1)
	if len(sub.C) != 0 {
		t.Fatal("fresh sub accepted a P-frame")
	}
	sub.offer(AU{Seq: 2, IDR: true}, 1)
	sub.offer(AU{Seq: 3, IDR: false}, 1)
	if len(sub.C) != 2 {
		t.Fatalf("want 2 queued, got %d", len(sub.C))
	}
	// Queue full: drop everything until the next IDR.
	sub.offer(AU{Seq: 4, IDR: false}, 1)
	sub.offer(AU{Seq: 5, IDR: true}, 1) // still full → stays dropped
	<-sub.C
	<-sub.C
	sub.offer(AU{Seq: 6, IDR: false}, 1) // dropped: waiting for IDR
	if len(sub.C) != 0 {
		t.Fatal("P-frame delivered while waiting for IDR")
	}
	sub.offer(AU{Seq: 7, IDR: true}, 0)
	if len(sub.C) != 0 {
		t.Fatal("stale generation delivered")
	}
	sub.offer(AU{Seq: 7, IDR: true}, 1)
	if got := <-sub.C; got.Seq != 7 || !got.IDR {
		t.Fatalf("resync frame = %+v, want IDR seq 7", got)
	}
}

// TestRequestKeyframeNoopWhenNotRunning exercises the guard clauses only —
// actually restarting ffmpeg needs a real binary and display.
func TestRequestKeyframeNoopWhenNotRunning(t *testing.T) {
	s := &Streamer{subs: map[*Sub]struct{}{}}
	s.RequestKeyframe()
	if s.running {
		t.Fatal("RequestKeyframe started the encoder while not running")
	}
	if !s.lastKeyframeReq.IsZero() {
		t.Fatal("RequestKeyframe touched lastKeyframeReq on a no-op call")
	}
}

func TestRequestKeyframeCooldown(t *testing.T) {
	s := &Streamer{subs: map[*Sub]struct{}{}, running: true}
	recent := time.Now()
	s.lastKeyframeReq = recent
	s.RequestKeyframe() // within cooldown: must return before touching cmd/lastKeyframeReq
	if s.cmd != nil {
		t.Fatal("RequestKeyframe restarted the encoder within the cooldown window")
	}
	if !s.lastKeyframeReq.Equal(recent) {
		t.Fatal("RequestKeyframe updated lastKeyframeReq despite being suppressed by cooldown")
	}
}

func TestSubscribeFailsImmediatelyWhenCaptureArgsUnset(t *testing.T) {
	s := New(Config{W: 64, H: 64, FPS: 15, BitrateK: 100, MaxrateK: 100, BufsizeK: 50})
	sub := s.Subscribe()
	select {
	case _, ok := <-sub.C:
		if ok {
			t.Fatal("expected closed channel, got a value")
		}
	default:
		t.Fatal("expected sub.C already closed after Subscribe")
	}
}

// TestSubscribeFailsWhenCaptureArgsReturnsEmpty guards against a real bug: a
// platform Handle's method value (e.g. windowsHandle{}.VideoCaptureArgs) is
// never a nil func even when calling it always returns an empty slice, so
// args()/startLocked must check the result, not whether CaptureArgs itself
// is nil.
func TestSubscribeFailsWhenCaptureArgsReturnsEmpty(t *testing.T) {
	s := New(Config{
		W: 64, H: 64, FPS: 15, BitrateK: 100, MaxrateK: 100, BufsizeK: 50,
		CaptureArgs: func(string, int, int, int) []string { return nil },
	})
	sub := s.Subscribe()
	select {
	case _, ok := <-sub.C:
		if ok {
			t.Fatal("expected closed channel, got a value")
		}
	default:
		t.Fatal("expected sub.C already closed after Subscribe")
	}
}
