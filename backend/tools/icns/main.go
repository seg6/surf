// Command icns wraps a PNG in the modern ICNS entries macOS uses for a
// 32-point Retina application icon. macOS scales the source for other views.
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
	if image.Bounds().Dx() != 64 || image.Bounds().Dy() != 64 {
		return nil, fmt.Errorf("icon dimensions are %dx%d, want 64x64",
			image.Bounds().Dx(), image.Bounds().Dy())
	}

	// icp6 is the 64x64 icon and ic12 is the 32x32@2x Retina icon. Both
	// contain PNG data in modern ICNS files.
	var output bytes.Buffer
	output.WriteString("icns")
	_ = binary.Write(&output, binary.BigEndian, uint32(8+2*(8+len(data))))
	for _, kind := range []string{"icp6", "ic12"} {
		output.WriteString(kind)
		_ = binary.Write(&output, binary.BigEndian, uint32(8+len(data)))
		output.Write(data)
	}
	return output.Bytes(), nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
