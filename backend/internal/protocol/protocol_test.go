package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameEncode(t *testing.T) {
	f := &Frame{Seq: 7, W: 1024, H: 768, ScrollX: 3, ScrollY: 123456, Data: []byte{0xff, 0xd8, 0xff}}
	b := f.Encode()
	if len(b) != FrameHeaderBytes+3 {
		t.Fatalf("length %d", len(b))
	}
	if string(b[0:4]) != FrameMagic || b[4] != FrameTypeFull {
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
	if binary.BigEndian.Uint32(b[20:24]) != 3 || !bytes.Equal(b[FrameHeaderBytes:], f.Data) {
		t.Fatal("bad payload")
	}
	if binary.BigEndian.Uint32(b[24:28]) != 3 || binary.BigEndian.Uint32(b[28:32]) != 123456 {
		t.Fatal("bad scroll offsets")
	}
}

func TestFrameEncodeClampsDims(t *testing.T) {
	f := &Frame{W: 100000, H: -5, Data: nil}
	b := f.Encode()
	if binary.BigEndian.Uint16(b[16:18]) != 65535 || binary.BigEndian.Uint16(b[18:20]) != 0 {
		t.Fatal("dims not clamped")
	}
}
