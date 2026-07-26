package stream

import "fmt"

// auSplitter cuts the raw Annex-B byte stream on Access Unit Delimiters
// (x264 runs with aud=1, so every AU starts with a type-9 NAL). The raw
// bytes, start codes included, are what the client wants.
type auSplitter struct {
	buf []byte
}

func newAUSplitter() *auSplitter { return &auSplitter{} }

// feed appends bytes and returns every complete AU found.
func (sp *auSplitter) feed(p []byte) ([]AU, error) {
	sp.buf = append(sp.buf, p...)
	if len(sp.buf) > maxAUBytes {
		return nil, fmt.Errorf("assembly buffer exceeded %d bytes", maxAUBytes)
	}
	var out []AU
	for {
		// Find the second AUD; everything before it is one complete AU.
		first := findAUD(sp.buf, 0)
		if first < 0 {
			break
		}
		next := findAUD(sp.buf, first+4)
		if next < 0 {
			break
		}
		au := make([]byte, next-first)
		copy(au, sp.buf[first:next])
		sp.buf = sp.buf[:copy(sp.buf, sp.buf[next:])]
		out = append(out, AU{Data: au, IDR: hasIDR(au)})
	}
	return out, nil
}

// findAUD returns the offset of the start code that introduces the next AUD
// (NAL type 9) at or after from, or -1.
func findAUD(b []byte, from int) int {
	for i := from; i+3 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 {
			continue
		}
		// 00 00 01 or 00 00 00 01
		var nalOff int
		if b[i+2] == 1 {
			nalOff = i + 3
		} else if b[i+2] == 0 && i+3 < len(b) && b[i+3] == 1 {
			nalOff = i + 4
		} else {
			continue
		}
		if nalOff >= len(b) {
			return -1 // start code at buffer edge; wait for more bytes
		}
		if b[nalOff]&0x1F == 9 {
			return i
		}
	}
	return -1
}

// hasIDR reports whether any NAL in the AU is an IDR slice (type 5).
func hasIDR(b []byte) bool {
	for i := 0; i+3 < len(b); i++ {
		if b[i] != 0 || b[i+1] != 0 {
			continue
		}
		var nalOff int
		if b[i+2] == 1 {
			nalOff = i + 3
		} else if b[i+2] == 0 && i+3 < len(b) && b[i+3] == 1 {
			nalOff = i + 4
		} else {
			continue
		}
		if nalOff < len(b) && b[nalOff]&0x1F == 5 {
			return true
		}
	}
	return false
}
