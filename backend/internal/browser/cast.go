package browser

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"sync"
	"time"

	"surf-backend/internal/cdp"
)

type sourceCDP interface {
	Call(sessionID, method string, params any) (json.RawMessage, error)
	Dispatch(sessionID, method string, params any) error
}

// SourceFrame is an immutable frame produced by Chromium. JPEG remains an
// internal capture representation; only VideoPipeline sees its bytes.
type SourceFrame struct {
	Session string
	JPEG    []byte
}

type FrameSource interface {
	Configure(tabID int, session string, quality, maxW, maxH int)
	Disable(session string)
	Casting() bool
	Handle(cdp.Event)
	Close()
}

type sourceConfig struct {
	tabID      int
	session    string
	quality    int
	maxW, maxH int
	enabled    bool
}

func clampQuality(q, fallback int) int {
	if q < 1 {
		q = fallback
	}
	if q > 100 {
		q = 100
	}
	return q
}

// ScreencastSource is the sole owner of Page.start/stopScreencast and capture
// credits. Desired-state generations ensure a late CDP completion can never
// revive an old tab or viewport.
type ScreencastSource struct {
	cdp     sourceCDP
	onFrame func(SourceFrame)

	mu         sync.Mutex
	desired    sourceConfig
	current    sourceConfig
	generation uint64
	wake       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
}

func NewScreencastSource(client sourceCDP, onFrame func(SourceFrame)) *ScreencastSource {
	s := &ScreencastSource{
		cdp: client, onFrame: onFrame,
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *ScreencastSource) Close() {
	s.closeOnce.Do(func() { close(s.done) })
}

func (s *ScreencastSource) stopCurrent() {
	s.mu.Lock()
	current := s.current
	s.current = sourceConfig{}
	s.mu.Unlock()
	if current.enabled {
		_, _ = s.cdp.Call(current.session, "Page.stopScreencast", nil)
	}
}

func (s *ScreencastSource) Configure(tabID int, session string, quality, maxW, maxH int) {
	quality = clampQuality(quality, 100)
	s.setDesired(sourceConfig{
		tabID: tabID, session: session, quality: quality,
		maxW: maxW, maxH: maxH, enabled: session != "",
	})
}

func (s *ScreencastSource) Disable(session string) {
	s.mu.Lock()
	if session != "" && s.desired.session != session {
		s.mu.Unlock()
		return
	}
	if !s.desired.enabled {
		s.mu.Unlock()
		return
	}
	s.desired.enabled = false
	s.generation++
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *ScreencastSource) setDesired(next sourceConfig) {
	s.mu.Lock()
	if s.desired == next {
		s.mu.Unlock()
		return
	}
	s.desired = next
	s.generation++
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *ScreencastSource) Casting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current.enabled
}

func (s *ScreencastSource) run() {
	for {
		select {
		case <-s.done:
			s.stopCurrent()
			return
		case <-s.wake:
		}
		for {
			select {
			case <-s.done:
				s.stopCurrent()
				return
			default:
			}
			s.mu.Lock()
			want, current, generation := s.desired, s.current, s.generation
			s.mu.Unlock()
			if current == want {
				break
			}
			if current.enabled {
				_, _ = s.cdp.Call(current.session, "Page.stopScreencast", nil)
				s.mu.Lock()
				if s.current == current {
					s.current = sourceConfig{}
				}
				s.mu.Unlock()
				continue
			}
			if !want.enabled {
				s.mu.Lock()
				if s.generation == generation {
					s.current = want
				}
				s.mu.Unlock()
				continue
			}
			opts := map[string]any{
				"format": "jpeg", "quality": want.quality,
				"maxWidth": want.maxW, "maxHeight": want.maxH, "everyNthFrame": 1,
			}
			var err error
			for attempt := 0; attempt < 15; attempt++ {
				if _, err = s.cdp.Call(want.session, "Page.startScreencast", opts); err == nil {
					break
				}
				time.Sleep(250 * time.Millisecond)
			}
			s.mu.Lock()
			stillWanted := s.generation == generation && s.desired == want
			if err == nil && stillWanted {
				s.current = want
			}
			s.mu.Unlock()
			if err != nil {
				log.Printf("startCast tab %d failed after retries: %v", want.tabID, err)
				break
			}
			if !stillWanted {
				// The start completed for a superseded target. Tear it down;
				// the next loop converges the latest desired generation.
				_, _ = s.cdp.Call(want.session, "Page.stopScreencast", nil)
				continue
			}
			log.Printf("cast tab %d started q=%d max=%dx%d", want.tabID, want.quality, want.maxW, want.maxH)
		}
	}
}

// Handle acknowledges the CDP credit before decoding. It is safe to call
// directly from the CDP event sink and never waits for the controller loop.
func (s *ScreencastSource) Handle(ev cdp.Event) {
	select {
	case <-s.done:
		return
	default:
	}
	var payload struct {
		Data      string `json:"data"`
		SessionID int    `json:"sessionId"`
	}
	if json.Unmarshal(ev.Params, &payload) != nil {
		return
	}
	_ = s.cdp.Dispatch(ev.SessionID, "Page.screencastFrameAck", map[string]any{"sessionId": payload.SessionID})
	s.mu.Lock()
	active := s.desired.enabled && s.desired.session == ev.SessionID
	s.mu.Unlock()
	if !active {
		return
	}
	data, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return
	}
	s.onFrame(SourceFrame{Session: ev.SessionID, JPEG: data})
}

func (b *Controller) ensureCast(t *Tab) {
	if b.source == nil || t == nil {
		return
	}
	if !b.hasVideoSubscribers() {
		b.source.Disable(t.Session)
		return
	}
	b.mu.Lock()
	active := t.ID == b.activeID
	session := t.Session
	w, h := b.viewW, b.viewH
	b.mu.Unlock()
	if active {
		b.source.Configure(t.ID, session, b.cfg.SourceJPEGQuality, w, h)
	}
}

func (b *Controller) stopCast(t *Tab) {
	if b.source != nil && t != nil {
		b.source.Disable(t.Session)
	}
}
