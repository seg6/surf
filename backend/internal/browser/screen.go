package browser

// syncVideoSurface retargets the H.264 encoder at a new viewport size — the
// only thing that used to need doing here was resizing the OS-level capture
// surface (X screen / hidden desktop) for x11grab/gdigrab's benefit; since
// the H.264 lane now transcodes CDP's own screencast frames instead of
// grabbing the screen, there's no OS surface to resize at all anymore.
func (b *Browser) syncVideoSurface(w, h int) {
	b.mu.Lock()
	current := w == b.viewW && h == b.viewH
	b.mu.Unlock()
	if !current {
		return
	}
	b.requestVideoResize(w, h, nil, "")
}
