package audio

import "testing"

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
