package stream

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type countingWriter struct{ n atomic.Int64 }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n.Add(1)
	return len(p), nil
}

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

func TestRestartMovesEverySubscriberToNewGeneration(t *testing.T) {
	s := &Streamer{subs: map[*Sub]struct{}{}, gen: 3}
	a := &Sub{C: make(chan AU, 2), s: s, gen: 1}
	b := &Sub{C: make(chan AU, 2), s: s, gen: 2}
	a.C <- AU{Seq: 1}
	b.C <- AU{Seq: 2}
	s.subs[a] = struct{}{}
	s.subs[b] = struct{}{}

	s.mu.Lock()
	s.resetSubsForGenLocked()
	s.mu.Unlock()

	for name, sub := range map[string]*Sub{"a": a, "b": b} {
		if sub.gen != 3 || !sub.dropped || !sub.fresh {
			t.Fatalf("sub %s not reset for generation 3: gen=%d dropped=%t fresh=%t", name, sub.gen, sub.dropped, sub.fresh)
		}
		if len(sub.C) != 0 {
			t.Fatalf("sub %s retained stale AUs", name)
		}
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

func TestArgsBuildsMjpegFromStdin(t *testing.T) {
	s := New(Config{W: 1024, H: 768, FPS: 30, BitrateK: 6000, MaxrateK: 8000, BufsizeK: 1800})
	args := s.args()
	want := []string{
		"-loglevel", "warning",
		"-fflags", "nobuffer",
		"-f", "mjpeg", "-framerate", "30", "-probesize", "32", "-analyzeduration", "0",
		"-i", "pipe:0",
	}
	if len(args) < len(want) {
		t.Fatalf("args=%v, too short", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d]=%q, want %q (full: %v)", i, args[i], want[i], args)
		}
	}
	if args[len(args)-1] != "pipe:1" {
		t.Fatalf("last arg=%q, want pipe:1 (full: %v)", args[len(args)-1], args)
	}
}

func TestArgsPassesThroughUntimestampedFrames(t *testing.T) {
	s := New(Config{W: 640, H: 480, FPS: 30, BitrateK: 1000, MaxrateK: 1200, BufsizeK: 500})
	args := strings.Join(s.args(), " ")
	if !strings.Contains(args, "-fps_mode passthrough") {
		t.Fatalf("args must preserve every event-driven JPEG frame: %s", args)
	}
	if !strings.Contains(args, "-framerate 30") {
		t.Fatalf("args must timestamp input at configured FPS: %s", args)
	}
}

func TestArgsAddsScaleFilterWhenDownscaling(t *testing.T) {
	s := New(Config{W: 1024, H: 1024, ScaleMaxW: 512, ScaleMaxH: 512, FPS: 30, BitrateK: 100, MaxrateK: 100, BufsizeK: 50})
	args := s.args()
	found := false
	for i, a := range args {
		if a == "-vf" && i+1 < len(args) && strings.HasPrefix(args[i+1], "scale=512:512") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a scale=512:512 filter in args: %v", args)
	}
}

func TestPushIsNoopWhenNotRunning(t *testing.T) {
	s := New(Config{W: 64, H: 64, FPS: 15, BitrateK: 100, MaxrateK: 100, BufsizeK: 50})
	// Must not panic or block: no encoder has been started (no Subscribe
	// call), so Push has nowhere to write and should just return.
	s.Push([]byte("not a real jpeg"))
}

// TestPushStateNeverBlocksSender guards the actual bug this design exists to
// prevent: an earlier version wrote straight to ffmpeg's stdin from Push,
// which runs on the CDP event dispatch goroutine — confirmed live that a
// real interactive page (bigger, more frequent frames than any synthetic
// test used) filled the pipe and froze frame delivery entirely, JPEG lane
// included. pushState.set must stay O(1) regardless of how fast the writer
// goroutine drains it — here, not at all — by coalescing to the latest
// frame instead of blocking.
func TestPushStateNeverBlocksSender(t *testing.T) {
	ps := newPushState(30)
	// No writer goroutine is running at all: if set ever blocked on the
	// consumer, this would hang forever instead of finishing quickly.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100000; i++ {
			ps.set([]byte{byte(i)})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pushState.set blocked despite nothing ever draining it")
	}
}

func TestPushStatePacesWrites(t *testing.T) {
	ps := newPushState(100)
	w := &countingWriter{}
	go ps.run(w)
	defer close(ps.done)

	deadline := time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(deadline) {
		ps.set([]byte{1})
	}
	// Allow the final scheduled tick, but nowhere near enough time for an
	// unpaced writer to hide behind scheduler variance.
	time.Sleep(15 * time.Millisecond)
	got := w.n.Load()
	if got < 8 || got > 15 {
		t.Fatalf("paced writer made %d writes in about 135ms at 100fps, want 8..15", got)
	}
}

func TestSeedLatestRequiresMatchingCaptureSize(t *testing.T) {
	s := &Streamer{
		cfg:       Config{CaptureW: 800, CaptureH: 600},
		push:      newPushState(30),
		lastJPEG:  []byte{1, 2, 3},
		lastJPEGW: 800,
		lastJPEGH: 600,
	}
	s.seedLatestLocked()
	s.push.mu.Lock()
	got := append([]byte(nil), s.push.pending...)
	s.push.pending = nil
	s.push.mu.Unlock()
	if !bytes.Equal(got, s.lastJPEG) {
		t.Fatalf("matching restart seed = %v, want %v", got, s.lastJPEG)
	}

	s.cfg.CaptureW = 1024
	s.seedLatestLocked()
	s.push.mu.Lock()
	got = append([]byte(nil), s.push.pending...)
	s.push.mu.Unlock()
	if len(got) != 0 {
		t.Fatalf("size-changing restart reused stale JPEG: %v", got)
	}
}

// TestSubscribeForcesRestartWhenAlreadyRunning guards the fix for a real,
// live bug: a subscriber joining an already-running encoder (e.g. still
// alive within lingerStop's grace period from a previous viewer) started
// "dropped" waiting for an IDR, but nothing forced one — with scenecut=0
// the next periodic IDR was up to keyint frames away, and since frames
// only arrive when CDP's screencast actually produces one, a subscriber
// landing on an already-static page could wait forever, silently falling
// back to JPEG (confirmed live: "no AU within 6s, lane unavailable" after
// navigating a lingering session to a settled page). Subscribe must now
// force a restart whenever it joins a running encoder instead of trusting
// one to show up on its own.
func TestSubscribeForcesRestartWhenAlreadyRunning(t *testing.T) {
	s := New(Config{
		FFmpegPath: "surf-definitely-does-not-exist-xyz",
		W:          64, H: 64, FPS: 15, BitrateK: 100, MaxrateK: 100, BufsizeK: 50,
	})
	// Simulate an encoder that's already running (as if still lingering
	// from a prior subscriber) without spawning a real process.
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	sub := s.Subscribe()

	// The bad FFmpegPath makes the forced restart's startLocked fail
	// immediately — proving a restart was actually attempted (rather than
	// Subscribe trusting the already-"running" encoder to eventually
	// produce an IDR): running flips back to false and every subscriber,
	// including this brand new one, gets closed via failAllLocked.
	select {
	case _, ok := <-sub.C:
		if ok {
			t.Fatal("expected closed channel, got a value")
		}
	default:
		t.Fatal("expected sub.C already closed after the forced restart failed")
	}
	s.mu.Lock()
	running, lastReq := s.running, s.lastKeyframeReq
	s.mu.Unlock()
	if running {
		t.Fatal("streamer still marked running after the forced restart's startLocked failed")
	}
	if lastReq.IsZero() {
		t.Fatal("Subscribe did not record the forced keyframe restart attempt")
	}
}

// TestSubscribeFailsImmediatelyWhenFFmpegMissing guards the one remaining
// startup-failure path: mjpeg-from-stdin has no platform-specific
// "unsupported" case anymore (unlike the old x11grab/gdigrab CaptureArgs
// hook), so the only way Subscribe can fail now is ffmpeg itself not
// existing.
func TestSubscribeFailsImmediatelyWhenFFmpegMissing(t *testing.T) {
	s := New(Config{
		FFmpegPath: "surf-definitely-does-not-exist-xyz",
		W:          64, H: 64, FPS: 15, BitrateK: 100, MaxrateK: 100, BufsizeK: 50,
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
