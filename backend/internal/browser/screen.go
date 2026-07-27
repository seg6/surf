package browser

import "log"

// syncVideoSurface asks the active platform to resize its capture surface to
// match a new viewport (a no-op where there's no virtual display, e.g. on
// Windows/macOS) and retargets the H.264 encoder at the new size.
func (b *Browser) syncVideoSurface(w, h int) {
	b.screenMu.Lock()
	defer b.screenMu.Unlock()

	b.mu.Lock()
	current := w == b.viewW && h == b.viewH
	b.mu.Unlock()
	if !current {
		return
	}
	if err := b.platform.ResizeSurface(w, h); err != nil {
		log.Printf("screen: resize capture surface to %dx%d: %v", w, h, err)
	}
	b.mu.Lock()
	current = w == b.viewW && h == b.viewH
	b.mu.Unlock()
	if !current {
		return
	}
	b.streamer.SetSize(w, h)
}
