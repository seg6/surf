// Package stream owns the H.264 lane: it transcodes the JPEG frames
// Chromium's own CDP screencast already produces (pushed in via
// VideoPipeline.Push — see browser.go's onScreencastFrame) into H.264 through
// one ffmpeg process (JPEG in over stdin, H.264/RTP over loopback), an RTP
// depacketizer, and per-subscriber fan-out with IDR-aware
// backpressure.
//
// Re-encoding frames CDP already delivers means this package needs no
// platform-specific capture method; the same invocation works on every OS.
//
// The encoder runs only while at least one subscriber exists (plus a short
// linger), so the lane costs nothing when no native video client is
// connected.
package stream

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"surf-backend/internal/proc"
	"surf-backend/internal/telemetry"
)

const (
	x264Speed = "ultrafast"
	// Metadata older than this cannot describe a freshly emitted AU. Some
	// FFmpeg builds retain an input timestamp across an idle/static interval
	// even though the RTP payload is current. Reporting that as a multi-second
	// encode or interaction latency is worse than explicitly marking the
	// correlation unknown.
	maxCorrelationAge = 500 * time.Millisecond

	// The WebSocket layer already has a four-AU GOP-aware queue. Keeping the
	// upstream fan-out equally small ensures no second, hidden reservoir can
	// turn temporary scheduler pressure into visible seconds of latency.
	subQueueCap = 4
	// lingerStop delays encoder shutdown after the last unsubscribe so quick
	// reconnects don't cycle the process.
	lingerStop = 5 * time.Second
	// maxAUBytes bounds one RTP-reassembled access unit.
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
	T                time.Time
	Generation       uint32
	SourceSeq        uint32
	InteractionID    uint64
	SourceReceiveNS  uint64
	EncodeCompleteNS uint64
}

func (s *VideoPipeline) Generation() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return uint32(s.gen)
}

// ResetCrashBudget is the explicit operator/client retry boundary.
func (s *VideoPipeline) ResetCrashBudget() {
	s.mu.Lock()
	s.crashes = nil
	s.mu.Unlock()
}

// Sub is one subscriber's view of the stream. A closed C means the encoder
// became unavailable.
type Sub struct {
	C chan AU

	s       *VideoPipeline
	mu      sync.Mutex
	dropped bool // dropping until the next IDR
	closed  bool
	fresh   bool // never delivered anything yet: wait for an IDR to start
	gen     int  // encoder generation this subscriber accepts
}

type VideoPipeline struct {
	cfg Config

	mu        sync.Mutex
	subs      map[*Sub]struct{}
	cmd       *os.Process
	stdin     io.WriteCloser
	push      *pushState // coalescing mailbox for Push; nil when not running
	lastJPEG  []byte     // latest immutable input, used to seed same-size restarts
	lastJPEGW int
	lastJPEGH int
	rtpConn   *net.UDPConn
	running   bool
	gen       int // process generation, guards stale readLoops
	seq       uint32
	stopTimer *time.Timer
	crashes   []time.Time // recent crash timestamps

	lastKeyframeReq time.Time

	sourceReplacements atomic.Uint64
	mappingFailures    atomic.Uint64
	rtpErrors          atomic.Uint64
	accessUnits        atomic.Uint64
}

// pushState is a single-slot coalescing mailbox between Push (called on the
// CDP event dispatch goroutine — must never block) and one writer goroutine
// that owns the actual stdin.Write calls. Only the latest not-yet-written
// frame is kept, the same latest-wins trade-off as the WebSocket video queue
// outbox: if the encoder falls behind, dropping stale frames is correct
// for a live feed, and a live feed is all this ever carries.
type pushState struct {
	mu          sync.Mutex
	pending     []byte
	pendingMeta inputMeta
	repeats     int
	done        chan struct{}  // closed once, by stopLocked, to stop the writer
	wake        chan struct{}  // latest frame changed
	written     chan inputMeta // metadata for each successful FFmpeg input
	period      time.Duration
}

type inputMeta struct {
	sourceNS    uint64
	sourceSeq   uint32
	interaction uint64
}

func newPushState(fps int) *pushState {
	if fps < 1 {
		fps = 30
	}
	return &pushState{
		done: make(chan struct{}), wake: make(chan struct{}, 1),
		written: make(chan inputMeta, 256),
		period:  time.Second / time.Duration(fps),
	}
}

func (ps *pushState) set(data []byte) {
	ps.setMeta(data, 0, 0, 1)
}

func (ps *pushState) setRepeats(data []byte, repeats int) {
	ps.setMeta(data, 0, 0, repeats)
}

func (ps *pushState) setMeta(data []byte, sourceSeq uint32, interaction uint64, repeats int) bool {
	if repeats < 1 {
		repeats = 1
	}
	ps.mu.Lock()
	replaced := len(ps.pending) != 0
	ps.pending = data
	ps.pendingMeta = inputMeta{sourceNS: telemetry.MonoNS(), sourceSeq: sourceSeq, interaction: interaction}
	ps.repeats = repeats
	ps.mu.Unlock()
	select {
	case ps.wake <- struct{}{}:
	default:
	}
	return replaced
}

// run is the dedicated paced writer goroutine: the only thing that ever
// calls w.Write, so a stuck/slow ffmpeg blocks this goroutine alone, never
// Push's caller. A frame is written immediately when the rate-limit window is
// open; only excess frames wait for the next slot and coalesce latest-wins.
// Chromium can emit screencast frames at the compositor's 60Hz rate; passing
// all of them through overloaded the target iPad even though the stream was
// advertised as 30fps (confirmed live: 300 AUs in a 5s perf window). The old
// free-running ticker added an avoidable 0–33ms to every frame depending on
// phase; this event-driven limiter preserves the cap without that latency.
func (ps *pushState) run(w io.Writer) {
	var timer *time.Timer
	var timerC <-chan time.Time
	var lastWrite time.Time
	for {
		select {
		case <-ps.done:
			if timer != nil {
				timer.Stop()
			}
			return
		case <-ps.wake:
		case <-timerC:
			timerC = nil
		}

		if !lastWrite.IsZero() {
			if delay := time.Until(lastWrite.Add(ps.period)); delay > 0 {
				if timerC == nil {
					if timer == nil {
						timer = time.NewTimer(delay)
					} else {
						timer.Reset(delay)
					}
					timerC = timer.C
				}
				continue
			}
		}

		ps.mu.Lock()
		data := ps.pending
		meta := ps.pendingMeta
		if ps.repeats > 1 {
			ps.repeats--
		} else {
			ps.pending = nil
			ps.repeats = 0
		}
		ps.mu.Unlock()
		if len(data) == 0 {
			continue
		}
		// Publish correlation before the pipe write: FFmpeg can consume the
		// JPEG and emit localhost RTP on another CPU before Write returns.
		// Publishing afterward created a permanent one-frame FIFO shift and
		// bogus multi-second source→encode spikes. The bounded channel may
		// backpressure only after 256 inputs without any output, which is an
		// unhealthy encoder rather than a live path worth feeding further.
		select {
		case ps.written <- meta:
		case <-ps.done:
			return
		}
		if _, err := w.Write(data); err != nil {
			return
		}
		lastWrite = time.Now()
		ps.mu.Lock()
		more := len(ps.pending) != 0
		ps.mu.Unlock()
		if more {
			select {
			case ps.wake <- struct{}{}:
			default:
			}
		}
	}
}

func New(cfg Config) *VideoPipeline {
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.CaptureW == 0 || cfg.CaptureH == 0 {
		cfg.CaptureW, cfg.CaptureH = cfg.W, cfg.H
	}
	cfg.W, cfg.H = cfg.codedSize(cfg.CaptureW, cfg.CaptureH)
	return &VideoPipeline{cfg: cfg, subs: map[*Sub]struct{}{}}
}

func (s *VideoPipeline) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *VideoPipeline) Stats() map[string]uint64 {
	return map[string]uint64{
		"source_mailbox_replacements": s.sourceReplacements.Load(),
		"source_au_mapping_failures":  s.mappingFailures.Load(),
		"rtp_errors":                  s.rtpErrors.Load(),
		"video_access_units":          s.accessUnits.Load(),
	}
}

// Push feeds one JPEG frame — straight from Chromium's own CDP screencast —
// into the encoder if one is running; a no-op otherwise (no subscriber has
// started the lane, or it's between a restart).
//
// Never blocks the caller: this runs on the CDP event dispatch goroutine,
// the same one that delivers every other CDP event, so blocking here would
// freeze the whole browser, not just video
// (confirmed live: an earlier version wrote straight to ffmpeg's stdin, and
// a real interactive page — bigger, more frequent frames than any synthetic
// test used — filled the pipe and froze frame delivery entirely). The frame
// is handed to a small coalescing mailbox; a dedicated writer goroutine
// drains it into ffmpeg's stdin, so a slow/stuck encoder just means the
// next Push replaces the still-unwritten frame instead of blocking anyone.
func (s *VideoPipeline) Push(jpeg []byte) {
	s.PushSource(jpeg, 0, 0)
}

// PushSource carries the source frame's causal metadata through the encoder.
func (s *VideoPipeline) PushSource(jpeg []byte, sourceSeq uint32, interactionID uint64) {
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
	if ps.setMeta(jpeg, sourceSeq, interactionID, 1) {
		s.sourceReplacements.Add(1)
	}
}

// SwitchSource starts the next GOP from an image belonging to a newly active
// tab. The wire stream remains the same; restarting x264 is how this stdin
// based encoder can request an immediate IDR instead of waiting up to the
// normal two-second GOP boundary.
func (s *VideoPipeline) SwitchSource(jpeg []byte, sourceSeq uint32, interactionID uint64) {
	if len(jpeg) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastJPEG = jpeg
	s.lastJPEGW, s.lastJPEGH = s.cfg.CaptureW, s.cfg.CaptureH
	if !s.running {
		return
	}
	log.Printf("stream: new tab frame, starting immediate IDR")
	s.stopLocked()
	s.startLocked()
	s.resetSubsForGenLocked()
	if s.push != nil {
		s.push.setMeta(jpeg, sourceSeq, interactionID, 3)
	}
}

// PushBootstrap repeats an immutable still frame a few times. FFmpeg's MJPEG
// demuxer needs more than one sample to finish probing on some versions, while
// CDP screencasting is change-driven and may never emit a second static frame.
func (s *VideoPipeline) PushBootstrap(jpeg []byte) {
	if len(jpeg) == 0 {
		return
	}
	s.mu.Lock()
	s.lastJPEG = jpeg
	s.lastJPEGW, s.lastJPEGH = s.cfg.CaptureW, s.cfg.CaptureH
	ps := s.push
	s.mu.Unlock()
	if ps != nil {
		ps.setRepeats(jpeg, 3)
	}
}

// SetSize retargets the encoder at a new viewport size (CaptureW/H — the
// size Chromium's screencast now delivers). The encoder is restarted so
// x264 gets a coded size matching the new incoming frames, optionally
// scaled down first so old clients decode fewer pixels without cropping
// the page.
func (s *VideoPipeline) SetSize(w, h int) {
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
func (s *VideoPipeline) Subscribe() *Sub {
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
		// that never comes on its own, and report unavailable. Restarting is
		// the only way to force one
		// (see RequestKeyframe) — the same already-accepted tradeoff of
		// briefly disrupting any other subscriber, for a product built
		// around a single active viewer.
		// A concurrent watchdog/keyframe request may already have started a
		// fresh generation. Do not immediately kill that process and discard
		// its priming frames; only force a restart when the running generation
		// is not already inside the cooldown window.
		if time.Since(s.lastKeyframeReq) >= keyframeCooldown {
			s.lastKeyframeReq = time.Now()
			s.stopLocked()
			s.startLocked()
		}
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
func (s *VideoPipeline) RequestKeyframe() {
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
func (s *VideoPipeline) seedLatestLocked() {
	if s.push != nil && len(s.lastJPEG) > 0 &&
		s.lastJPEGW == s.cfg.CaptureW && s.lastJPEGH == s.cfg.CaptureH {
		s.push.setRepeats(s.lastJPEG, 3)
	}
}

// resetSubsForGenLocked moves every subscriber onto the current encoder
// generation and drains AUs from the superseded process. s.mu held by caller.
func (s *VideoPipeline) resetSubsForGenLocked() {
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

// offer delivers one AU without ever blocking the RTP reader. Channel full →
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
func (s *VideoPipeline) args(outputURL string) []string {
	c := s.cfg
	keyint := 2 * c.FPS
	args := []string{
		"-loglevel", "warning",
		// Each CDP payload is one complete JPEG image. image2pipe models that
		// directly; the mjpeg demuxer treats stdin like a network MJPEG stream
		// and was measured buffering multiple frames before decode.
		"-f", "image2pipe", "-vcodec", "mjpeg",
		"-framerate", fmt.Sprintf("%d", c.FPS),
		"-probesize", "32", "-analyzeduration", "0",
		"-threads", "0",
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
		// Let FFmpeg/x264 size its worker pool for the host. This is necessary
		// for 60fps on smaller VPS cores; client diagnostics remain the guard
		// against choosing a resolution the iPad cannot decode in time.
		"-threads", "0",
		"-profile:v", "baseline", "-level", c.h264Level(),
		"-preset", x264Speed,
		"-tune", "zerolatency",
		"-x264-params", fmt.Sprintf("keyint=%d:min-keyint=%d:scenecut=0:repeat-headers=1:aud=1", keyint, keyint),
		"-b:v", fmt.Sprintf("%dk", c.BitrateK),
		"-maxrate", fmt.Sprintf("%dk", c.MaxrateK),
		"-bufsize", fmt.Sprintf("%dk", c.BufsizeK),
		"-pix_fmt", "yuv420p",
		// RTP's marker bit gives us the exact end of an access unit. Reading
		// raw Annex-B from stdout required waiting for the next AUD before
		// releasing the current frame, adding a full frame (or indefinitely
		// on a static page) to the live path.
		"-flush_packets", "1",
		"-muxdelay", "0",
		"-f", "rtp", "-payload_type", "96", outputURL,
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
func (s *VideoPipeline) startLocked() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		log.Printf("stream: RTP listener failed: %v", err)
		s.failAllLocked()
		return
	}
	_ = conn.SetReadBuffer(4 << 20)
	port := conn.LocalAddr().(*net.UDPAddr).Port
	outputURL := fmt.Sprintf("rtp://127.0.0.1:%d?pkt_size=1200", port)
	started, err := proc.Start(s.cfg.FFmpegPath, s.args(outputURL), proc.Options{
		Env:    append(os.Environ(), s.cfg.Env...),
		Stdin:  true,
		Stderr: true,
	})
	if err != nil {
		_ = conn.Close()
		log.Printf("stream: ffmpeg start failed: %v", err)
		s.failAllLocked()
		return
	}
	s.cmd = started.Process
	s.stdin = started.Stdin
	s.rtpConn = conn
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
	go s.rtpReadLoop(conn, gen, s.push.written)
	go s.waitLoop(started.Process, gen)
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
func (s *VideoPipeline) stopLocked() {
	if s.push != nil {
		close(s.push.done)
		s.push = nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.rtpConn != nil {
		_ = s.rtpConn.Close()
	}
	if s.cmd != nil {
		proc.Kill(s.cmd.Pid)
	}
	s.running = false
	s.cmd = nil
	s.stdin = nil
	s.rtpConn = nil
}

// cleanupExitedLocked releases pipes/mailbox state after Wait reports an
// unexpected process exit, without trying to kill the already-dead process.
// s.mu held by caller.
func (s *VideoPipeline) cleanupExitedLocked() {
	if s.push != nil {
		close(s.push.done)
		s.push = nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.rtpConn != nil {
		_ = s.rtpConn.Close()
	}
	s.running = false
	s.cmd = nil
	s.stdin = nil
	s.rtpConn = nil
}

// Shutdown stops everything; server exit path.
func (s *VideoPipeline) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	s.failAllLocked()
}

// failAllLocked closes every subscriber channel (lane is dead); s.mu held.
func (s *VideoPipeline) failAllLocked() {
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

// rtpReadLoop rebuilds Annex-B AUs from FFmpeg's loopback RTP stream. RTP's
// marker bit releases an AU immediately; it does not need a following frame.
func (s *VideoPipeline) rtpReadLoop(conn *net.UDPConn, gen int, written <-chan inputMeta) {
	dep := newRTPDepacketizer()
	buf := make([]byte, 64<<10)
	var currentTS uint32
	var haveTS bool
	var currentMeta inputMeta
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if n > 0 {
			_, _, _, timestamp, headerErr := parseRTP(buf[:n])
			if headerErr == nil && (!haveTS || timestamp != currentTS) {
				haveTS, currentTS = true, timestamp
				currentMeta = inputMeta{}
				select {
				case currentMeta = <-written:
				default:
					s.mappingFailures.Add(1)
					telemetry.Emit("source_au_mapping_failure", "video", "metadata_missing", nil)
				}
			}
			au, complete, depErr := dep.push(buf[:n])
			if depErr != nil {
				s.rtpErrors.Add(1)
				log.Printf("stream: RTP packet dropped: %v", depErr)
			}
			if complete {
				s.accessUnits.Add(1)
				nowNS := telemetry.MonoNS()
				if currentMeta.sourceNS != 0 &&
					nowNS >= currentMeta.sourceNS &&
					nowNS-currentMeta.sourceNS <= uint64(maxCorrelationAge) {
					au.SourceReceiveNS = currentMeta.sourceNS
					au.SourceSeq = currentMeta.sourceSeq
					au.InteractionID = currentMeta.interaction
				} else if currentMeta.sourceNS != 0 {
					s.mappingFailures.Add(1)
					telemetry.Emit("source_au_mapping_failure", "video", "metadata_expired", nil)
				}
				au.T = time.Now()
				au.EncodeCompleteNS = nowNS
				telemetry.Emit("encoded_au", "video", "rtp", map[string]any{"bytes": len(au.Data), "idr": au.IDR})
				s.broadcast(au, gen)
			}
		}
		if err != nil {
			break
		}
	}
}

// waitLoop is the sole process waiter and owns crash recovery.
func (s *VideoPipeline) waitLoop(cmd *os.Process, gen int) {
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

func (s *VideoPipeline) broadcast(au AU, gen int) {
	s.mu.Lock()
	if gen != s.gen || !s.running {
		s.mu.Unlock()
		return
	}
	s.seq++
	au.Seq = s.seq
	au.Generation = uint32(gen)
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
