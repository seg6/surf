package browser

import (
	"math"
	"testing"

	"surf-backend/internal/protocol"
)

func TestMapTouchPointUsesCSSVisualViewport(t *testing.T) {
	viewport := cssVisualViewport{
		OffsetX: 12, OffsetY: 7, ClientWidth: 850, ClientHeight: 1033.724, Scale: 0.903529,
	}
	mapped, err := mapTouchPoint(protocol.TouchPoint{
		ID: 3, X: 0.5, Y: 0.5, RadiusX: 0.01, RadiusY: 0.02, Force: 0.4,
	}, viewport)
	if err != nil {
		t.Fatal(err)
	}
	if mapped["id"] != 3 || math.Abs(mapped["x"].(float64)-437) > 0.001 ||
		math.Abs(mapped["y"].(float64)-523.862) > 0.001 {
		t.Fatalf("mapped point = %#v", mapped)
	}
	if math.Abs(mapped["radiusX"].(float64)-8.5) > 0.001 ||
		math.Abs(mapped["radiusY"].(float64)-20.67448) > 0.001 {
		t.Fatalf("mapped radius = %#v", mapped)
	}
}

func TestMapTouchPointRejectsCoordinatesOutsideSurface(t *testing.T) {
	for _, point := range []protocol.TouchPoint{
		{ID: 1, X: 1.2, Y: 0.5},
		{ID: 1, X: 0.5, Y: -0.1},
		{ID: 1, X: math.Inf(1), Y: 0.5},
		{ID: 1, X: 0.5, Y: 0.5, Force: math.NaN()},
	} {
		if mapped, err := mapTouchPoint(point, cssVisualViewport{ClientWidth: 768, ClientHeight: 950}); err == nil {
			t.Fatalf("accepted point %#v as %#v", point, mapped)
		}
	}
}

func TestBrowserTouchIDsStayDenseAndReuseReleasedIDs(t *testing.T) {
	in := &touchInput{browserIDs: map[int]int{}}
	for clientID := 50; clientID < 50+maxTouchContacts; clientID++ {
		got, ok := in.addBrowserTouchID(clientID)
		if !ok || got != clientID-50 {
			t.Fatalf("client ID %d mapped to %d ok=%t", clientID, got, ok)
		}
	}
	if got, ok := in.addBrowserTouchID(50); !ok || got != 0 {
		t.Fatalf("existing client ID remapped to %d ok=%t", got, ok)
	}
	if _, ok := in.addBrowserTouchID(500); ok {
		t.Fatal("allocated more browser contacts than Chromium supports")
	}
	delete(in.browserIDs, 52)
	if got, ok := in.addBrowserTouchID(500); !ok || got != 2 {
		t.Fatalf("reused browser ID=%d ok=%t want 2", got, ok)
	}
}

func TestTouchCommandValidation(t *testing.T) {
	valid := &protocol.TouchCommand{Phase: "start", Sequence: 1, Surface: 2, TimestampNS: 3,
		Points: []protocol.TouchPoint{{ID: 1, X: 0.5, Y: 0.5}}}
	if !validTouchCommand(valid) {
		t.Fatal("valid touch start rejected")
	}
	invalid := *valid
	invalid.Phase = "cancel"
	if validTouchCommand(&invalid) {
		t.Fatal("cancel with points accepted")
	}
	invalid = *valid
	invalid.Points = nil
	if validTouchCommand(&invalid) {
		t.Fatal("start without points accepted")
	}
	invalid = *valid
	invalid.TimestampNS = 0
	if validTouchCommand(&invalid) {
		t.Fatal("zero timestamp accepted")
	}
}
