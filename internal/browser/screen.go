package browser

import (
	"fmt"
	"log"
	"os/exec"
)

// syncScreen resizes the Xvfb screen (RANDR) to the client viewport so the
// kiosk window — and with it the video lane's x11grab — shows exactly what
// clients see. Xvfb starts on an oversized canvas (start.sh) because RANDR
// can shrink below but never grow beyond the initial framebuffer.
//
// Blocking (three tiny X round-trips); call off the WS read goroutine unless
// at startup. Failure is non-fatal: the JPEG lane doesn't care, the video
// lane just keeps its previous framing.
func syncScreen(w, h int) {
	name := fmt.Sprintf("%dx%d", w, h)
	ht, vt := w+64, h+16
	clock := float64(ht) * float64(vt) * 60.0 / 1e6 // ~60Hz; Xvfb doesn't care
	// Create + attach the mode; both fail harmlessly when it already exists.
	_ = exec.Command("xrandr", "--newmode", name, fmt.Sprintf("%.2f", clock),
		fmt.Sprint(w), fmt.Sprint(w+16), fmt.Sprint(w+32), fmt.Sprint(ht),
		fmt.Sprint(h), fmt.Sprint(h+3), fmt.Sprint(h+6), fmt.Sprint(vt)).Run()
	_ = exec.Command("xrandr", "--addmode", "screen", name).Run()
	if out, err := exec.Command("xrandr", "--output", "screen", "--mode", name).CombinedOutput(); err != nil {
		log.Printf("screen: xrandr %s failed: %v %s", name, err, out)
		return
	}
	log.Printf("screen: X display resized to %s", name)
}
