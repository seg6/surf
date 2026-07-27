package stream

import (
	"encoding/binary"
	"fmt"
)

var annexBStart = []byte{0, 0, 0, 1}

// rtpDepacketizer implements the H.264 packet forms emitted by FFmpeg's RTP
// muxer (single NAL, STAP-A and FU-A). It emits exactly one Annex-B AU at the
// RTP marker bit and discards a damaged AU after a sequence gap.
type rtpDepacketizer struct {
	buf       []byte
	haveSeq   bool
	nextSeq   uint16
	haveTS    bool
	timestamp uint32
	dropping  bool
	activeFU  bool
}

func newRTPDepacketizer() *rtpDepacketizer { return &rtpDepacketizer{} }

func (d *rtpDepacketizer) resetAU() {
	d.buf = d.buf[:0]
	d.activeFU = false
}

func (d *rtpDepacketizer) push(packet []byte) (AU, bool, error) {
	payload, marker, seq, ts, err := parseRTP(packet)
	if err != nil {
		return AU{}, false, err
	}
	if d.haveSeq && seq != d.nextSeq {
		d.resetAU()
		d.dropping = true
	}
	d.haveSeq = true
	d.nextSeq = seq + 1
	if d.haveTS && ts != d.timestamp && len(d.buf) != 0 {
		d.resetAU()
		d.dropping = true
	}
	d.haveTS = true
	d.timestamp = ts

	nalType := payload[0] & 0x1f
	startable := nalType >= 1 && nalType <= 24
	if nalType == 28 && len(payload) >= 2 {
		startable = payload[1]&0x80 != 0
	}
	if d.dropping {
		if !startable {
			if marker {
				d.dropping = false
			}
			return AU{}, false, nil
		}
		d.dropping = false
	}

	switch {
	case nalType >= 1 && nalType <= 23:
		d.buf = append(d.buf, annexBStart...)
		d.buf = append(d.buf, payload...)
		d.activeFU = false
	case nalType == 24: // STAP-A
		for p := payload[1:]; len(p) != 0; {
			if len(p) < 2 {
				d.resetAU()
				return AU{}, false, fmt.Errorf("truncated STAP-A length")
			}
			n := int(binary.BigEndian.Uint16(p))
			p = p[2:]
			if n == 0 || n > len(p) {
				d.resetAU()
				return AU{}, false, fmt.Errorf("invalid STAP-A NAL length %d", n)
			}
			d.buf = append(d.buf, annexBStart...)
			d.buf = append(d.buf, p[:n]...)
			p = p[n:]
		}
		d.activeFU = false
	case nalType == 28: // FU-A
		if len(payload) < 3 {
			d.resetAU()
			return AU{}, false, fmt.Errorf("truncated FU-A")
		}
		start := payload[1]&0x80 != 0
		end := payload[1]&0x40 != 0
		if start {
			d.buf = append(d.buf, annexBStart...)
			d.buf = append(d.buf, (payload[0]&0xe0)|(payload[1]&0x1f))
			d.activeFU = true
		} else if !d.activeFU {
			d.dropping = !marker
			return AU{}, false, fmt.Errorf("FU-A continuation without start")
		}
		d.buf = append(d.buf, payload[2:]...)
		if end {
			d.activeFU = false
		}
	default:
		d.resetAU()
		d.dropping = !marker
		return AU{}, false, fmt.Errorf("unsupported H.264 RTP packet type %d", nalType)
	}

	if len(d.buf) > maxAUBytes {
		d.resetAU()
		d.dropping = !marker
		return AU{}, false, fmt.Errorf("RTP AU exceeded %d bytes", maxAUBytes)
	}
	if !marker {
		return AU{}, false, nil
	}
	if d.activeFU {
		d.resetAU()
		return AU{}, false, fmt.Errorf("marker before FU-A end")
	}
	data := append([]byte(nil), d.buf...)
	d.resetAU()
	if len(data) == 0 {
		return AU{}, false, nil
	}
	return AU{Data: data, IDR: hasIDR(data)}, true, nil
}

func parseRTP(packet []byte) (payload []byte, marker bool, seq uint16, ts uint32, err error) {
	if len(packet) < 12 || packet[0]>>6 != 2 {
		return nil, false, 0, 0, fmt.Errorf("invalid RTP header")
	}
	headerLen := 12 + 4*int(packet[0]&0x0f)
	if headerLen > len(packet) {
		return nil, false, 0, 0, fmt.Errorf("truncated RTP CSRC list")
	}
	if packet[0]&0x10 != 0 {
		if headerLen+4 > len(packet) {
			return nil, false, 0, 0, fmt.Errorf("truncated RTP extension")
		}
		headerLen += 4 + 4*int(binary.BigEndian.Uint16(packet[headerLen+2:]))
		if headerLen > len(packet) {
			return nil, false, 0, 0, fmt.Errorf("truncated RTP extension data")
		}
	}
	end := len(packet)
	if packet[0]&0x20 != 0 {
		padding := int(packet[end-1])
		if padding == 0 || padding > end-headerLen {
			return nil, false, 0, 0, fmt.Errorf("invalid RTP padding")
		}
		end -= padding
	}
	if headerLen >= end {
		return nil, false, 0, 0, fmt.Errorf("empty RTP payload")
	}
	return packet[headerLen:end], packet[1]&0x80 != 0,
		binary.BigEndian.Uint16(packet[2:4]), binary.BigEndian.Uint32(packet[4:8]), nil
}
