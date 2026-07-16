// nativeprobe exercises the exact native-client flow against a real server:
// login → /native-config → /ws?nv=…, RBR1 frame parsing, type-1 acks, and
// (with -video) the H.264 lane — capturing AUs, asserting IDR cadence and
// in-band SPS/PPS, and shelling out to ffmpeg to prove the stream decodes.
// It exists so client bugs and server bugs can be told apart without the iPad.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"rbrowser/internal/config"
)

type nativeConfig struct {
	Token string `json:"token"`
	VW    int    `json:"vw"`
	VH    int    `json:"vh"`
	NV    string `json:"nv"`
}

type frameHeader struct {
	Type       byte
	Flags      byte
	HdrLen     uint16
	Seq        uint32
	W, H       uint16
	PayloadLen uint32
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "PROBE-FAIL: "+format+"\n", args...)
	os.Exit(1)
}

func parseHeader(data []byte) (frameHeader, []byte, error) {
	var h frameHeader
	if len(data) < 32 || string(data[0:4]) != "RBR1" {
		return h, nil, fmt.Errorf("bad magic/short frame (%d bytes)", len(data))
	}
	h.Type = data[4]
	h.Flags = data[5]
	h.HdrLen = binary.BigEndian.Uint16(data[6:8])
	h.Seq = binary.BigEndian.Uint32(data[8:12])
	h.W = binary.BigEndian.Uint16(data[16:18])
	h.H = binary.BigEndian.Uint16(data[18:20])
	h.PayloadLen = binary.BigEndian.Uint32(data[20:24])
	if int(h.HdrLen) > len(data) {
		return h, nil, fmt.Errorf("hdrlen %d > frame %d", h.HdrLen, len(data))
	}
	payload := data[h.HdrLen:]
	if int(h.PayloadLen) != len(payload) {
		return h, nil, fmt.Errorf("payloadLen %d != actual %d", h.PayloadLen, len(payload))
	}
	return h, payload, nil
}

// nalTypes lists the NAL unit types present in an Annex-B AU.
func nalTypes(au []byte) map[byte]bool {
	out := map[byte]bool{}
	for i := 0; i+3 < len(au); i++ {
		if au[i] != 0 || au[i+1] != 0 {
			continue
		}
		var off int
		if au[i+2] == 1 {
			off = i + 3
		} else if au[i+2] == 0 && i+3 < len(au) && au[i+3] == 1 {
			off = i + 4
		} else {
			continue
		}
		if off < len(au) {
			out[au[off]&0x1F] = true
		}
	}
	return out
}

func main() {
	host := flag.String("host", "http://localhost:18080", "server base URL")
	pass := flag.String("pass", "", "server password")
	frames := flag.Int("frames", 5, "type-1 frames to receive before OK")
	video := flag.Bool("video", false, "exercise the H.264 lane")
	stall := flag.Bool("video-stall", false, "with -video: stop reading 5s mid-stream, assert IDR resync")
	videoSecs := flag.Int("video-secs", 10, "seconds of AUs to capture")
	out := flag.String("out", "out.h264", "capture file for -video")
	flag.Parse()

	if *pass == "" {
		fatalf("-pass required")
	}
	base, err := url.Parse(*host)
	if err != nil {
		fatalf("bad host: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// 1. login
	resp, err := hc.PostForm(base.String()+"/login", url.Values{"password": {*pass}})
	if err != nil {
		fatalf("login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		fatalf("login: HTTP %d", resp.StatusCode)
	}

	// 2. native-config
	resp, err = hc.Get(base.String() + "/native-config")
	if err != nil {
		fatalf("native-config: %v", err)
	}
	var nc nativeConfig
	err = json.NewDecoder(resp.Body).Decode(&nc)
	resp.Body.Close()
	if err != nil || nc.Token == "" {
		fatalf("native-config: bad response (err=%v)", err)
	}
	if nc.NV != config.NativeVersion {
		fatalf("native version mismatch: server %s, probe built with %s", nc.NV, config.NativeVersion)
	}
	fmt.Printf("native-config ok: vw=%d vh=%d nv=%s\n", nc.VW, nc.VH, nc.NV)

	// 3. websocket
	wsURL := *base
	if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path = "/ws"
	wsURL.RawQuery = url.Values{"k": {nc.Token}, "nv": {config.NativeVersion}}.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	send := func(v any) {
		if err := conn.WriteJSON(v); err != nil {
			fatalf("ws write: %v", err)
		}
	}
	send(map[string]any{"t": "size", "w": nc.VW, "h": nc.VH})

	sawHello := false
	type1 := 0
	deadline := time.Now().Add(30 * time.Second)

	// Phase A: control messages + type-1 frames.
	for type1 < *frames {
		if time.Now().After(deadline) {
			fatalf("timeout: hello=%t type1=%d/%d", sawHello, type1, *frames)
		}
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			fatalf("ws read: %v", err)
		}
		if mt == websocket.TextMessage {
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			t, _ := m["t"].(string)
			if t == "hello" {
				sawHello = true
			}
			fmt.Printf("  <- %s\n", t)
			continue
		}
		h, payload, err := parseHeader(data)
		if err != nil {
			fatalf("frame: %v", err)
		}
		if h.Type == 1 {
			if h.W == 0 || h.H == 0 {
				fatalf("type-1 frame with zero dims")
			}
			if !bytes.HasPrefix(payload, []byte{0xFF, 0xD8}) {
				fatalf("type-1 payload is not JPEG")
			}
			type1++
			send(map[string]any{"t": "ready"})
		}
	}
	if !sawHello {
		fatalf("never saw hello")
	}
	fmt.Printf("jpeg lane ok: %d type-1 frames, headers sane, acked\n", type1)

	if !*video {
		fmt.Println("PROBE-OK")
		return
	}

	// Phase B: the video lane.
	send(map[string]any{"t": "video", "on": true})
	var vcfg struct {
		OK  bool
		FPS int
		W   int
		H   int
	}
	vdeadline := time.Now().Add(15 * time.Second)
	for {
		if time.Now().After(vdeadline) {
			fatalf("no video-config within 15s")
		}
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			fatalf("ws read (video): %v", err)
		}
		if mt == websocket.TextMessage {
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			if t, _ := m["t"].(string); t == "video-config" {
				vcfg.OK, _ = m["ok"].(bool)
				if f, ok := m["fps"].(float64); ok {
					vcfg.FPS = int(f)
				}
				if f, ok := m["w"].(float64); ok {
					vcfg.W = int(f)
				}
				if f, ok := m["h"].(float64); ok {
					vcfg.H = int(f)
				}
				break
			}
			continue
		}
		if h, _, err := parseHeader(data); err == nil && h.Type == 1 {
			send(map[string]any{"t": "ready"})
		}
	}
	if !vcfg.OK {
		fatalf("video-config ok=false — lane unavailable")
	}
	fmt.Printf("video-config ok: %dx%d @%dfps\n", vcfg.W, vcfg.H, vcfg.FPS)

	f, err := os.Create(*out)
	if err != nil {
		fatalf("create %s: %v", *out, err)
	}
	defer f.Close()

	type auRec struct {
		seq   uint32
		idr   bool
		types map[byte]bool
	}
	var aus []auRec
	stalled := false
	firstAfterStall := -1
	captureEnd := time.Now().Add(time.Duration(*videoSecs) * time.Second)
	for time.Now().Before(captureEnd) {
		if *stall && !stalled && len(aus) > vcfg.FPS*2 {
			// Stop reading but keep scrolling: motion fattens the AUs so the
			// stall actually overruns the TCP + outbox buffers instead of
			// being absorbed silently.
			fmt.Println("stalling reads for 6s (while scrolling)…")
			for i := 0; i < 120; i++ {
				send(map[string]any{"t": "wheel", "x": 0.5, "y": 0.5, "dx": 0.0, "dy": 0.02})
				time.Sleep(50 * time.Millisecond)
			}
			stalled = true
			firstAfterStall = len(aus)
			captureEnd = captureEnd.Add(6 * time.Second)
		}
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			fatalf("ws read (AUs): %v", err)
		}
		if mt == websocket.TextMessage {
			continue
		}
		h, payload, err := parseHeader(data)
		if err != nil {
			fatalf("video frame: %v", err)
		}
		switch h.Type {
		case 1:
			// sharp settle frame — the one JPEG allowed in video mode
			send(map[string]any{"t": "ready"})
		case 3:
			if int(h.W) != vcfg.W || int(h.H) != vcfg.H {
				fatalf("AU dims %dx%d != config %dx%d", h.W, h.H, vcfg.W, vcfg.H)
			}
			if _, err := f.Write(payload); err != nil {
				fatalf("write capture: %v", err)
			}
			types := nalTypes(payload)
			idrFlag := h.Flags&1 == 1
			if idrFlag != types[5] {
				fatalf("seq %d: flags bit0=%t but NAL5 present=%t", h.Seq, idrFlag, types[5])
			}
			if idrFlag && (!types[7] || !types[8]) {
				fatalf("seq %d: IDR without in-band SPS/PPS", h.Seq)
			}
			aus = append(aus, auRec{seq: h.Seq, idr: idrFlag, types: types})
		}
	}
	send(map[string]any{"t": "video", "on": false})

	if len(aus) < vcfg.FPS**videoSecs/2 {
		fatalf("only %d AUs in %ds (expected ≈%d)", len(aus), *videoSecs, vcfg.FPS**videoSecs)
	}
	// IDR cadence: gaps between IDRs must be ≈ 2s (keyint = 2*fps).
	lastIDR := -1
	idrs := 0
	for i, au := range aus {
		if !au.idr {
			continue
		}
		idrs++
		if lastIDR >= 0 {
			gap := i - lastIDR
			if gap > vcfg.FPS*2+vcfg.FPS/2 {
				fatalf("IDR gap %d AUs (want ≈%d)", gap, vcfg.FPS*2)
			}
		}
		lastIDR = i
	}
	if idrs < 2 {
		fatalf("only %d IDRs captured", idrs)
	}
	if *stall {
		if firstAfterStall >= len(aus) {
			fatalf("no AUs after stall")
		}
		// A drop shows up as a seq gap; delivery must resume at an IDR. No
		// gap means the buffers absorbed the stall — lossless, also fine.
		gapAt := -1
		for i := max(firstAfterStall, 1); i < len(aus); i++ {
			if aus[i].seq != aus[i-1].seq+1 {
				gapAt = i
				break
			}
		}
		if gapAt < 0 {
			fmt.Println("stall absorbed losslessly (no seq gap — buffers covered it)")
		} else if !aus[gapAt].idr {
			fatalf("seq gap %d→%d resumed on a non-IDR AU", aus[gapAt-1].seq, aus[gapAt].seq)
		} else {
			fmt.Printf("stall resync ok: gap %d→%d resumed at IDR\n", aus[gapAt-1].seq, aus[gapAt].seq)
		}
	}
	fmt.Printf("captured %d AUs (%d IDRs) to %s\n", len(aus), idrs, *out)

	// Decode proof: desktop ffmpeg must chew the capture with zero errors.
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Println("ffmpeg not on PATH — decode check SKIPPED")
	} else {
		cmd := exec.Command("ffmpeg", "-v", "error", "-i", *out, "-f", "null", "-")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil || strings.TrimSpace(stderr.String()) != "" {
			fatalf("ffmpeg decode: err=%v stderr=%s", err, stderr.String())
		}
		fmt.Println("ffmpeg decode ok: zero errors")
	}
	fmt.Println("PROBE-OK")
}
