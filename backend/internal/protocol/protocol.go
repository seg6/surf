// Package protocol defines the wire format shared with the canvas client:
// a 32-byte binary header ("RBR1") in front of each JPEG frame, plus the
// JSON control messages in both directions.
//
// Input coordinates travel as fractions of the remote viewport (0..1), not
// pixels, so mid-gesture resolution switches can never misplace a tap.
package protocol

import "encoding/binary"

const (
	// Header layout: magic[4] type[1] flags[1] hdrlen[2] seq[4] pad[4]
	// w[2] h[2] payloadLen[4] scrollX[4] scrollY[4]. The client parses by
	// hdrlen, so the header can grow again without breaking anything.
	FrameHeaderBytes = 32
	FrameMagic       = "RBR1"
	FrameTypeFull    = 1
	// FrameTypeVideo carries one complete H.264 Annex-B access unit; sent
	// only to native clients in video mode. flags bit0 = IDR.
	FrameTypeVideo = 3
	// FrameTypeAudio carries signed little-endian PCM. w=sample rate, h=channels.
	FrameTypeAudio = 4
)

// Frame is one JPEG destined for the client canvas. ScrollX/Y are the page's
// scroll offset (remote CSS pixels) at capture time — the client uses them to
// reconcile its locally-panned view against reality.
type Frame struct {
	Seq     uint32
	W, H    int
	ScrollX uint32
	ScrollY uint32
	Data    []byte
	// Sharp marks the post-settle captureScreenshot. Not on the wire; the hub
	// uses it to route the one JPEG that video-mode clients still receive
	// (the crisp overlay).
	Sharp bool
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

// Encode renders header+payload ready for a single binary WS message.
func (f *Frame) Encode() []byte {
	out := make([]byte, FrameHeaderBytes+len(f.Data))
	copy(out[0:4], FrameMagic)
	out[4] = FrameTypeFull
	out[5] = 0 // flags, reserved
	binary.BigEndian.PutUint16(out[6:8], FrameHeaderBytes)
	binary.BigEndian.PutUint32(out[8:12], f.Seq)
	binary.BigEndian.PutUint16(out[12:14], 0) // reserved (was tile x)
	binary.BigEndian.PutUint16(out[14:16], 0) // reserved (was tile y)
	binary.BigEndian.PutUint16(out[16:18], clamp16(f.W))
	binary.BigEndian.PutUint16(out[18:20], clamp16(f.H))
	binary.BigEndian.PutUint32(out[20:24], uint32(len(f.Data)))
	binary.BigEndian.PutUint32(out[24:28], f.ScrollX)
	binary.BigEndian.PutUint32(out[28:32], f.ScrollY)
	copy(out[FrameHeaderBytes:], f.Data)
	return out
}

// EncodeVideoAU renders a type-3 frame around one Annex-B access unit.
// w/h are the coded size (constant per encoder run); flags bit0 marks IDR.
func EncodeVideoAU(seq uint32, idr bool, w, h int, au []byte) []byte {
	out := make([]byte, FrameHeaderBytes+len(au))
	copy(out[0:4], FrameMagic)
	out[4] = FrameTypeVideo
	if idr {
		out[5] = 1
	}
	binary.BigEndian.PutUint16(out[6:8], FrameHeaderBytes)
	binary.BigEndian.PutUint32(out[8:12], seq)
	binary.BigEndian.PutUint16(out[16:18], clamp16(w))
	binary.BigEndian.PutUint16(out[18:20], clamp16(h))
	binary.BigEndian.PutUint32(out[20:24], uint32(len(au)))
	copy(out[FrameHeaderBytes:], au)
	return out
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

// ClientMessage is the union of everything the client can send; T selects
// which fields matter. X/Y/DX/DY/CX/CY are viewport fractions (0..1).
type ClientMessage struct {
	T       string  `json:"t"`
	W       int     `json:"w,omitempty"`
	H       int     `json:"h,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	DX      float64 `json:"dx,omitempty"`
	DY      float64 `json:"dy,omitempty"`
	URL     string  `json:"url,omitempty"`
	Action  string  `json:"action,omitempty"`
	ID      int     `json:"id,omitempty"`
	Down    bool    `json:"down,omitempty"`
	Key     string  `json:"key,omitempty"`
	Code    string  `json:"code,omitempty"`
	KeyCode int     `json:"keyCode,omitempty"`
	Text    string  `json:"text,omitempty"`
	Scale   float64 `json:"scale,omitempty"` // zoom
	CX      float64 `json:"cx,omitempty"`    // zoom center (fraction)
	CY      float64 `json:"cy,omitempty"`
	Q       string  `json:"q,omitempty"`       // find / suggest / history query
	Dir     int     `json:"dir,omitempty"`     // find direction: 1 next, -1 prev
	Sel     bool    `json:"sel,omitempty"`     // lpup: select word at point (no drag happened)
	On      bool    `json:"on,omitempty"`      // video/audio/reader: enter/leave
	Profile string  `json:"profile,omitempty"` // stream profile: sharp/balanced/fast/potato
	Accept  bool    `json:"accept,omitempty"`  // dialogreply: OK (true) / Cancel
	Offset  int     `json:"offset,omitempty"`  // history: paging offset
	What    string  `json:"what,omitempty"`    // clear: history|cookies|cache
	TS      int64   `json:"ts,omitempty"`      // histdel: entry timestamp
	Name    string  `json:"name,omitempty"`    // dldel: download filename
}

// TabInfo is one entry of the 'tabs' broadcast.
type TabInfo struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Active bool   `json:"active"`
	Icon   string `json:"icon,omitempty"` // /tabicon/<id>?v=<hash> when known
}
