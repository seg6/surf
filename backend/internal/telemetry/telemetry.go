// Package telemetry provides bounded, dependency-free metrics and trace
// primitives for the latency-sensitive browser/media pipeline.
package telemetry

import (
	"encoding/json"
	"io"
	"log/slog"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var processStart = time.Now()
var activeTrace atomic.Pointer[TraceRing]

func MonoNS() uint64 { return uint64(time.Since(processStart)) }

func SetActiveTrace(r *TraceRing) { activeTrace.Store(r) }

func Emit(name, category, thread string, args map[string]any) {
	if ring := activeTrace.Load(); ring != nil {
		ring.Add(TraceEvent{Name: name, Category: category, Phase: "i", PID: 1, TID: thread, Args: args})
	}
}

type Counter struct{ value atomic.Uint64 }

func (c *Counter) Add(n uint64)  { c.value.Add(n) }
func (c *Counter) Value() uint64 { return c.value.Load() }

type Gauge struct{ value atomic.Int64 }

func (g *Gauge) Set(n int64)  { g.value.Store(n) }
func (g *Gauge) Add(n int64)  { g.value.Add(n) }
func (g *Gauge) Value() int64 { return g.value.Load() }

// Histogram assigns each observation to the first inclusive upper bound.
// It has fixed memory use and stores no individual samples.
type Histogram struct {
	bounds []uint64
	counts []atomic.Uint64 // final bucket is +Inf
	sum    atomic.Uint64
	total  atomic.Uint64
}

func NewHistogram(bounds ...uint64) *Histogram {
	b := append([]uint64(nil), bounds...)
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	return &Histogram{bounds: b, counts: make([]atomic.Uint64, len(b)+1)}
}

func (h *Histogram) Observe(v uint64) {
	i := sort.Search(len(h.bounds), func(i int) bool { return v <= h.bounds[i] })
	h.counts[i].Add(1)
	h.sum.Add(v)
	h.total.Add(1)
}

type HistogramSnapshot struct {
	Bounds []uint64 `json:"bounds"`
	Counts []uint64 `json:"counts"`
	Count  uint64   `json:"count"`
	Sum    uint64   `json:"sum"`
}

func (h *Histogram) Snapshot() HistogramSnapshot {
	s := HistogramSnapshot{Bounds: append([]uint64(nil), h.bounds...), Counts: make([]uint64, len(h.counts)), Count: h.total.Load(), Sum: h.sum.Load()}
	for i := range h.counts {
		s.Counts[i] = h.counts[i].Load()
	}
	return s
}

type TraceEvent struct {
	Name        string         `json:"name"`
	Category    string         `json:"cat,omitempty"`
	Phase       string         `json:"ph"`
	TimestampUS uint64         `json:"ts"`
	DurationUS  uint64         `json:"dur,omitempty"`
	PID         int            `json:"pid"`
	TID         string         `json:"tid"`
	Args        map[string]any `json:"args,omitempty"`
}

// TraceRing overwrites oldest events and counts that loss explicitly.
type TraceRing struct {
	mu          sync.Mutex
	events      []TraceEvent
	next, count int
	dropped     uint64
	enabled     bool
}

func NewTraceRing(capacity int) *TraceRing {
	if capacity < 1 {
		capacity = 1
	}
	return &TraceRing{events: make([]TraceEvent, capacity)}
}
func (r *TraceRing) SetEnabled(on bool) { r.mu.Lock(); r.enabled = on; r.mu.Unlock() }
func (r *TraceRing) Add(e TraceEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return
	}
	if e.TimestampUS == 0 {
		e.TimestampUS = MonoNS() / 1000
	}
	if r.count == len(r.events) {
		r.dropped++
	} else {
		r.count++
	}
	r.events[r.next] = e
	r.next = (r.next + 1) % len(r.events)
}
func (r *TraceRing) Snapshot() ([]TraceEvent, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]TraceEvent, r.count)
	start := (r.next - r.count + len(r.events)) % len(r.events)
	for i := range out {
		out[i] = r.events[(start+i)%len(r.events)]
	}
	return out, r.dropped
}
func (r *TraceRing) WriteChromeTrace(w io.Writer) error {
	events, dropped := r.Snapshot()
	return json.NewEncoder(w).Encode(map[string]any{"traceEvents": events, "metadata": map[string]any{"dropped": dropped}})
}

// ClockSample is an NTP-style four-timestamp exchange. All timestamps use
// their respective host's monotonic clock.
type ClockSample struct{ ClientSend, BackendReceive, BackendSend, ClientReceive uint64 }

func (s ClockSample) RTT() uint64 {
	server := s.BackendSend - s.BackendReceive
	elapsed := s.ClientReceive - s.ClientSend
	if elapsed < server {
		return 0
	}
	return elapsed - server
}
func (s ClockSample) Offset() int64 {
	return (int64(s.BackendReceive) - int64(s.ClientSend) + int64(s.BackendSend) - int64(s.ClientReceive)) / 2
}

type ClockSync struct {
	mu      sync.Mutex
	samples []ClockSample
}

func (c *ClockSync) Add(s ClockSample) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples = append(c.samples, s)
	if len(c.samples) > 8 {
		c.samples = c.samples[len(c.samples)-8:]
	}
}
func (c *ClockSync) Best() (offset int64, uncertainty uint64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.samples) == 0 {
		return 0, 0, false
	}
	best := c.samples[0]
	for _, s := range c.samples[1:] {
		if s.RTT() < best.RTT() {
			best = s
		}
	}
	return best.Offset(), best.RTT() / 2, true
}

func RuntimeSnapshot() map[string]uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return map[string]uint64{"go_heap_bytes": m.HeapAlloc, "go_gc_cycles": uint64(m.NumGC), "go_goroutines": uint64(runtime.NumGoroutine())}
}

func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
