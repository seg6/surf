package audio

import (
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"
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
	// Capture opens a PCM source that produces signed little-endian 16 kHz
	// mono samples. Chromium tab capture supplies it on every platform.
	Capture func() (io.ReadCloser, error)
}

type Chunk struct {
	Seq        uint32
	SampleRate int
	Channels   int
	Data       []byte
	// T is when this chunk was read from tab capture, before fan-out —
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
	if s.cfg.Capture == nil {
		log.Printf("audio: capture unsupported on this platform")
		s.failAllLocked()
		return
	}
	capture, err := s.cfg.Capture()
	if err != nil {
		log.Printf("audio: tab capture failed: %v", err)
		s.failAllLocked()
		return
	}
	s.runID++
	runID := s.runID
	s.capture = capture
	s.running = true
	log.Printf("audio: tab capture started %dHz mono", sampleRate)
	go s.readLoop(capture, runID)
}

func (s *AudioPipeline) readLoop(r io.Reader, runID uint64) {
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
	s.mu.Lock()
	if runID == s.runID {
		log.Printf("audio: capture stopped")
		s.running = false
		s.capture = nil
		s.failAllLocked()
	}
	s.mu.Unlock()
}

func (s *AudioPipeline) stopLocked() {
	s.runID++
	if s.capture != nil {
		_ = s.capture.Close()
	}
	s.running = false
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
