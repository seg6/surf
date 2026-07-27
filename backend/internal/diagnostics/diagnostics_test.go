package diagnostics

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"surf-backend/internal/telemetry"
)

func TestTraceMutationRequiresPostHeader(t *testing.T) {
	m, err := New(t.TempDir(), func() map[string]any { return map[string]any{"clients": 1} })
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/diagnostics/trace/start", nil),
		httptest.NewRequest(http.MethodPost, "/diagnostics/trace/start", nil),
	} {
		w := httptest.NewRecorder()
		m.route(w, request)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s without header = %d", request.Method, w.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/diagnostics/trace/start", nil)
	request.Header.Set("X-Surf-Diagnostics", "1")
	w := httptest.NewRecorder()
	m.route(w, request)
	if w.Code != http.StatusNoContent || !m.active {
		t.Fatalf("authorized start = %d active=%v", w.Code, m.active)
	}
}

func TestSummaryCorrelatesEachInteractionOnce(t *testing.T) {
	events := []telemetry.TraceEvent{
		{Name: "interaction_send", TimestampUS: 1000, Args: map[string]any{"iid": float64(7)}},
		{Name: "presented_frame", TimestampUS: 11000, Args: map[string]any{"iid": float64(7)}},
		{Name: "presented_frame", TimestampUS: 21000, Args: map[string]any{"iid": float64(7)}},
		{Name: "source_au_mapping_failure"},
	}
	summary := summarize(events, 2, time.Now())
	latency := summary["interactionToPresent"].(map[string]any)
	if latency["count"] != 1 || latency["p95Ms"] != float64(10) || latency["unpresented"] != 0 {
		t.Fatalf("latency = %#v", latency)
	}
	failures := summary["failures"].(map[string]any)
	if failures["sourceAUMapping"] != 1 || failures["traceDrops"] != uint64(2) {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestSummaryCountsSupersededInputsAsCoalesced(t *testing.T) {
	events := []telemetry.TraceEvent{
		{Name: "interaction_send", TimestampUS: 1000, Args: map[string]any{"iid": float64(1)}},
		{Name: "interaction_send", TimestampUS: 2000, Args: map[string]any{"iid": float64(2)}},
		{Name: "interaction_send", TimestampUS: 3000, Args: map[string]any{"iid": float64(3)}},
		{Name: "presented_frame", TimestampUS: 13000, Args: map[string]any{"iid": float64(3)}},
	}
	latency := summarize(events, 0, time.Now())["interactionToPresent"].(map[string]any)
	if latency["count"] != 1 || latency["coalesced"] != 2 || latency["unpresented"] != 0 {
		t.Fatalf("latency = %#v", latency)
	}
}

func TestBundleRetention(t *testing.T) {
	m, err := New(t.TempDir(), func() map[string]any { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		path := filepath.Join(m.dir, time.Unix(int64(i), 0).Format("150405")+".zip")
		if err := os.WriteFile(path, []byte{1}, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, time.Unix(int64(i), 0), time.Unix(int64(i), 0)); err != nil {
			t.Fatal(err)
		}
	}
	m.retain()
	if got := len(m.bundles()); got != maxBundles {
		t.Fatalf("bundles=%d want %d", got, maxBundles)
	}
}
