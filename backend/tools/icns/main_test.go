package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"
)

func TestEncodeICNS(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 1024, 1024))); err != nil {
		t.Fatal(err)
	}
	encoded, err := encode(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded[:4], []byte("icns")) || binary.BigEndian.Uint32(encoded[4:8]) != uint32(len(encoded)) {
		t.Fatalf("header=%x size=%d", encoded[:8], len(encoded))
	}
	offset := 8
	if got := string(encoded[offset : offset+4]); got != "ic10" {
		t.Fatalf("entry at %d=%q, want ic10", offset, got)
	}
	size := int(binary.BigEndian.Uint32(encoded[offset+4 : offset+8]))
	if !bytes.Equal(encoded[offset+8:offset+size], source.Bytes()) {
		t.Fatal("ic10 payload differs from source PNG")
	}
	offset += size
	if offset != len(encoded) {
		t.Fatalf("parsed %d bytes, encoded %d", offset, len(encoded))
	}
}
