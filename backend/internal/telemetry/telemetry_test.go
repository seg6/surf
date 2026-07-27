package telemetry

import (
	"bytes"
	"testing"
)

func TestHistogramBoundedBuckets(t *testing.T) {
	h := NewHistogram(10, 20)
	for _, v := range []uint64{1, 10, 11, 20, 21} {
		h.Observe(v)
	}
	s := h.Snapshot()
	if s.Count != 5 || s.Sum != 63 || len(s.Counts) != 3 || s.Counts[0] != 2 || s.Counts[1] != 2 || s.Counts[2] != 1 {
		t.Fatalf("snapshot %#v", s)
	}
}

func TestTraceRingBoundsAndOrder(t *testing.T) {
	r := NewTraceRing(2)
	r.SetEnabled(true)
	r.Add(TraceEvent{Name: "one"})
	r.Add(TraceEvent{Name: "two"})
	r.Add(TraceEvent{Name: "three"})
	e, dropped := r.Snapshot()
	if dropped != 1 || len(e) != 2 || e[0].Name != "two" || e[1].Name != "three" {
		t.Fatalf("%v dropped=%d", e, dropped)
	}
	var b bytes.Buffer
	if err := r.WriteChromeTrace(&b); err != nil || b.Len() == 0 {
		t.Fatal(err)
	}
}

func TestClockSyncChoosesLowestRTT(t *testing.T) {
	var c ClockSync
	c.Add(ClockSample{ClientSend: 100, BackendReceive: 130, BackendSend: 140, ClientReceive: 190})
	c.Add(ClockSample{ClientSend: 200, BackendReceive: 220, BackendSend: 225, ClientReceive: 250})
	offset, uncertainty, ok := c.Best()
	if !ok || offset != -2 || uncertainty != 22 {
		t.Fatalf("offset=%d uncertainty=%d", offset, uncertainty)
	}
}
