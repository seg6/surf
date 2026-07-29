package audio

import (
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"surf-backend/internal/proc"
)

const (
	sampleRate = 16000
	channels   = 1
	chunkMS    = 20
	chunkBytes = sampleRate * channels * 2 * chunkMS / 1000
	// Fan-out is only a handoff to ClientTransport, not another jitter
	// buffer. Keeping one newest packet prevents two independent 250ms
	// reservoirs from accumulating audible lag.
	queueCap   = 1 // one 20ms packet; overflow drops the oldest below
	lingerStop = 5 * time.Second
)

type Config struct {
	FFmpegPath string
	Source     string
	Env        []string
	// Capture opens a native PCM source that already produces signed
	// little-endian 16 kHz mono samples. Windows uses this for WASAPI
	// process-loopback capture. When nil, the pipeline launches FFmpeg with
	// CaptureArgs (the Linux PulseAudio path).
	Capture func() (io.ReadCloser, error)
	// CaptureArgs builds the ffmpeg arguments (up to and including "-i
	// <source>") that grab this platform's system audio. Nil means the PCM
	// lane is unsupported here; startLocked fails every subscriber
	// immediately.
	CaptureArgs func(source string) []string
}

type Chunk struct {
	Seq        uint32
	SampleRate int
	Channels   int
	Data       []byte
	// T is when this chunk was read off ffmpeg's stdout, before fan-out —
	// mirrors stream.AU.T, used to measure per-subscriber queueing delay.
	T time.Time
}

type Sub struct {
	C chan Chunk
	s *AudioPipeline
}

type AudioPipeline struct {
	cfg       Config
	mu        sync.Mutex
	subs      map[*Sub]struct{}
	cmd       *exec.Cmd
	capture   io.ReadCloser
	running   bool
	runID     uint64
	seq       uint32
	stopTimer *time.Timer
	drops     atomic.Uint64
}

func (s *AudioPipeline) Stats() map[string]uint64 {
	return map[string]uint64{"audio_fanout_drops": s.drops.Load()}
}

func New(cfg Config) *AudioPipeline {
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.Source == "" {
		cfg.Source = "surf_output.monitor"
	}
	return &AudioPipeline{cfg: cfg, subs: map[*Sub]struct{}{}}
}

func (s *AudioPipeline) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *AudioPipeline) Subscribe() *Sub {
	sub := &Sub{C: make(chan Chunk, queueCap), s: s}
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
	return sub
}

func (sub *Sub) Close() {
	s := sub.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub]; !ok {
		return
	}
	delete(s.subs, sub)
	close(sub.C)
	if len(s.subs) == 0 && s.running {
		s.stopTimer = time.AfterFunc(lingerStop, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if len(s.subs) == 0 && s.running {
				log.Printf("audio: idle, stopping capture")
				s.stopLocked()
			}
		})
	}
}

func (s *AudioPipeline) startLocked() {
	if s.cfg.Capture != nil {
		capture, err := s.cfg.Capture()
		if err != nil {
			log.Printf("audio: native capture failed: %v", err)
			s.failAllLocked()
			return
		}
		s.runID++
		runID := s.runID
		s.capture = capture
		s.running = true
		log.Printf("audio: native capture started %dHz mono", sampleRate)
		go s.readLoop(capture, nil, runID)
		return
	}
	var captureArgs []string
	if s.cfg.CaptureArgs != nil {
		captureArgs = s.cfg.CaptureArgs(s.cfg.Source)
	}
	if len(captureArgs) == 0 {
		log.Printf("audio: capture unsupported on this platform")
		s.failAllLocked()
		return
	}
	args := append(append([]string{}, captureArgs...),
		"-ac", "1", "-ar", "16000", "-f", "s16le", "pipe:1")
	cmd := proc.Command(s.cfg.FFmpegPath, args...)
	cmd.Env = append(os.Environ(), s.cfg.Env...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("audio: stdout pipe: %v", err)
		s.failAllLocked()
		return
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		log.Printf("audio: ffmpeg start failed: %v", err)
		s.failAllLocked()
		return
	}
	s.runID++
	runID := s.runID
	s.cmd = cmd
	s.capture = stdout
	s.running = true
	log.Printf("audio: capture started pid=%d %dHz mono", cmd.Process.Pid, sampleRate)
	if stderr != nil {
		go logStderr(stderr)
	}
	go s.readLoop(stdout, cmd, runID)
}

func (s *AudioPipeline) readLoop(r io.Reader, cmd *exec.Cmd, runID uint64) {
	buf := make([]byte, chunkBytes)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		data := append([]byte(nil), buf...)
		s.mu.Lock()
		if runID != s.runID || !s.running {
			s.mu.Unlock()
			return
		}
		s.seq++
		chunk := Chunk{Seq: s.seq, SampleRate: sampleRate, Channels: channels, Data: data, T: time.Now()}
		for sub := range s.subs {
			select {
			case sub.C <- chunk:
			default:
				// Audio should stay live, not buffered. Discard the oldest
				// chunk and admit the newest so a slow writer cannot build a
				// delay that only reconnecting clears.
				select {
				case <-sub.C:
					s.drops.Add(1)
				default:
				}
				select {
				case sub.C <- chunk:
				default:
				}
			}
		}
		s.mu.Unlock()
	}
	if cmd != nil {
		_ = cmd.Wait()
	}
	s.mu.Lock()
	if runID == s.runID {
		log.Printf("audio: capture stopped")
		s.running = false
		s.cmd = nil
		s.capture = nil
		s.failAllLocked()
	}
	s.mu.Unlock()
}

func logStderr(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, line := range strings.Split(strings.TrimSpace(string(buf[:n])), "\n") {
				if line != "" {
					log.Printf("audio/ffmpeg: %s", line)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *AudioPipeline) stopLocked() {
	s.runID++
	if s.capture != nil {
		_ = s.capture.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		proc.Kill(s.cmd.Process.Pid)
	}
	s.running = false
	s.cmd = nil
	s.capture = nil
}

func (s *AudioPipeline) failAllLocked() {
	for sub := range s.subs {
		delete(s.subs, sub)
		close(sub.C)
	}
}

func (s *AudioPipeline) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	s.failAllLocked()
}
