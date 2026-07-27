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
	s := &VideoPipeline{subs: map[*Sub]struct{}{}, gen: 3}
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
	s := &VideoPipeline{subs: map[*Sub]struct{}{}}
	s.RequestKeyframe()
	if s.running {
		t.Fatal("RequestKeyframe started the encoder while not running")
	}
	if !s.lastKeyframeReq.IsZero() {
		t.Fatal("RequestKeyframe touched lastKeyframeReq on a no-op call")
	}
}

func TestRequestKeyframeCooldown(t *testing.T) {
	s := &VideoPipeline{subs: map[*Sub]struct{}{}, running: true}
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
	args := s.args("rtp://127.0.0.1:1234")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-loglevel warning", "-f image2pipe", "-vcodec mjpeg",
		"-framerate 30", "-probesize 32",
		"-analyzeduration 0", "-i pipe:0",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
	if args[len(args)-1] != "rtp://127.0.0.1:1234" {
		t.Fatalf("last arg=%q, want RTP URL (full: %v)", args[len(args)-1], args)
	}
}

func TestArgsPassesThroughUntimestampedFrames(t *testing.T) {
	s := New(Config{W: 640, H: 480, FPS: 30, BitrateK: 1000, MaxrateK: 1200, BufsizeK: 500})
	args := strings.Join(s.args("rtp://127.0.0.1:1234"), " ")
	if !strings.Contains(args, "-fps_mode passthrough") {
		t.Fatalf("args must preserve every event-driven JPEG frame: %s", args)
	}
	if !strings.Contains(args, "-framerate 30") {
		t.Fatalf("args must timestamp input at configured FPS: %s", args)
	}
}

func TestArgsAddsScaleFilterWhenDownscaling(t *testing.T) {
	s := New(Config{W: 1024, H: 1024, ScaleMaxW: 512, ScaleMaxH: 512, FPS: 30, BitrateK: 100, MaxrateK: 100, BufsizeK: 50})
	args := s.args("rtp://127.0.0.1:1234")
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
// test used) filled the pipe and froze frame delivery entirely. pushState.set
// must stay O(1) regardless of how fast the writer
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
	s := &VideoPipeline{
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
// landing on an already-static page could wait forever and become unavailable
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

// TestSubscribeFailsImmediatelyWhenFFmpegMissing guards encoder startup
// failure when the configured executable does not exist.
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
