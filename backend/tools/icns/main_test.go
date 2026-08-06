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
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 64, 64))); err != nil {
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
	for _, kind := range []string{"icp6", "ic12"} {
		if got := string(encoded[offset : offset+4]); got != kind {
			t.Fatalf("entry at %d=%q, want %q", offset, got, kind)
		}
		size := int(binary.BigEndian.Uint32(encoded[offset+4 : offset+8]))
		if !bytes.Equal(encoded[offset+8:offset+size], source.Bytes()) {
			t.Fatalf("%s payload differs from source PNG", kind)
		}
		offset += size
	}
	if offset != len(encoded) {
		t.Fatalf("parsed %d bytes, encoded %d", offset, len(encoded))
	}
}
