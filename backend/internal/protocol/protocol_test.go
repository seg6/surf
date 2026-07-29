package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestVideoEncode(t *testing.T) {
	data := []byte{0, 0, 1}
	meta := VideoMeta{AUSeq: 7, SourceSeq: 6, W: 1024, H: 768, InteractionID: 42,
		SourceReceiveNS: 100, EncodeCompleteNS: 200, EncoderGeneration: 3,
		InputReceiveNS: 50, CDPAcceptedNS: 75, ScrollX: 12.5, ScrollY: -3.25,
		PageScale: 1.0, Profile: 2}
	b := EncodeVideo(meta, true, data)
	if len(b) != FrameHeaderBytes+3 {
		t.Fatalf("length %d", len(b))
	}
	if string(b[0:4]) != FrameMagic || b[4] != FrameTypeVideo || b[5] != 1 {
		t.Fatal("bad magic/type")
	}
	if binary.BigEndian.Uint16(b[6:8]) != FrameHeaderBytes {
		t.Fatal("bad header len")
	}
	if binary.BigEndian.Uint32(b[8:12]) != 7 {
		t.Fatal("bad seq")
	}
	if binary.BigEndian.Uint16(b[16:18]) != 1024 || binary.BigEndian.Uint16(b[18:20]) != 768 {
		t.Fatal("bad dims")
	}
	if binary.BigEndian.Uint32(b[12:16]) != 6 || binary.BigEndian.Uint32(b[20:24]) != 3 || !bytes.Equal(b[FrameHeaderBytes:], data) {
		t.Fatal("bad payload")
	}
	if binary.BigEndian.Uint64(b[24:32]) != 42 || binary.BigEndian.Uint64(b[32:40]) != 100 ||
		binary.BigEndian.Uint64(b[40:48]) != 200 || binary.BigEndian.Uint32(b[56:60]) != 3 {
		t.Fatal("bad metadata")
	}
	if binary.BigEndian.Uint64(b[64:72]) != 50 || binary.BigEndian.Uint64(b[72:80]) != 75 ||
		int32(binary.BigEndian.Uint32(b[80:84])) != 12*65536+32768 ||
		int32(binary.BigEndian.Uint32(b[84:88])) != -3*65536-16384 ||
		binary.BigEndian.Uint32(b[88:92]) != 65536 || b[92] != 2 {
		t.Fatal("bad extended metadata")
	}
	StampSocketWrite(b, 300)
	if binary.BigEndian.Uint64(b[48:56]) != 300 {
		t.Fatal("socket timestamp")
	}
}

func TestFrameEncodeClampsDims(t *testing.T) {
	b := EncodeVideo(VideoMeta{W: 100000, H: -5}, false, nil)
	if binary.BigEndian.Uint16(b[16:18]) != 65535 || binary.BigEndian.Uint16(b[18:20]) != 0 {
		t.Fatal("dims not clamped")
	}
}
