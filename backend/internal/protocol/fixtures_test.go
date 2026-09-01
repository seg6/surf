package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func fixturePath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "protocol", "fixtures", name)
}

func loadFixtures(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(fixturePath(name))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []json.RawMessage
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func TestCanonicalCommandFixturesDecode(t *testing.T) {
	fixtures := loadFixtures(t, "commands.json")
	if len(fixtures) != 43 {
		t.Fatalf("command fixture count = %d, want 43", len(fixtures))
	}
	for _, fixture := range fixtures {
		if _, err := DecodeCommand(fixture); err != nil {
			t.Errorf("fixture %s: %v", fixture, err)
		}
	}
}

func TestCanonicalEventFixturesMatchGoEncoders(t *testing.T) {
	entry := LibraryEntry{URL: "https://example.com", Title: "Example", TS: 42}
	events := []Event{
		HelloEvent{Type: "hello", W: 1180, H: 680},
		TabsEvent{Type: "tabs", Tabs: []TabInfo{{ID: 7, Title: "Example", URL: "https://example.com", Active: true, Icon: "/api/v1/tab-icons/7?v=x"}}},
		VideoConfigEvent{Type: "video-config", State: "ready", W: 1180, H: 680, Generation: 3, Profile: "sharp"},
		AudioConfigEvent{Type: "audio-config", OK: true, Rate: 16000, Channels: 1},
		BoolEvent{Type: "loading", On: true},
		BoolEvent{Type: "fullscreen"},
		BoolEvent{Type: "found", On: true},
		BoolEvent{Type: "starred"},
		TextEvent{Type: "toast", Text: "ready"},
		ClockEvent{Type: "clock", ClientSendNS: 1, BackendRecvNS: 2, BackendSendNS: 3},
		URLStateEvent{Type: "url", URL: "https://example.com", Starred: true, Security: "secure"},
		HistoryStateEvent{Type: "histstate", Back: true},
		NameEvent{Type: "download", Name: "archive.zip"},
		DownloadProgressEvent{Type: "dlprogress", Name: "archive.zip", Pct: 50},
		SuggestEvent{Type: "suggest", Items: []LibraryEntry{entry}},
		LibraryEvent{Type: "hist", History: []LibraryEntry{}, Bookmarks: []LibraryEntry{}},
		HistoryPageEvent{Type: "history", Query: "surf", Items: []LibraryEntry{}, Total: 0},
		DownloadsEvent{Type: "downloads", Items: []DownloadItem{{Name: "archive.zip", Size: 1234, TS: 42}}},
		DialogEvent{Type: "dialog", Kind: "prompt", Text: "Name?", Default: "Surf"},
		EmptyEvent{Type: "dialogdone"},
		FileChooserEvent{Type: "filechooser", Multiple: true},
		SecurityEvent{Type: "security", State: "secure"},
		URLStateEvent{Type: "pageerror", URL: "https://bad.invalid"},
		ReaderEvent{Type: "reader", OK: true, Title: "Article", HTML: "<p>Text</p>", URL: "https://example.com/article"},
		EditableEvent{Type: "editable", On: true, ShowKeyboard: true, Kind: "text", Rect: []float64{0.1, 0.2, 0.3, 0.1}},
		SelectEvent{Type: "select", RequestID: "select-1", Title: "Choose", Multiple: true, Options: []SelectOption{{Label: "One", Selected: true}}, Rect: []float64{0.1, 0.2, 0.3, 0.1}},
		MediaStateEvent{Type: "media-state", Available: true, Count: 1, Volume: 1, CurrentTime: 3, Duration: 90, Title: "Video"},
		PageFrameEvent{Type: "pageframe", SourceSeq: 99},
		ClipboardEvent{Type: "clipboard", RequestID: "clip-1", Text: "copied", Sync: true},
		ClipboardSyncEvent{Type: "clipboard-sync", Enabled: true, Known: true, Text: "copied"},
		EmptyEvent{Type: "log-request"},
		EmptyEvent{Type: "log-clear"},
	}
	fixtures := loadFixtures(t, "events.json")
	if len(fixtures) != len(events) {
		t.Fatalf("event fixture count = %d, Go event count = %d", len(fixtures), len(events))
	}
	for index, event := range events {
		actualData, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		var actual, expected any
		if err := json.Unmarshal(actualData, &actual); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(fixtures[index], &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("event fixture %d mismatch\nactual:   %s\nexpected: %s", index, actualData, fixtures[index])
		}
	}
}

func FuzzDecodeCommand(f *testing.F) {
	data, err := os.ReadFile(fixturePath("commands.json"))
	if err != nil {
		f.Fatal(err)
	}
	var fixtures []json.RawMessage
	if err := json.Unmarshal(data, &fixtures); err != nil {
		f.Fatal(err)
	}
	for _, fixture := range fixtures {
		f.Add([]byte(fixture))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		command, err := DecodeCommand(data)
		if err == nil && command.Kind() == "" {
			t.Fatal("successfully decoded command has no kind")
		}
	})
}
