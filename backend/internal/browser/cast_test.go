package browser

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type fakeSourceCDP struct {
	mu       sync.Mutex
	calls    []string
	started  chan string
	release  chan struct{}
	blockOne sync.Once
}

func (f *fakeSourceCDP) Call(session, method string, _ any) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, session+":"+method)
	f.mu.Unlock()
	if method == "Page.startScreencast" {
		select {
		case f.started <- session:
		default:
		}
		f.blockOne.Do(func() { <-f.release })
	}
	return nil, nil
}

func (f *fakeSourceCDP) Dispatch(string, string, any) error { return nil }

func TestScreencastSourceRejectsStaleStartCompletion(t *testing.T) {
	fake := &fakeSourceCDP{
		started: make(chan string, 4),
		release: make(chan struct{}),
	}
	source := NewScreencastSource(fake, func(SourceFrame) {})
	defer source.Close()
	source.Configure(1, "old", 100, 800, 600)
	select {
	case session := <-fake.started:
		if session != "old" {
			t.Fatalf("first start session = %q", session)
		}
	case <-time.After(time.Second):
		t.Fatal("old source did not start")
	}
	source.Configure(2, "new", 100, 1024, 768)
	close(fake.release)
	select {
	case session := <-fake.started:
		if session != "new" {
			t.Fatalf("second start session = %q", session)
		}
	case <-time.After(time.Second):
		t.Fatal("new source did not start")
	}
	deadline := time.Now().Add(time.Second)
	for {
		source.mu.Lock()
		current := source.current
		source.mu.Unlock()
		if current.enabled && current.session == "new" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("current source = %+v", current)
		}
		time.Sleep(time.Millisecond)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	foundStop := false
	for _, call := range fake.calls {
		if call == "old:Page.stopScreencast" {
			foundStop = true
		}
	}
	if !foundStop {
		t.Fatalf("stale source was not stopped: %v", fake.calls)
	}
}
