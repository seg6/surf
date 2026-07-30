package audio

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestSubscribeFailsImmediatelyWhenCaptureArgsUnset(t *testing.T) {
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

// TestSubscribeFailsWhenCaptureArgsReturnsEmpty guards against a real bug: a
// platform Handle's method value (e.g. windowsHandle{}.AudioCaptureArgs) is
// never a nil func even when calling it always returns an empty slice, so
// startLocked must check the result, not whether CaptureArgs itself is nil.
func TestSubscribeFailsWhenCaptureArgsReturnsEmpty(t *testing.T) {
	s := New(Config{CaptureArgs: func(string) []string { return nil }})
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

func TestPrimaryCaptureFailureConsultsPlatformFallback(t *testing.T) {
	fallbackCalled := false
	s := New(Config{
		Capture: func() (io.ReadCloser, error) {
			return nil, errors.New("primary unavailable")
		},
		CaptureArgs: func(string) []string {
			fallbackCalled = true
			return nil
		},
	})
	sub := s.Subscribe()
	if !fallbackCalled {
		t.Fatal("platform fallback was not consulted after primary capture failed")
	}
	if _, ok := <-sub.C; ok {
		t.Fatal("expected closed subscription when both capture paths fail")
	}
}
