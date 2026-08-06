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

func TestFinalMovePointsPromotesLiftCoordinatesIntoActiveSnapshot(t *testing.T) {
	in := &touchInput{
		viewport: cssVisualViewport{ClientWidth: 400, ClientHeight: 600},
		active: map[int]protocol.TouchPoint{
			2: {ID: 2, X: .8, Y: .4, RadiusX: .01, RadiusY: .01, Force: .5},
			1: {ID: 1, X: .2, Y: .7, RadiusX: .01, RadiusY: .01, Force: .5},
		},
	}
	points, moved, err := in.finalMovePoints([]protocol.TouchPoint{
		{ID: 1, X: .2, Y: .3, RadiusX: .01, RadiusY: .01, Force: .5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !moved || len(points) != 2 {
		t.Fatalf("final move moved=%t points=%#v", moved, points)
	}
	first := points[0].(map[string]any)
	second := points[1].(map[string]any)
	if first["id"] != 1 || first["y"] != 180.0 || second["id"] != 2 || second["y"] != 240.0 {
		t.Fatalf("final move points=%#v", points)
	}

	points, moved, err = in.finalMovePoints([]protocol.TouchPoint{
		{ID: 2, X: .8, Y: .4, RadiusX: .01, RadiusY: .01, Force: .5},
	})
	if err != nil || moved || points != nil {
		t.Fatalf("stationary lift points=%#v moved=%t err=%v", points, moved, err)
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
