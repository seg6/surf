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
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// subQueueCap bounds how far a slow subscriber may lag before it is
	// dropped to the next IDR (~2s of AUs at 15fps).
	subQueueCap = 30
	// lingerStop delays encoder shutdown after the last unsubscribe so fast
	// video off/on toggles don't cycle the process.
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
	Display                      string // ":99"
	W, H                         int    // coded size
	CaptureW, CaptureH           int    // X grab size; defaults to coded size
	ScaleMaxW, ScaleMaxH         int    // optional coded-size bounding box
	FPS                          int
	BitrateK, MaxrateK, BufsizeK int
	Preset                       string
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
	cmd       *exec.Cmd
	stdout    io.ReadCloser
	running   bool
	gen       int // process generation, guards stale readLoops
	seq       uint32
	stopTimer *time.Timer
	crashes   []time.Time // recent crash timestamps

	lastKeyframeReq time.Time
}

func New(cfg Config) *Streamer {
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

// Configure changes encoder knobs live. The current capture size is preserved;
// coded size is recomputed from the new scale box and subscribers resync on
// the next IDR after the restart.
func (s *Streamer) Configure(fps, maxW, maxH, bitrateK, maxrateK, bufsizeK int, preset string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fps >= 10 && fps <= 60 {
		s.cfg.FPS = fps
	}
	if maxW >= 64 && maxH >= 64 {
		s.cfg.ScaleMaxW, s.cfg.ScaleMaxH = maxW, maxH
	}
	if bitrateK >= 300 {
		s.cfg.BitrateK = bitrateK
	}
	if maxrateK >= s.cfg.BitrateK {
		s.cfg.MaxrateK = maxrateK
	}
	if bufsizeK >= 100 {
		s.cfg.BufsizeK = bufsizeK
	}
	if preset != "" {
		s.cfg.Preset = preset
	}
	s.cfg.W, s.cfg.H = s.cfg.codedSize(s.cfg.CaptureW, s.cfg.CaptureH)
	log.Printf("stream: configured capture=%dx%d coded=%dx%d@%dfps %dk/%dk buf=%dk preset=%s",
		s.cfg.CaptureW, s.cfg.CaptureH, s.cfg.W, s.cfg.H, s.cfg.FPS, s.cfg.BitrateK, s.cfg.MaxrateK, s.cfg.BufsizeK, s.cfg.Preset)
	if s.running {
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
// precedented tradeoff — SetSize/Configure restart the encoder for the same
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

func (s *Streamer) args() []string {
	c := s.cfg
	captureW, captureH := c.CaptureW, c.CaptureH
	if captureW == 0 || captureH == 0 {
		captureW, captureH = c.W, c.H
	}
	keyint := 2 * c.FPS
	args := []string{
		"-loglevel", "warning",
		// No input buffering/probing: x11grab is raw frames, every ms of
		// demuxer buffer is glass-to-glass latency (M1.2).
		"-fflags", "nobuffer",
		"-probesize", "32",
		"-f", "x11grab",
		"-draw_mouse", "0", // the X cursor is CDP's phantom, not the user's
		"-framerate", fmt.Sprint(c.FPS),
		"-video_size", fmt.Sprintf("%dx%d", captureW, captureH),
		"-i", c.Display,
	}
	if captureW != c.W || captureH != c.H {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear", c.W, c.H))
	}
	args = append(args,
		"-c:v", "libx264",
		"-profile:v", "baseline", "-level", c.h264Level(),
		"-preset", c.Preset,
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
	cmd := exec.Command("ffmpeg", s.args()...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("stream: stdout pipe: %v", err)
		s.failAllLocked()
		return
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		log.Printf("stream: ffmpeg start failed: %v", err)
		s.failAllLocked()
		return
	}
	s.cmd = cmd
	s.stdout = stdout
	s.running = true
	s.gen++
	gen := s.gen
	log.Printf("stream: encoder started pid=%d capture=%dx%d coded=%dx%d@%dfps %dk preset=%s",
		cmd.Process.Pid, s.cfg.CaptureW, s.cfg.CaptureH, s.cfg.W, s.cfg.H, s.cfg.FPS, s.cfg.BitrateK, s.cfg.Preset)
	if stderr != nil {
		go logStderr(stderr)
	}
	go s.readLoop(stdout, cmd, gen)
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
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
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
func (s *Streamer) readLoop(r io.Reader, cmd *exec.Cmd, gen int) {
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
	_ = cmd.Wait()

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

// ---- AU splitter -----------------------------------------------------------

// auSplitter cuts the raw Annex-B byte stream on Access Unit Delimiters
// (x264 runs with aud=1, so every AU starts with a type-9 NAL). The raw
// bytes, start codes included, are what the client wants.
type auSplitter struct {
	buf []byte
}

func newAUSplitter() *auSplitter { return &auSplitter{} }

// feed appends bytes and returns every complete AU found.
func (sp *auSplitter) feed(p []byte) ([]AU, error) {
	sp.buf = append(sp.buf, p...)
	if len(sp.buf) > maxAUBytes {
		return nil, fmt.Errorf("assembly buffer exceeded %d bytes", maxAUBytes)
	}
	var out []AU
	for {
		// Find the second AUD — everything before it is one complete AU.
		first := findAUD(sp.buf, 0)
		if first < 0 {
			break
		}
		next := findAUD(sp.buf, first+4)
		if next < 0 {
			break
		}
		au := make([]byte, next-first)
		copy(au, sp.buf[first:next])
		sp.buf = sp.buf[:copy(sp.buf, sp.buf[next:])]
		out = append(out, AU{Data: au, IDR: hasIDR(au)})
	}
	return out, nil
}

// findAUD returns the offset of the start code that introduces the next AUD
// (NAL type 9) at or after from, or -1.
func findAUD(b []byte, from int) int {
	for i := from; i+3 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 {
			continue
		}
		// 00 00 01 or 00 00 00 01
		var nalOff int
		if b[i+2] == 1 {
			nalOff = i + 3
		} else if b[i+2] == 0 && i+3 < len(b) && b[i+3] == 1 {
			nalOff = i + 4
		} else {
			continue
		}
		if nalOff >= len(b) {
			return -1 // start code at buffer edge; wait for more bytes
		}
		if b[nalOff]&0x1F == 9 {
			return i
		}
	}
	return -1
}

// hasIDR reports whether any NAL in the AU is an IDR slice (type 5).
func hasIDR(b []byte) bool {
	for i := 0; i+3 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 {
			continue
		}
		var nalOff int
		if b[i+2] == 1 {
			nalOff = i + 3
		} else if b[i+2] == 0 && i+3 < len(b) && b[i+3] == 1 {
			nalOff = i + 4
		} else {
			continue
		}
		if nalOff < len(b) && b[nalOff]&0x1F == 5 {
			return true
		}
	}
	return false
}
