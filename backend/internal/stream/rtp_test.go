package stream

import (
	"encoding/binary"
	"testing"
)

func rtpPacket(seq uint16, ts uint32, marker bool, payload []byte) []byte {
	p := make([]byte, 12+len(payload))
	p[0], p[1] = 0x80, 96
	if marker {
		p[1] |= 0x80
	}
	binary.BigEndian.PutUint16(p[2:], seq)
	binary.BigEndian.PutUint32(p[4:], ts)
	copy(p[12:], payload)
	return p
}

func TestRTPDepacketizesSingleNAL(t *testing.T) {
	d := newRTPDepacketizer()
	au, ok, err := d.push(rtpPacket(1, 90, true, []byte{0x65, 1, 2}))
	if err != nil || !ok || !au.IDR {
		t.Fatalf("push = ok %v IDR %v err %v", ok, au.IDR, err)
	}
	want := []byte{0, 0, 0, 1, 0x65, 1, 2}
	if string(au.Data) != string(want) {
		t.Fatalf("data %x, want %x", au.Data, want)
	}
}

func TestRTPDepacketizesFUA(t *testing.T) {
	d := newRTPDepacketizer()
	if _, ok, err := d.push(rtpPacket(7, 100, false, []byte{0x7c, 0x85, 1, 2})); err != nil || ok {
		t.Fatalf("start ok=%v err=%v", ok, err)
	}
	au, ok, err := d.push(rtpPacket(8, 100, true, []byte{0x7c, 0x45, 3, 4}))
	if err != nil || !ok || !au.IDR {
		t.Fatalf("end ok=%v IDR=%v err=%v", ok, au.IDR, err)
	}
	want := []byte{0, 0, 0, 1, 0x65, 1, 2, 3, 4}
	if string(au.Data) != string(want) {
		t.Fatalf("data %x, want %x", au.Data, want)
	}
}

func TestRTPDepacketizesSTAPA(t *testing.T) {
	d := newRTPDepacketizer()
	payload := []byte{0x78, 0, 2, 0x67, 1, 0, 2, 0x68, 2}
	au, ok, err := d.push(rtpPacket(1, 1, true, payload))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := []byte{0, 0, 0, 1, 0x67, 1, 0, 0, 0, 1, 0x68, 2}
	if string(au.Data) != string(want) {
		t.Fatalf("data %x, want %x", au.Data, want)
	}
}

func TestRTPSequenceGapDropsPartialFU(t *testing.T) {
	d := newRTPDepacketizer()
	_, _, _ = d.push(rtpPacket(1, 1, false, []byte{0x7c, 0x85, 1}))
	if _, ok, _ := d.push(rtpPacket(3, 1, true, []byte{0x7c, 0x45, 2})); ok {
		t.Fatal("damaged AU emitted")
	}
	_, ok, err := d.push(rtpPacket(4, 2, true, []byte{0x41, 9}))
	if err != nil || !ok {
		t.Fatalf("next intact AU ok=%v err=%v", ok, err)
	}
}
