package browser

// jpegSize returns a baseline/progressive JPEG's pixel dimensions by
// scanning its marker segments for a Start Of Frame marker, or ok=false if
// none is found (truncated/corrupt data). Chromium's screencast frames
// always have one; this is far cheaper than decoding the whole image just
// to read its size, and it's the authoritative size — unlike the configured
// Page.startScreencast bounds, what CDP actually
// delivers can drift from that: transitional frames right after
// navigation, aspect-ratio rounding, cast quality/size transitions. Feeding
// a size the running H.264 encoder doesn't expect corrupts its ffmpeg
// pipeline (confirmed live: "No JPEG data found in image" / "Invalid data
// found when processing input", the encoder crash-looping under real
// interactive use) — see onSourceFrame's serialized resize correction, which
// this exists to feed.
func jpegSize(data []byte) (w, h int, ok bool) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, false
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			i++
			continue
		}
		marker := data[i+1]
		// Standalone markers (no length/payload) — TEM and RSTn — just get
		// skipped past.
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD9) {
			i += 2
			continue
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		if segLen < 2 {
			return 0, 0, false
		}
		// SOF0-SOF15 except DHT(C4)/JPG(C8)/DAC(CC), which share the range
		// but aren't frame headers.
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			if i+9 > len(data) {
				return 0, 0, false
			}
			h = int(data[i+5])<<8 | int(data[i+6])
			w = int(data[i+7])<<8 | int(data[i+8])
			return w, h, w > 0 && h > 0
		}
		i += 2 + segLen
	}
	return 0, 0, false
}
