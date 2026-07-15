// Command icon renders the app icon (cast-a-screen glyph, flat dark base,
// blue accent) to public/icon-{57,72,114,144}.png and public/icon.png.
// Pure stdlib: per-pixel evaluation in unit space with 4x4 supersampling.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

var (
	bg     = [3]float64{0x14, 0x15, 0x17} // graphite chassis, slightly lighter than app bg
	white  = [3]float64{0xec, 0xed, 0xee}
	accent = [3]float64{0xd9, 0xa4, 0x41} // bookmark gold
)

// sdRoundRect: signed distance to a rounded-rect border centered at (cx,cy).
func sdRoundRect(u, v, cx, cy, hw, hh, r float64) float64 {
	qx := math.Abs(u-cx) - (hw - r)
	qy := math.Abs(v-cy) - (hh - r)
	ax, ay := math.Max(qx, 0), math.Max(qy, 0)
	return math.Sqrt(ax*ax+ay*ay) + math.Min(math.Max(qx, qy), 0) - r
}

// colorAt evaluates the glyph in unit space (0..1, y down).
func colorAt(u, v float64) [3]float64 {
	// Cast origin: bottom-left inner corner of the screen.
	px, py := 0.24, 0.70
	du, dv := u-px, v-py
	dist := math.Sqrt(du*du + dv*dv)

	// Screen outline (rounded rect), broken around the cast origin.
	stroke := 0.040
	sd := sdRoundRect(u, v, 0.5, 0.47, 0.315, 0.235, 0.075)
	inCutout := dist < 0.235
	if math.Abs(sd) < stroke/2 && !inCutout {
		return white
	}

	// Cast arcs: two quarter-ring bands opening up-right from the origin.
	inQuadrant := du > -0.015 && dv < 0.015
	if inQuadrant {
		for _, r := range []float64{0.115, 0.195} {
			if math.Abs(dist-r) < stroke/2 {
				return accent
			}
		}
	}
	// Cast dot.
	if dist < 0.052 {
		return accent
	}
	return bg
}

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // supersamples per axis
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var acc [3]float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					u := (float64(x) + (float64(sx)+0.5)/ss) / float64(size)
					v := (float64(y) + (float64(sy)+0.5)/ss) / float64(size)
					c := colorAt(u, v)
					acc[0] += c[0]
					acc[1] += c[1]
					acc[2] += c[2]
				}
			}
			n := float64(ss * ss)
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(acc[0]/n + 0.5), G: uint8(acc[1]/n + 0.5), B: uint8(acc[2]/n + 0.5), A: 255,
			})
		}
	}
	return img
}

func main() {
	out := "public"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	for _, s := range []int{57, 72, 114, 144} {
		write(filepath.Join(out, "icon-"+itoa(s)+".png"), render(s))
	}
	write(filepath.Join(out, "icon.png"), render(144))
	log.Println("icons written to", out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func write(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
