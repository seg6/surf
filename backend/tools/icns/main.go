// Command icns wraps Surf's full-resolution PNG in the modern 1024-point ICNS
// entry. macOS scales that production artwork for each launcher presentation.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: icns INPUT.png OUTPUT.icns")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	check(err)
	output, err := encode(data)
	check(err)
	check(os.WriteFile(os.Args[2], output, 0o644))
}

func encode(data []byte) ([]byte, error) {
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if image.Bounds().Dx() != 1024 || image.Bounds().Dy() != 1024 {
		return nil, fmt.Errorf("icon dimensions are %dx%d, want 1024x1024",
			image.Bounds().Dx(), image.Bounds().Dy())
	}

	// ic10 is the 512-point @2x representation. Keeping the upstream PNG intact
	// avoids a low-resolution intermediate and lets macOS perform final scaling.
	var output bytes.Buffer
	output.WriteString("icns")
	_ = binary.Write(&output, binary.BigEndian, uint32(8+8+len(data)))
	output.WriteString("ic10")
	_ = binary.Write(&output, binary.BigEndian, uint32(8+len(data)))
	output.Write(data)
	return output.Bytes(), nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
