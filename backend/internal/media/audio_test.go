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
	for name, tc := range map[string]struct {
		format AudioFormat
		want   AudioFormat
		bytes  int
	}{
		"default":  {AudioFormat{}, HighQualityAudio, 3840},
		"high":     {HighQualityAudio, HighQualityAudio, 3840},
		"standard": {StandardAudio, StandardAudio, 640},
	} {
		t.Run(name, func(t *testing.T) {
			want := bytes.Repeat([]byte{0x34, 0x12}, tc.bytes/2)
			s := NewAudioPipeline(AudioConfig{
				Format: tc.format,
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
			if chunk.SampleRate != tc.want.SampleRate || chunk.Channels != tc.want.Channels {
				t.Fatalf("format=%dHz/%dch", chunk.SampleRate, chunk.Channels)
			}
			if !bytes.Equal(chunk.Data, want) {
				t.Fatalf("PCM chunk differs: got %d bytes, want %d", len(chunk.Data), len(want))
			}
		})
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
