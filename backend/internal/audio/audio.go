package audio

import (
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	sampleRate = 16000
	channels   = 1
	chunkMS    = 20
	chunkBytes = sampleRate * channels * 2 * chunkMS / 1000
	queueCap   = 20
	lingerStop = 5 * time.Second
)

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
	s *Streamer
}

type Streamer struct {
	mu        sync.Mutex
	subs      map[*Sub]struct{}
	cmd       *exec.Cmd
	running   bool
	seq       uint32
	stopTimer *time.Timer
}

func New() *Streamer { return &Streamer{subs: map[*Sub]struct{}{}} }

func (s *Streamer) Subscribe() *Sub {
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

func (s *Streamer) startLocked() {
	cmd := exec.Command("ffmpeg", "-loglevel", "warning",
		"-f", "pulse", "-i", "surf_output.monitor",
		"-ac", "1", "-ar", "16000", "-f", "s16le", "pipe:1")
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
	s.cmd = cmd
	s.running = true
	log.Printf("audio: capture started pid=%d %dHz mono", cmd.Process.Pid, sampleRate)
	if stderr != nil {
		go logStderr(stderr)
	}
	go s.readLoop(stdout, cmd)
}

func (s *Streamer) readLoop(r io.Reader, cmd *exec.Cmd) {
	buf := make([]byte, chunkBytes)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		data := append([]byte(nil), buf...)
		s.mu.Lock()
		if cmd != s.cmd || !s.running {
			s.mu.Unlock()
			return
		}
		s.seq++
		chunk := Chunk{Seq: s.seq, SampleRate: sampleRate, Channels: channels, Data: data, T: time.Now()}
		for sub := range s.subs {
			select {
			case sub.C <- chunk:
			default:
				// Audio should stay live, not buffered; drop if a client lags.
			}
		}
		s.mu.Unlock()
	}
	_ = cmd.Wait()
	s.mu.Lock()
	if cmd == s.cmd {
		log.Printf("audio: capture stopped")
		s.running = false
		s.cmd = nil
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

func (s *Streamer) stopLocked() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.running = false
	s.cmd = nil
}

func (s *Streamer) failAllLocked() {
	for sub := range s.subs {
		delete(s.subs, sub)
		close(sub.C)
	}
}

func (s *Streamer) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	s.failAllLocked()
}
