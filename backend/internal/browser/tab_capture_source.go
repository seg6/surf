package browser

import (
	"encoding/json"
	"log"
	"sync"
)

type FrameSource interface {
	Configure(session string)
	Disable(session string)
	Casting() bool
	Close()
}

// tabCaptureSource tracks whether the active tab should be producing frames.
// The extension bridge owns the actual capture lifecycle.
type tabCaptureSource struct {
	mu      sync.Mutex
	session string
}

func (s *tabCaptureSource) Configure(session string) {
	s.mu.Lock()
	s.session = session
	s.mu.Unlock()
}

func (s *tabCaptureSource) Disable(session string) {
	s.mu.Lock()
	if session == "" || s.session == session {
		s.session = ""
	}
	s.mu.Unlock()
}

func (s *tabCaptureSource) Casting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session != ""
}

func (s *tabCaptureSource) Close() {
	s.mu.Lock()
	s.session = ""
	s.mu.Unlock()
}

func (b *Controller) ensureCast(tab *Tab) {
	if b.source == nil || tab == nil {
		return
	}
	if !b.hasVideoSubscribers() {
		b.source.Disable(tab.Session)
		return
	}
	b.mu.Lock()
	active := tab.ID == b.activeID
	session := tab.Session
	b.mu.Unlock()
	if active {
		b.source.Configure(session)
	}
}

func (b *Controller) stopCast(tab *Tab) {
	if b.source != nil && tab != nil {
		b.source.Disable(tab.Session)
	}
}

func (b *Controller) resizeTabCaptureSurface(targetID string, width, height int) {
	if targetID == "" || width < 1 || height < 1 {
		return
	}
	raw, err := b.cdp.Call("", "Browser.getWindowForTarget", map[string]any{
		"targetId": targetID,
	})
	if err != nil {
		log.Printf("tab capture: get capture surface: %v", err)
		return
	}
	var window struct {
		ID     int `json:"windowId"`
		Bounds struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"bounds"`
	}
	if err := json.Unmarshal(raw, &window); err != nil || window.ID == 0 {
		log.Printf("tab capture: decode capture surface: %v", err)
		return
	}
	if window.Bounds.Width == width && window.Bounds.Height == height {
		return
	}
	if _, err := b.cdp.Call("", "Browser.setWindowBounds", map[string]any{
		"windowId": window.ID,
		"bounds": map[string]any{
			"width": width, "height": height,
		},
	}); err != nil {
		log.Printf("tab capture: resize capture surface to %dx%d: %v", width, height, err)
		return
	}
	log.Printf("tab capture: capture surface resized to %dx%d", width, height)
}
