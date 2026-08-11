package media

import (
	"bytes"
	"io"
	"testing"
)

func TestSubscribeFailsImmediatelyWhenCaptureUnset(t *testing.T) {
	s := NewAudioPipeline(AudioConfig{})
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
	s := NewAudioPipeline(AudioConfig{
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

func TestPCMSignalWindowDistinguishesSignalFromSilence(t *testing.T) {
	var signal pcmSignalWindow
	signal.observe([]byte{
		0x00, 0x00, // 0
		0x01, 0x00, // 1
		0xff, 0xff, // -1
		0xff, 0x7f, // 32767
		0x00, 0x80, // -32768
	})

	fields := signal.fields()
	if got := fields["pcm_state"]; got != "signal" {
		t.Fatalf("pcm_state=%v, want signal", got)
	}
	if got := fields["pcm_chunks"]; got != uint64(1) {
		t.Fatalf("pcm_chunks=%v, want 1", got)
	}
	if got := fields["pcm_silent_chunks"]; got != uint64(0) {
		t.Fatalf("pcm_silent_chunks=%v, want 0", got)
	}
	if got := fields["pcm_samples"]; got != uint64(5) {
		t.Fatalf("pcm_samples=%v, want 5", got)
	}
	if got := fields["pcm_nonzero_sample_percent"]; got != 80.0 {
		t.Fatalf("pcm_nonzero_sample_percent=%v, want 80", got)
	}
	if got := fields["pcm_mean_absolute"]; got != 13107.4 {
		t.Fatalf("pcm_mean_absolute=%v, want 13107.4", got)
	}
	if got := fields["pcm_peak"]; got != uint64(32768) {
		t.Fatalf("pcm_peak=%v, want 32768", got)
	}

	signal.reset()
	signal.observe(make([]byte, chunkBytes))
	fields = signal.fields()
	if got := fields["pcm_state"]; got != "silent" {
		t.Fatalf("silent pcm_state=%v, want silent", got)
	}
	if got := fields["pcm_silent_chunks"]; got != uint64(1) {
		t.Fatalf("silent pcm_silent_chunks=%v, want 1", got)
	}
}

func TestCaptureFailureClosesSubscription(t *testing.T) {
	s := NewAudioPipeline(AudioConfig{
		Capture: func() (io.ReadCloser, error) {
			return nil, io.ErrClosedPipe
		},
	})
	sub := s.Subscribe()
	if _, ok := <-sub.C; ok {
		t.Fatal("expected closed subscription when tab capture fails")
	}
}
