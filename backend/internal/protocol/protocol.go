// Package protocol defines the H.264/audio wire format shared with the native
// client.
//
// Input coordinates travel as fractions of the remote viewport (0..1), not
// pixels, so mid-gesture resolution switches can never misplace a tap.
package protocol

import "encoding/binary"

const (
	// Header layout (network byte order):
	// magic[4], type[1], flags[1], hdrlen[2], AU seq[4], source seq[4],
	// width[2], height[2], payload len[4], interaction ID[8],
	// source-receive ns[8], encode-complete ns[8], socket-write ns[8],
	// encoder generation[4], reserved[4], backend input-receive ns[8],
	// CDP-dispatch-complete ns[8], scroll x/y Q16.16[4+4],
	// page scale Q16.16[4], adaptive profile[1], reserved[3].
	FrameHeaderBytes = 96
	FrameMagic       = "RBR1"
	// FrameTypeVideo carries one complete H.264 Annex-B access unit; sent
	// to native clients. flags bit0 = IDR.
	FrameTypeVideo = 3
	// FrameTypeAudio carries signed little-endian PCM. w=sample rate, h=channels.
	FrameTypeAudio = 4
)

// VideoMeta preserves causal and timing information through the encoder.
// Monotonic timestamps are nanoseconds since backend process start.
type VideoMeta struct {
	AUSeq, SourceSeq                  uint32
	W, H                              int
	InteractionID                     uint64
	SourceReceiveNS, EncodeCompleteNS uint64
	SocketWriteNS                     uint64
	EncoderGeneration                 uint32
	InputReceiveNS, CDPAcceptedNS     uint64
	ScrollX, ScrollY, PageScale       float64
	Profile                           uint8
}

func fixed16(v float64) uint32 {
	n := int64(v * 65536)
	if n > int64(^uint32(0)>>1) {
		n = int64(^uint32(0) >> 1)
	}
	if n < -int64(^uint32(0)>>1)-1 {
		n = -int64(^uint32(0)>>1) - 1
	}
	return uint32(int32(n))
}

func clamp16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}

// EncodeVideoAU renders a type-3 frame around one Annex-B access unit.
// w/h are the coded size (constant per encoder run); flags bit0 marks IDR.
func EncodeVideoAU(seq uint32, idr bool, w, h int, au []byte) []byte {
	return EncodeVideo(VideoMeta{AUSeq: seq, W: w, H: h}, idr, au)
}

func EncodeVideo(meta VideoMeta, idr bool, au []byte) []byte {
	out := make([]byte, FrameHeaderBytes+len(au))
	copy(out[0:4], FrameMagic)
	out[4] = FrameTypeVideo
	if idr {
		out[5] = 1
	}
	binary.BigEndian.PutUint16(out[6:8], FrameHeaderBytes)
	binary.BigEndian.PutUint32(out[8:12], meta.AUSeq)
	binary.BigEndian.PutUint32(out[12:16], meta.SourceSeq)
	binary.BigEndian.PutUint16(out[16:18], clamp16(meta.W))
	binary.BigEndian.PutUint16(out[18:20], clamp16(meta.H))
	binary.BigEndian.PutUint32(out[20:24], uint32(len(au)))
	binary.BigEndian.PutUint64(out[24:32], meta.InteractionID)
	binary.BigEndian.PutUint64(out[32:40], meta.SourceReceiveNS)
	binary.BigEndian.PutUint64(out[40:48], meta.EncodeCompleteNS)
	binary.BigEndian.PutUint64(out[48:56], meta.SocketWriteNS)
	binary.BigEndian.PutUint32(out[56:60], meta.EncoderGeneration)
	binary.BigEndian.PutUint64(out[64:72], meta.InputReceiveNS)
	binary.BigEndian.PutUint64(out[72:80], meta.CDPAcceptedNS)
	binary.BigEndian.PutUint32(out[80:84], fixed16(meta.ScrollX))
	binary.BigEndian.PutUint32(out[84:88], fixed16(meta.ScrollY))
	binary.BigEndian.PutUint32(out[88:92], fixed16(meta.PageScale))
	out[92] = meta.Profile
	copy(out[FrameHeaderBytes:], au)
	return out
}

// StampSocketWrite updates the only field that cannot be known until the
// writer goroutine owns the message.
func StampSocketWrite(frame []byte, monotonicNS uint64) {
	if len(frame) >= FrameHeaderBytes && string(frame[:4]) == FrameMagic {
		binary.BigEndian.PutUint64(frame[48:56], monotonicNS)
	}
}

func EncodeAudioPCM(seq uint32, sampleRate, channels int, pcm []byte) []byte {
	out := make([]byte, FrameHeaderBytes+len(pcm))
	copy(out[0:4], FrameMagic)
	out[4] = FrameTypeAudio
	binary.BigEndian.PutUint16(out[6:8], FrameHeaderBytes)
	binary.BigEndian.PutUint32(out[8:12], seq)
	binary.BigEndian.PutUint16(out[16:18], clamp16(sampleRate))
	binary.BigEndian.PutUint16(out[18:20], clamp16(channels))
	binary.BigEndian.PutUint32(out[20:24], uint32(len(pcm)))
	copy(out[FrameHeaderBytes:], pcm)
	return out
}

// TabInfo is one entry of the 'tabs' broadcast.
type TabInfo struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Active bool   `json:"active"`
	Icon   string `json:"icon,omitempty"` // /api/v1/tab-icons/<id>?v=<hash> when known
}
