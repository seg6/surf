package browser

import (
	"testing"

	"surf-backend/internal/protocol"
)

func validPointer() *protocol.PointerCommand {
	return &protocol.PointerCommand{
		Phase: "move", Sequence: 1, Surface: 2, TimestampNS: 3,
		X: 0.25, Y: 0.75, Button: "none",
	}
}

func TestPointerCommandValidation(t *testing.T) {
	command := validPointer()
	if !validPointerCommand(command) {
		t.Fatal("valid pointer command rejected")
	}
	invalid := *command
	invalid.X = -0.1
	if validPointerCommand(&invalid) {
		t.Fatal("negative normalized coordinate accepted")
	}
	invalid = *command
	invalid.Phase = "down"
	if validPointerCommand(&invalid) {
		t.Fatal("buttonless down accepted")
	}
	invalid.Button = "left"
	invalid.Buttons = 1
	invalid.ClickCount = 1
	if !validPointerCommand(&invalid) {
		t.Fatal("valid left down rejected")
	}
}

func TestWheelCommandValidation(t *testing.T) {
	command := &protocol.WheelCommand{
		Sequence: 1, Surface: 2, TimestampNS: 3,
		X: 0.25, Y: 0.75, DeltaY: 0.1,
	}
	if !validWheelCommand(command) {
		t.Fatal("valid wheel command rejected")
	}
	command.DeltaY = 4.1
	if validWheelCommand(command) {
		t.Fatal("unbounded wheel delta accepted")
	}
}

func TestPointerMoveReplacementPreservesEdges(t *testing.T) {
	first := validPointer()
	second := *first
	second.Sequence = 2
	if !replaceablePointerWork(
		pointerWork{pointer: first}, pointerWork{pointer: &second},
	) {
		t.Fatal("adjacent pointer moves were not replaceable")
	}
	second.Phase = "down"
	second.Button = "left"
	second.Buttons = 1
	second.ClickCount = 1
	if replaceablePointerWork(
		pointerWork{pointer: first}, pointerWork{pointer: &second},
	) {
		t.Fatal("pointer edge was replaceable")
	}
}

func TestWheelReplacementAccumulatesBoundedDistance(t *testing.T) {
	previous := pointerWork{wheel: &protocol.WheelCommand{Surface: 2, DeltaX: 1, DeltaY: 3.5}}
	next := pointerWork{wheel: &protocol.WheelCommand{Surface: 2, DeltaX: 2, DeltaY: 1}}
	coalescePointerWork(&previous, &next)
	if next.wheel.DeltaX != 3 || next.wheel.DeltaY != 4 {
		t.Fatalf("coalesced wheel delta = (%v, %v)", next.wheel.DeltaX, next.wheel.DeltaY)
	}
}

func TestPressedButtonMapping(t *testing.T) {
	got := pressedButtons(2 | 16)
	if len(got) != 2 || got[0].name != "right" || got[1].name != "forward" {
		t.Fatalf("pressed buttons mapped to %#v", got)
	}
}
