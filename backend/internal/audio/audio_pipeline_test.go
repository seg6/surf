package audio

import (
	"bytes"
	"io"
	"testing"
)

func TestSubscribeFailsImmediatelyWhenCaptureUnset(t *testing.T) {
	s := New(Config{})
	sub := s.Subscribe()
	select {
	case _, ok := <-sub.C:
		if ok {
			t.Fatal("expected closed channel, got a value")
		}
	default:
		t.Fatal("expected sub.C already closed after Subscribe")
	}
}

func TestNativeCaptureFeedsPCMChunks(t *testing.T) {
	want := bytes.Repeat([]byte{0x34, 0x12}, chunkBytes/2)
	s := New(Config{
		Capture: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(want)), nil
		},
	})
	defer s.Shutdown()
	sub := s.Subscribe()
	defer sub.Close()
	chunk, ok := <-sub.C
	if !ok {
		t.Fatal("native capture closed before producing a chunk")
	}
	if chunk.SampleRate != sampleRate || chunk.Channels != channels {
		t.Fatalf("format=%dHz/%dch", chunk.SampleRate, chunk.Channels)
	}
	if !bytes.Equal(chunk.Data, want) {
		t.Fatalf("PCM chunk differs: got %d bytes, want %d", len(chunk.Data), len(want))
	}
}

func TestCaptureFailureClosesSubscription(t *testing.T) {
	s := New(Config{
		Capture: func() (io.ReadCloser, error) {
			return nil, io.ErrClosedPipe
		},
	})
	sub := s.Subscribe()
	if _, ok := <-sub.C; ok {
		t.Fatal("expected closed subscription when tab capture fails")
	}
}
