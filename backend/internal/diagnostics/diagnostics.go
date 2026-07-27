package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/telemetry"
)

const (
	maxBundles     = 5
	maxBundleBytes = 25 << 20
)

type Router interface {
	Gated(pattern string, handler http.HandlerFunc)
}

type Manager struct {
	mu       sync.Mutex
	dir      string
	snapshot func() map[string]any
	trace    *telemetry.TraceRing
	active   bool
	started  time.Time
	timer    *time.Timer
	notify   func(bool)
}

func (m *Manager) SetCaptureNotifier(notify func(bool)) {
	m.mu.Lock()
	m.notify = notify
	m.mu.Unlock()
}

func New(home string, snapshot func() map[string]any) (*Manager, error) {
	dir := filepath.Join(home, "diagnostics")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Manager{dir: dir, snapshot: snapshot, trace: telemetry.NewTraceRing(120000)}, nil
}

func (m *Manager) Register(r Router) {
	r.Gated("/diagnostics", m.dashboard)
	r.Gated("/diagnostics/", m.route)
}

func (m *Manager) dashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, dashboardHTML)
}

func (m *Manager) route(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimPrefix(r.URL.Path, "/diagnostics/") {
	case "snapshot":
		m.writeSnapshot(w)
	case "events":
		m.events(w, r)
	case "metrics":
		m.metrics(w)
	case "trace/start":
		if !mutationAllowed(w, r) {
			return
		}
		m.startCapture()
		w.WriteHeader(http.StatusNoContent)
	case "trace/stop":
		if !mutationAllowed(w, r) {
			return
		}
		if _, err := m.stopCapture(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "client-trace":
		if !mutationAllowed(w, r) {
			return
		}
		m.clientTrace(w, r)
	case "bundles":
		m.listBundles(w)
	default:
		if strings.HasPrefix(r.URL.Path, "/diagnostics/bundles/") {
			m.downloadBundle(w, r)
			return
		}
		http.NotFound(w, r)
	}
}

func (m *Manager) clientTrace(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var events []telemetry.TraceEvent
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&events); err != nil || len(events) > 10000 {
		http.Error(w, "invalid trace batch", http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	active := m.active
	m.mu.Unlock()
	if active {
		for _, event := range events {
			// Only the fixed trace schema is accepted. Payload bytes, URLs and
			// arbitrary top-level fields cannot enter the bundle.
			event.Category = "native"
			event.PID = 2
			event.TID = "ipad"
			iid := event.Args["iid"]
			event.Args = nil
			if _, ok := iid.(float64); ok {
				event.Args = map[string]any{"iid": iid}
			}
			m.trace.Add(event)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func mutationAllowed(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost || r.Header.Get("X-Surf-Diagnostics") != "1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (m *Manager) state() map[string]any {
	s := m.snapshot()
	if s == nil {
		s = map[string]any{}
	}
	// URLs, typed text, credentials and media never enter diagnostics.
	delete(s, "activeURL")
	for k, v := range telemetry.RuntimeSnapshot() {
		s[k] = v
	}
	m.mu.Lock()
	s["traceActive"] = m.active
	if m.active {
		s["traceElapsedSec"] = time.Since(m.started).Seconds()
	}
	m.mu.Unlock()
	return s
}

func (m *Manager) writeSnapshot(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(m.state())
}

func (m *Manager) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		data, _ := json.Marshal(m.state())
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) metrics(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for key, value := range m.state() {
		switch n := value.(type) {
		case int:
			fmt.Fprintf(w, "surf_%s %d\n", metricName(key), n)
		case uint64:
			fmt.Fprintf(w, "surf_%s %d\n", metricName(key), n)
		case float64:
			fmt.Fprintf(w, "surf_%s %g\n", metricName(key), n)
		}
	}
}

func metricName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
			r += 'a' - 'A'
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *Manager) startCapture() {
	m.mu.Lock()
	if m.timer != nil {
		m.timer.Stop()
	}
	m.trace = telemetry.NewTraceRing(120000)
	m.trace.SetEnabled(true)
	telemetry.SetActiveTrace(m.trace)
	m.trace.Add(telemetry.TraceEvent{Name: "capture", Category: "diagnostics", Phase: "B", PID: os.Getpid(), TID: "backend"})
	m.active = true
	m.started = time.Now()
	m.timer = time.AfterFunc(30*time.Second, func() { _, _ = m.stopCapture() })
	notify := m.notify
	m.mu.Unlock()
	if notify != nil {
		notify(true)
	}
}

func (m *Manager) stopCapture() (string, error) {
	m.mu.Lock()
	if !m.active {
		m.mu.Unlock()
		return "", nil
	}
	m.trace.Add(telemetry.TraceEvent{Name: "capture", Category: "diagnostics", Phase: "E", PID: os.Getpid(), TID: "backend"})
	m.trace.SetEnabled(false)
	telemetry.SetActiveTrace(nil)
	m.active = false
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	started := m.started
	trace := m.trace
	notify := m.notify
	m.mu.Unlock()
	if notify != nil {
		notify(false)
	}

	name := "surf-" + time.Now().UTC().Format("20060102T150405Z") + ".zip"
	path := filepath.Join(m.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(f)
	writeJSON := func(name string, value any) error {
		entry, err := zw.Create(name)
		if err != nil {
			return err
		}
		return json.NewEncoder(entry).Encode(value)
	}
	events, dropped := trace.Snapshot()
	err = writeJSON("trace.json", map[string]any{"traceEvents": events, "metadata": map[string]any{"dropped": dropped}})
	if err == nil {
		err = writeJSON("summary.json", summarize(events, dropped, started))
	}
	if err == nil {
		err = writeJSON("metrics.json", m.state())
	}
	if err == nil {
		err = writeJSON("metadata.json", map[string]any{"appVersion": config.AppVersion, "protocolVersion": config.NativeVersion})
	}
	if err == nil {
		entry, createErr := zw.Create("logs.ndjson")
		if createErr != nil {
			err = createErr
		} else {
			// The trace schema is the backend's structured, pre-redacted event
			// log. Re-encode it as NDJSON rather than copying the legacy human
			// log, which may contain navigated URLs or filesystem paths.
			encoder := json.NewEncoder(entry)
			for _, event := range events {
				if event.PID != 1 {
					continue
				}
				if encodeErr := encoder.Encode(event); encodeErr != nil {
					err = encodeErr
					break
				}
			}
		}
	}
	closeErr := zw.Close()
	fileErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = fileErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	m.retain()
	return name, nil
}

func summarize(events []telemetry.TraceEvent, dropped uint64, started time.Time) map[string]any {
	sent := make(map[uint64]uint64)
	var interactionMS []float64
	var mappingFailures int
	var coalesced int
	for _, event := range events {
		if event.Name == "source_au_mapping_failure" {
			mappingFailures++
		}
		iid, ok := traceIID(event.Args)
		if !ok {
			continue
		}
		switch event.Name {
		case "interaction_send":
			sent[iid] = event.TimestampUS
		case "presented_frame":
			if begin, exists := sent[iid]; exists && event.TimestampUS >= begin {
				interactionMS = append(interactionMS, float64(event.TimestampUS-begin)/1000)
			}
			// The presented watermark causally includes iid. Older pending
			// inputs were superseded by a newer input before a display tick;
			// count them as coalesced rather than incorrectly calling them
			// unpresented.
			for pending := range sent {
				if pending <= iid {
					if pending != iid {
						coalesced++
					}
					delete(sent, pending)
				}
			}
		}
	}
	sort.Float64s(interactionMS)
	return map[string]any{
		"started": started.UTC(), "durationSec": time.Since(started).Seconds(),
		"interactionToPresent": map[string]any{
			"count": len(interactionMS), "p50Ms": percentile(interactionMS, 50),
			"p95Ms": percentile(interactionMS, 95), "p99Ms": percentile(interactionMS, 99),
			"coalesced": coalesced, "unpresented": len(sent),
		},
		"failures": map[string]any{"sourceAUMapping": mappingFailures, "traceDrops": dropped},
	}
}

func traceIID(args map[string]any) (uint64, bool) {
	switch n := args["iid"].(type) {
	case float64:
		return uint64(n), n > 0
	case uint64:
		return n, n > 0
	case int:
		return uint64(n), n > 0
	}
	return 0, false
}

func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := (p*len(sorted) + 99) / 100
	if i < 1 {
		i = 1
	}
	return sorted[i-1]
}

func (m *Manager) bundles() []os.FileInfo {
	entries, _ := os.ReadDir(m.dir)
	var out []os.FileInfo
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		if info, err := entry.Info(); err == nil {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime().After(out[j].ModTime()) })
	return out
}

func (m *Manager) retain() {
	files := m.bundles()
	var total int64
	for i, file := range files {
		total += file.Size()
		if i >= maxBundles || total > maxBundleBytes {
			_ = os.Remove(filepath.Join(m.dir, file.Name()))
		}
	}
}

func (m *Manager) listBundles(w http.ResponseWriter) {
	type bundle struct {
		Name string    `json:"name"`
		Size int64     `json:"size"`
		Time time.Time `json:"time"`
	}
	var bundles []bundle
	for _, file := range m.bundles() {
		bundles = append(bundles, bundle{file.Name(), file.Size(), file.ModTime()})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(bundles)
}

func (m *Manager) downloadBundle(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(strings.TrimPrefix(r.URL.Path, "/diagnostics/bundles/"))
	if !strings.HasSuffix(name, ".zip") {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(m.dir, name))
}

const dashboardHTML = `<!doctype html><meta charset=utf-8><title>Surf Diagnostics</title>
<style>body{font:14px system-ui;background:#111;color:#eee;margin:2rem}button{padding:.6rem;margin-right:.5rem}pre{white-space:pre-wrap}</style>
<h1>Surf Diagnostics</h1><button onclick="capture('start')">Start 30s capture</button><button onclick="capture('stop')">Stop</button>
<pre id=s>connecting…</pre><h2>Bundles</h2><div id=b></div><script>
const s=document.querySelector('#s');new EventSource('/diagnostics/events').onmessage=e=>s.textContent=JSON.stringify(JSON.parse(e.data),null,2);
async function bundles(){const x=await(await fetch('/diagnostics/bundles')).json();b.innerHTML=x.map(v=>'<p><a href="/diagnostics/bundles/'+encodeURIComponent(v.name)+'">'+v.name+'</a> '+Math.round(v.size/1024)+' KiB</p>').join('')}
async function capture(x){await fetch('/diagnostics/trace/'+x,{method:'POST',headers:{'X-Surf-Diagnostics':'1'}});setTimeout(bundles,1000)}bundles()
</script>`
