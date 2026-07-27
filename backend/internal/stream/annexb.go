package stream

// Annex-B helpers used after RTP depacketization.

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
