// Package stream owns the H.264 lane: it transcodes the JPEG frames
// Chromium's own CDP screencast already produces (pushed in via
// Streamer.Push — see browser.go's onScreencastFrame) into H.264 through
// one ffmpeg process (JPEG in over stdin, Annex-B H.264 out), an
// access-unit splitter, and per-subscriber fan-out with IDR-aware
// backpressure.
//
// This used to grab the OS display directly (x11grab/gdigrab), bypassing
// CDP entirely. Re-encoding frames CDP already delivers instead means this
// package needs no platform-specific capture method at all — the same
// ffmpeg invocation works on every OS, and it works even when Chromium runs
// on a Windows hidden desktop, which OS-level screen capture cannot reach:
// confirmed live that gdigrab fails with ERROR_ACCESS_DENIED against a
// desktop that has never been the foreground one, independent of its DACL.
//
// The encoder runs only while at least one subscriber exists (plus a short
// linger), so the lane costs nothing when no native video client is
// connected.
package stream

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"surf-backend/internal/proc"
)

const (
	x264Speed = "ultrafast"

	// subQueueCap bounds how far a slow subscriber may lag before it is
	// dropped to the next IDR (~2s of AUs at 15fps).
	subQueueCap = 30
	// lingerStop delays encoder shutdown after the last unsubscribe so quick
	// reconnects don't cycle the process.
	lingerStop = 5 * time.Second
	// maxAUBytes caps the splitter's assembly buffer; a runaway means we lost
	// sync with the byte stream and the process is restarted.
	maxAUBytes = 4 << 20
	// keyframeCooldown bounds how often an on-demand keyframe request may
	// restart the encoder — a resync storm from a struggling client must not
	// out-pace the process it's trying to shortcut.
	keyframeCooldown = 2 * time.Second
)

type Config struct {
	FFmpegPath string
	Env        []string
	W, H       int // coded size
	// CaptureW, CaptureH is the size of the JPEG frames being pushed in —
	// Chromium's screencast maxWidth/maxHeight, kept in sync with the
	// client's viewport by browser.go. Defaults to the coded size.
	CaptureW, CaptureH   int
	ScaleMaxW, ScaleMaxH int // optional coded-size bounding box
	// FPS is nominal now, not a literal capture poll rate (frames arrive
	// whenever CDP's screencast produces one, not on a fixed clock): used
	// only for the keyint/level math below.
	FPS                          int
	BitrateK, MaxrateK, BufsizeK int
}

// AU is one complete Annex-B access unit (start codes intact). W/H are the
// coded size of the encoder run that produced it — they change when the
// screen is resized mid-subscription.
type AU struct {
	Data []byte
	IDR  bool
	Seq  uint32
	W, H int
	// T is when this AU was fully assembled (readLoop), before fan-out. Not a
	// true capture timestamp — ffmpeg is an opaque subprocess with no internal
	// timing hook — but it's the earliest point our own code can observe the
	// frame, so T -> SendBinary directly measures per-subscriber queueing delay.
	T time.Time
}

// Sub is one subscriber's view of the stream. Read AUs from C; a closed C
// means the lane died (encoder gave up) — fall back to JPEG.
type Sub struct {
	C chan AU

	s       *Streamer
	mu      sync.Mutex
	dropped bool // dropping until the next IDR
	closed  bool
	fresh   bool // never delivered anything yet: wait for an IDR to start
	gen     int  // encoder generation this subscriber accepts
}

type Streamer struct {
	cfg Config

	mu        sync.Mutex
	subs      map[*Sub]struct{}
	cmd       *os.Process
	stdin     io.WriteCloser
	push      *pushState // coalescing mailbox for Push; nil when not running
	lastJPEG  []byte     // latest immutable input, used to seed same-size restarts
	lastJPEGW int
	lastJPEGH int
	stdout    io.ReadCloser
	running   bool
	gen       int // process generation, guards stale readLoops
	seq       uint32
	stopTimer *time.Timer
	crashes   []time.Time // recent crash timestamps

	lastKeyframeReq time.Time
}

// pushState is a single-slot coalescing mailbox between Push (called on the
// CDP event dispatch goroutine — must never block) and one writer goroutine
// that owns the actual stdin.Write calls. Only the latest not-yet-written
// frame is kept, same trade-off ws.Client.queue makes for the WebSocket
// outbox: if the encoder falls behind, dropping stale frames is correct
// for a live feed, and a live feed is all this ever carries.
type pushState struct {
	mu      sync.Mutex
	pending []byte
	done    chan struct{} // closed once, by stopLocked, to stop the writer
	period  time.Duration
}

func newPushState(fps int) *pushState {
	if fps < 1 {
		fps = 30
	}
	return &pushState{done: make(chan struct{}), period: time.Second / time.Duration(fps)}
}

func (ps *pushState) set(data []byte) {
	ps.mu.Lock()
	ps.pending = data
	ps.mu.Unlock()
}

// run is the dedicated paced writer goroutine: the only thing that ever
// calls w.Write, so a stuck/slow ffmpeg blocks this goroutine alone, never
// Push's caller. It samples the latest pending frame at the configured FPS.
// Chromium can emit screencast frames at the compositor's 60Hz rate; passing
// all of them through overloaded the target iPad even though the stream was
// advertised as 30fps (confirmed live: 300 AUs in a 5s perf window). Pacing
// here makes FPS real while preserving the latest-frame-wins behavior.
func (ps *pushState) run(w io.Writer) {
	ticker := time.NewTicker(ps.period)
	defer ticker.Stop()
	for {
		select {
		case <-ps.done:
			return
		case <-ticker.C:
			ps.mu.Lock()
			data := ps.pending
			ps.pending = nil
			ps.mu.Unlock()
			if len(data) == 0 {
				continue
			}
			if _, err := w.Write(data); err != nil {
				return
			}
		}
	}
}

func New(cfg Config) *Streamer {
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.CaptureW == 0 || cfg.CaptureH == 0 {
		cfg.CaptureW, cfg.CaptureH = cfg.W, cfg.H
	}
	cfg.W, cfg.H = cfg.codedSize(cfg.CaptureW, cfg.CaptureH)
	return &Streamer{cfg: cfg, subs: map[*Sub]struct{}{}}
}

func (s *Streamer) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Push feeds one JPEG frame — straight from Chromium's own CDP screencast —
// into the encoder if one is running; a no-op otherwise (no subscriber has
// started the lane, or it's between a restart).
//
// Never blocks the caller: this runs on the CDP event dispatch goroutine,
// the same one that delivers every other event including the JPEG lane
// itself, so blocking here would freeze the whole browser, not just video
// (confirmed live: an earlier version wrote straight to ffmpeg's stdin, and
// a real interactive page — bigger, more frequent frames than any synthetic
// test used — filled the pipe and froze frame delivery entirely). The frame
// is handed to a small coalescing mailbox; a dedicated writer goroutine
// drains it into ffmpeg's stdin, so a slow/stuck encoder just means the
// next Push replaces the still-unwritten frame instead of blocking anyone.
func (s *Streamer) Push(jpeg []byte) {
	if len(jpeg) == 0 {
		return
	}
	s.mu.Lock()
	s.lastJPEG = jpeg
	s.lastJPEGW, s.lastJPEGH = s.cfg.CaptureW, s.cfg.CaptureH
	ps := s.push
	s.mu.Unlock()
	if ps == nil {
		return
	}
	ps.set(jpeg)
}

// SetSize retargets the encoder at a new viewport size (CaptureW/H — the
// size Chromium's screencast now delivers). The encoder is restarted so
// x264 gets a coded size matching the new incoming frames, optionally
// scaled down first so old clients decode fewer pixels without cropping
// the page.
func (s *Streamer) SetSize(w, h int) {
	if w < 64 || h < 64 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cw, ch := s.cfg.codedSize(w, h)
	if w == s.cfg.CaptureW && h == s.cfg.CaptureH && cw == s.cfg.W && ch == s.cfg.H {
		return
	}
	s.cfg.CaptureW, s.cfg.CaptureH = w, h
	s.cfg.W, s.cfg.H = cw, ch
	if s.running {
		log.Printf("stream: resizing encoder capture=%dx%d coded=%dx%d", w, h, cw, ch)
		s.stopLocked()
		s.startLocked()
	}
	s.resetSubsForGenLocked()
}

func even(v int) int {
	if v < 2 {
		return 2
	}
	return v &^ 1
}

func (c Config) codedSize(w, h int) (int, int) {
	if c.ScaleMaxW < 64 || c.ScaleMaxH < 64 {
		return even(w), even(h)
	}
	s := min(float64(c.ScaleMaxW)/float64(w), float64(c.ScaleMaxH)/float64(h))
	if s >= 1 {
		return even(w), even(h)
	}
	return even(int(float64(w)*s + 0.5)), even(int(float64(h)*s + 0.5))
}

// Subscribe registers a consumer and starts the encoder if it isn't running.
// The subscriber starts in "wait for IDR" state, so the first AU it sees is
// always independently decodable.
func (s *Streamer) Subscribe() *Sub {
	sub := &Sub{C: make(chan AU, subQueueCap), s: s, fresh: true, dropped: true}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs[sub] = struct{}{}
	if s.stopTimer != nil {
		s.stopTimer.Stop()
		s.stopTimer = nil
	}
	if !s.running {
		s.startLocked()
	} else {
		// Joining an already-running encoder: this subscriber starts
		// "dropped" and needs an IDR to begin, but with scenecut=0 the next
		// one is only guaranteed every keyint encoded frames — and frames
		// now only arrive when CDP's screencast actually produces one, not
		// on a fixed clock. A subscriber landing on an already-static page
		// (e.g. lingerStop kept a prior viewer's encoder alive and this is
		// a quick reconnect) could then wait out firstAUWait for an IDR
		// that never comes on its own, and silently fall back to JPEG
		// (confirmed live). Restarting is the only way to force one
		// (see RequestKeyframe) — the same already-accepted tradeoff of
		// briefly disrupting any other subscriber, for a product built
		// around a single active viewer.
		s.lastKeyframeReq = time.Now()
		s.stopLocked()
		s.startLocked()
	}
	// A restart changes the accepted generation for every existing
	// subscriber, not just the newcomer. Leaving older subscribers on the
	// previous generation silently strands them because offer rejects every
	// subsequent AU.
	s.resetSubsForGenLocked()
	return sub
}

// ForceResync makes the subscriber skip AUs until the next IDR — used when
// the websocket outbox dropped a frame, which poisons every P-frame after it.
func (sub *Sub) ForceResync() {
	sub.mu.Lock()
	sub.dropped = true
	sub.mu.Unlock()
}

// RequestKeyframe restarts the encoder so every subscriber's next AU is an
// IDR — the only available "force a keyframe" primitive: ffmpeg's stdin now
// carries the JPEG frame stream itself, not a control channel, so there's
// no way to ask a running x264 for an early IDR without restarting it. A
// freshly spawned x264 process's first output frame is always an IDR
// regardless of keyint. This is a single-active-viewer product, so a
// restart affecting every subscriber is an acceptable, already-precedented
// tradeoff — SetSize restarts the encoder for the same reason.
// Cooldown-guarded so a resync storm can't thrash the process.
func (s *Streamer) RequestKeyframe() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return // nothing to restart; a fresh Subscribe already waits for an IDR
	}
	now := time.Now()
	if now.Sub(s.lastKeyframeReq) < keyframeCooldown {
		return
	}
	s.lastKeyframeReq = now
	log.Printf("stream: keyframe requested, restarting encoder")
	s.stopLocked()
	s.startLocked()
	s.resetSubsForGenLocked()
}

// seedLatestLocked gives a same-size restart an immediate first frame. CDP's
// screencast is change-driven, so a static page may never emit another frame
// after a keyframe request, reconnect, or crash recovery. s.mu held by caller.
func (s *Streamer) seedLatestLocked() {
	if s.push != nil && len(s.lastJPEG) > 0 &&
		s.lastJPEGW == s.cfg.CaptureW && s.lastJPEGH == s.cfg.CaptureH {
		s.push.set(s.lastJPEG)
	}
}

// resetSubsForGenLocked moves every subscriber onto the current encoder
// generation and drains AUs from the superseded process. s.mu held by caller.
func (s *Streamer) resetSubsForGenLocked() {
	for sub := range s.subs {
		sub.resetForGen(s.gen)
	}
}

func (sub *Sub) resetForGen(gen int) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	sub.gen = gen
	sub.dropped = true
	sub.fresh = true
	for {
		select {
		case <-sub.C:
		default:
			return
		}
	}
}

func (sub *Sub) Close() {
	s := sub.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub]; !ok {
		return
	}
	delete(s.subs, sub)
	sub.mu.Lock()
	if !sub.closed {
		sub.closed = true
		close(sub.C)
	}
	sub.mu.Unlock()
	if len(s.subs) == 0 && s.running {
		s.stopTimer = time.AfterFunc(lingerStop, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if len(s.subs) == 0 && s.running {
				log.Printf("stream: idle, stopping encoder")
				s.stopLocked()
			}
		})
	}
}

// offer delivers one AU without ever blocking the splitter. Channel full →
// drop this and everything until the next IDR, then resume from that IDR.
func (sub *Sub) offer(au AU, gen int) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed || gen != sub.gen {
		return
	}
	if sub.dropped && !au.IDR {
		return
	}
	select {
	case sub.C <- au:
		if sub.dropped && !sub.fresh {
			log.Printf("stream: sub resynced at seq=%d", au.Seq)
		}
		sub.dropped = false
		sub.fresh = false
	default:
		if !sub.dropped {
			log.Printf("stream: sub lagging, dropping to next IDR (seq=%d)", au.Seq)
		}
		sub.dropped = true
	}
}

// args builds the ffmpeg command line: decode JPEG frames pushed via stdin
// and re-encode as H.264. Fixed and platform-independent — no OS-level
// capture involved at all.
func (s *Streamer) args() []string {
	c := s.cfg
	keyint := 2 * c.FPS
	args := []string{
		"-loglevel", "warning",
		// No input buffering/probing: every ms of demuxer buffer is
		// glass-to-glass latency (carried over from the old x11grab args,
		// still true for a live stdin stream).
		"-fflags", "nobuffer",
		"-f", "mjpeg", "-framerate", fmt.Sprintf("%d", c.FPS),
		"-probesize", "32", "-analyzeduration", "0",
		"-i", "pipe:0",
	}
	if c.CaptureW != c.W || c.CaptureH != c.H {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear", c.W, c.H))
	}
	args = append(args,
		// CDP's event-driven JPEG frames carry no timestamps. FFmpeg's
		// default output sync treats their repeated/absent PTS as duplicate
		// frames and drops nearly all of them (confirmed live: 60 complete
		// JPEG writes produced two encoded frames, with verbose logging
		// reporting "dropping frame ... at ts 0"). Passthrough makes this a
		// strict one-input-frame -> one-output-frame transcoder; timing is
		// intentionally owned by arrival order, not a synthetic media clock.
		"-fps_mode", "passthrough",
		"-c:v", "libx264",
		"-profile:v", "baseline", "-level", c.h264Level(),
		"-preset", x264Speed,
		"-tune", "zerolatency",
		"-x264-params", fmt.Sprintf("keyint=%d:min-keyint=%d:scenecut=0:repeat-headers=1:aud=1", keyint, keyint),
		"-b:v", fmt.Sprintf("%dk", c.BitrateK),
		"-maxrate", fmt.Sprintf("%dk", c.MaxrateK),
		"-bufsize", fmt.Sprintf("%dk", c.BufsizeK),
		"-pix_fmt", "yuv420p",
		"-f", "h264", "pipe:1",
	)
	return args
}

func (c Config) h264Level() string {
	if c.FPS > 30 || macroblocksPerSecond(c.W, c.H, c.FPS) > 108000 {
		return "4.1"
	}
	return "3.1"
}

func macroblocksPerSecond(w, h, fps int) int {
	mbW := (w + 15) / 16
	mbH := (h + 15) / 16
	return mbW * mbH * fps
}

// startLocked launches ffmpeg; s.mu held by caller.
func (s *Streamer) startLocked() {
	started, err := proc.Start(s.cfg.FFmpegPath, s.args(), proc.Options{
		Env:    append(os.Environ(), s.cfg.Env...),
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		log.Printf("stream: ffmpeg start failed: %v", err)
		s.failAllLocked()
		return
	}
	s.cmd = started.Process
	s.stdin = started.Stdin
	s.stdout = started.Stdout
	s.push = newPushState(s.cfg.FPS)
	go s.push.run(started.Stdin)
	s.running = true
	s.gen++
	s.seedLatestLocked()
	gen := s.gen
	log.Printf("stream: encoder started pid=%d coded=%dx%d@%dfps %dk x264=%s",
		started.Process.Pid, s.cfg.W, s.cfg.H, s.cfg.FPS, s.cfg.BitrateK, x264Speed)
	if started.Stderr != nil {
		go logStderr(started.Stderr)
	}
	go s.readLoop(started.Stdout, started.Process, gen)
}

func logStderr(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, line := range strings.Split(strings.TrimSpace(string(buf[:n])), "\n") {
				if line != "" {
					log.Printf("stream/ffmpeg: %s", line)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// stopLocked kills the encoder; s.mu held by caller.
func (s *Streamer) stopLocked() {
	if s.push != nil {
		close(s.push.done)
		s.push = nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil {
		proc.Kill(s.cmd.Pid)
	}
	s.running = false
	s.cmd = nil
	s.stdin = nil
	s.stdout = nil
}

// cleanupExitedLocked releases pipes/mailbox state after Wait reports an
// unexpected process exit, without trying to kill the already-dead process.
// s.mu held by caller.
func (s *Streamer) cleanupExitedLocked() {
	if s.push != nil {
		close(s.push.done)
		s.push = nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.stdout != nil {
		_ = s.stdout.Close()
	}
	s.running = false
	s.cmd = nil
	s.stdin = nil
	s.stdout = nil
}

// Shutdown stops everything; server exit path.
func (s *Streamer) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	s.failAllLocked()
}

// failAllLocked closes every subscriber channel (lane is dead); s.mu held.
func (s *Streamer) failAllLocked() {
	for sub := range s.subs {
		sub.mu.Lock()
		if !sub.closed {
			sub.closed = true
			close(sub.C)
		}
		sub.mu.Unlock()
		delete(s.subs, sub)
	}
}

// readLoop splits ffmpeg's Annex-B output into AUs and fans them out.
func (s *Streamer) readLoop(r io.Reader, cmd *os.Process, gen int) {
	sp := newAUSplitter()
	buf := make([]byte, 64<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			aus, splitErr := sp.feed(buf[:n])
			if splitErr != nil {
				log.Printf("stream: splitter error: %v (restarting encoder)", splitErr)
				break
			}
			now := time.Now()
			for _, au := range aus {
				au.T = now
				s.broadcast(au, gen)
			}
		}
		if err != nil {
			break
		}
	}
	_, _ = cmd.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.gen || !s.running {
		return // superseded by a restart or an intentional stop
	}
	s.cleanupExitedLocked()
	if len(s.subs) == 0 {
		return
	}
	// Crash with subscribers attached: restart once, but give up if it keeps
	// dying (two crashes within a minute).
	now := time.Now()
	recent := s.crashes[:0]
	for _, t := range s.crashes {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	s.crashes = append(recent, now)
	if len(s.crashes) >= 2 {
		log.Printf("stream: encoder crashed twice within a minute, giving up")
		s.failAllLocked()
		return
	}
	log.Printf("stream: encoder died with subscribers attached, restarting in 2s")
	time.AfterFunc(2*time.Second, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.running && len(s.subs) > 0 {
			s.startLocked()
			s.resetSubsForGenLocked()
		}
	})
}

func (s *Streamer) broadcast(au AU, gen int) {
	s.mu.Lock()
	if gen != s.gen || !s.running {
		s.mu.Unlock()
		return
	}
	s.seq++
	au.Seq = s.seq
	au.W, au.H = s.cfg.W, s.cfg.H
	targets := make([]*Sub, 0, len(s.subs))
	for sub := range s.subs {
		targets = append(targets, sub)
	}
	s.mu.Unlock()
	for _, sub := range targets {
		sub.offer(au, gen)
	}
}
