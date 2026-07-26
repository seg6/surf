package browser

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

// syncScreen best-effort resizes the live X screen (RANDR) to the client
// viewport. Xvfb also starts on an oversized canvas (start.sh), so the video
// lane can still grab the correctly-sized top-left viewport even when this X
// server refuses custom RANDR modes.
//
// Blocking (three tiny X round-trips); call off the WS read goroutine unless
// at startup. Failure is non-fatal: the JPEG lane doesn't care, the video
// lane just keeps its previous framing.
func syncScreen(xrandrPath, output string, env []string, w, h int) bool {
	if xrandrPath == "" {
		xrandrPath = "xrandr"
	}
	if output == "" {
		output = "screen"
	}
	name := fmt.Sprintf("%dx%d", w, h)
	ht, vt := w+64, h+16
	clock := float64(ht) * float64(vt) * 60.0 / 1e6 // ~60Hz; Xvfb doesn't care
	// Create + attach the mode; both fail harmlessly when it already exists.
	cmd := exec.Command(xrandrPath, "--newmode", name, fmt.Sprintf("%.2f", clock),
		fmt.Sprint(w), fmt.Sprint(w+16), fmt.Sprint(w+32), fmt.Sprint(ht),
		fmt.Sprint(h), fmt.Sprint(h+3), fmt.Sprint(h+6), fmt.Sprint(vt))
	cmd.Env = append(os.Environ(), env...)
	_ = cmd.Run()
	cmd = exec.Command(xrandrPath, "--addmode", output, name)
	cmd.Env = append(os.Environ(), env...)
	_ = cmd.Run()
	cmd = exec.Command(xrandrPath, "--output", output, "--mode", name)
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Run(); err != nil {
		return false
	}
	log.Printf("screen: X display resized to %s", name)
	return true
}

func (b *Browser) syncVideoSurface(w, h int) {
	b.screenMu.Lock()
	defer b.screenMu.Unlock()

	b.mu.Lock()
	current := w == b.viewW && h == b.viewH
	b.mu.Unlock()
	if !current {
		return
	}
	syncScreen(b.cfg.XrandrPath, b.cfg.XOutput, b.cfg.ChildEnv, w, h)
	b.mu.Lock()
	current = w == b.viewW && h == b.viewH
	b.mu.Unlock()
	if !current {
		return
	}
	b.streamer.SetSize(w, h)
}
