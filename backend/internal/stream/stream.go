// Package stream owns the H.264 lane: one ffmpeg process grabbing the X
// display (bypassing CDP entirely), an access-unit splitter, and per-
// subscriber fan-out with IDR-aware backpressure. The encoder runs only
// while at least one subscriber exists (plus a short linger), so the lane
// costs nothing when no native video client is connected.
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
	Display                      string // ":99" on Linux, or whatever surface name the platform uses
	FFmpegPath                   string
	Env                          []string
	W, H                         int // coded size
	CaptureW, CaptureH           int // grab size; defaults to coded size
	ScaleMaxW, ScaleMaxH         int // optional coded-size bounding box
	FPS                          int
	BitrateK, MaxrateK, BufsizeK int
	// CaptureArgs builds the ffmpeg arguments (up to and including "-i
	// <surface>") that grab this platform's rendered surface. Nil means the
	// H.264 lane is unsupported here; startLocked fails every subscriber
	// immediately and callers fall back to the JPEG/CDP screencast.
	CaptureArgs func(surface string, w, h, fps int) []string
	// Desktop names a Windows desktop (runenv.Handle.HiddenDesktop) ffmpeg
	// would launch onto instead of the interactive one, kept here for a
	// future capture method. Currently unreachable in practice: gdigrab
	// can't capture a desktop that isn't the foreground one (confirmed
	// live — see runenv_windows.go's VideoCaptureArgs), so that method
	// returns nil, and CaptureArgs is nil, whenever a hidden desktop is
	// active — this lane just falls back to JPEG/CDP screencast then.
	Desktop string
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
	stdout    io.ReadCloser
	running   bool
	gen       int // process generation, guards stale readLoops
	seq       uint32
	stopTimer *time.Timer
	crashes   []time.Time // recent crash timestamps

	lastKeyframeReq time.Time
}

func New(cfg Config) *Streamer {
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.Display == "" {
		cfg.Display = os.Getenv("DISPLAY")
		if cfg.Display == "" {
			cfg.Display = ":99"
		}
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

// SetSize retargets the encoder at a new viewport size. ffmpeg grabs the full
// X viewport, then optionally scales it down before x264 so old clients decode
// fewer pixels without cropping the page.
func (s *Streamer) SetSize(w, h int) {
	if w < 64 || h < 64 {
		return
	}
	cw, ch := s.cfg.codedSize(w, h)
	s.mu.Lock()
	defer s.mu.Unlock()
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
	for sub := range s.subs {
		sub.resetForGen(s.gen)
	}
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
	}
	if _, ok := s.subs[sub]; ok {
		sub.resetForGen(s.gen)
	}
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
// IDR — the only available "force a keyframe" primitive, since ffmpeg runs as
// an external pipe:1 subprocess with no live IPC hook (no stdin control, no
// zmq/sendcmd filter). A freshly spawned x264 process's first output frame is
// always an IDR regardless of keyint. This is a single-active-viewer product,
// so a restart affecting every subscriber is an acceptable, already-
// precedented tradeoff — SetSize restarts the encoder for the same
// reason. Cooldown-guarded so a resync storm can't thrash the process.
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

// args builds the full ffmpeg command line, or nil if this platform's
// CaptureArgs is unset or reports the H.264 lane unsupported (an empty
// slice) — checked here rather than by comparing CaptureArgs to nil,
// because callers usually pass a bound Handle method value, which is never
// a nil func even when the platform behind it always returns no args.
func (s *Streamer) args() []string {
	c := s.cfg
	if c.CaptureArgs == nil {
		return nil
	}
	captureW, captureH := c.CaptureW, c.CaptureH
	if captureW == 0 || captureH == 0 {
		captureW, captureH = c.W, c.H
	}
	capture := c.CaptureArgs(c.Display, captureW, captureH, c.FPS)
	if len(capture) == 0 {
		return nil
	}
	keyint := 2 * c.FPS
	args := append([]string{}, capture...)
	if captureW != c.W || captureH != c.H {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear", c.W, c.H))
	}
	args = append(args,
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
	args := s.args()
	if args == nil {
		log.Printf("stream: capture unsupported on this platform")
		s.failAllLocked()
		return
	}
	started, err := proc.Start(s.cfg.FFmpegPath, args, proc.Options{
		Env:     append(os.Environ(), s.cfg.Env...),
		Desktop: s.cfg.Desktop,
		Stdout:  true,
		Stderr:  true,
	})
	if err != nil {
		log.Printf("stream: ffmpeg start failed: %v", err)
		s.failAllLocked()
		return
	}
	s.cmd = started.Process
	s.stdout = started.Stdout
	s.running = true
	s.gen++
	gen := s.gen
	log.Printf("stream: encoder started pid=%d capture=%dx%d coded=%dx%d@%dfps %dk x264=%s",
		started.Process.Pid, s.cfg.CaptureW, s.cfg.CaptureH, s.cfg.W, s.cfg.H, s.cfg.FPS, s.cfg.BitrateK, x264Speed)
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
	if s.cmd != nil {
		proc.Kill(s.cmd.Pid)
	}
	s.running = false
	s.cmd = nil
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
	s.running = false
	s.cmd = nil
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
