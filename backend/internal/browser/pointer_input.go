package browser

import (
	"log"
	"math"
	"sync"
	"time"

	"surf-backend/internal/protocol"
	"surf-backend/internal/telemetry"
	"surf-backend/internal/transport"
)

const pointerQueueLimit = 64

type pointerWork struct {
	client     *transport.Client
	pointer    *protocol.PointerCommand
	wheel      *protocol.WheelCommand
	command    protocol.Command
	receivedNS uint64
	cancel     bool
	done       chan struct{}
}

// pointerInput is the ordered desktop input lane. Pointer moves and wheel
// samples are replaceable under overload, while button edges remain reliable
// and cannot be trapped behind browser/controller work.
type pointerInput struct {
	b *Controller

	mu    sync.Mutex
	queue []pointerWork
	wake  chan struct{}
	stop  chan struct{}

	owner        *transport.Client
	session      string
	surface      uint32
	lastSeq      uint64
	lastClientNS uint64
	viewport     cssVisualViewport
	buttons      int
}

func newPointerInput(b *Controller) *pointerInput {
	in := &pointerInput{b: b, wake: make(chan struct{}, 1), stop: make(chan struct{})}
	go in.run()
	return in
}

func (in *pointerInput) enqueuePointer(client *transport.Client, command *protocol.PointerCommand, receivedNS uint64) {
	in.enqueue(pointerWork{client: client, pointer: command, receivedNS: receivedNS})
}

func (in *pointerInput) enqueueWheel(client *transport.Client, command *protocol.WheelCommand, receivedNS uint64) {
	in.enqueue(pointerWork{client: client, wheel: command, receivedNS: receivedNS})
}

func (in *pointerInput) enqueueCommand(client *transport.Client, command protocol.Command, receivedNS uint64) {
	in.enqueue(pointerWork{client: client, command: command, receivedNS: receivedNS})
}

func (in *pointerInput) enqueue(work pointerWork) {
	in.mu.Lock()
	if len(in.queue) > 0 && replaceablePointerWork(in.queue[len(in.queue)-1], work) {
		coalescePointerWork(&in.queue[len(in.queue)-1], &work)
		in.queue[len(in.queue)-1] = work
		in.mu.Unlock()
		return
	}
	if len(in.queue) >= pointerQueueLimit {
		removed := false
		for index, queued := range in.queue {
			if queued.replaceable() {
				copy(in.queue[index:], in.queue[index+1:])
				in.queue = in.queue[:len(in.queue)-1]
				removed = true
				break
			}
		}
		if !removed {
			in.mu.Unlock()
			work.client.Close()
			return
		}
	}
	in.queue = append(in.queue, work)
	in.mu.Unlock()
	select {
	case in.wake <- struct{}{}:
	default:
	}
}

func (work pointerWork) replaceable() bool {
	return work.wheel != nil || (work.pointer != nil && work.pointer.Phase == "move")
}

func replaceablePointerWork(previous, next pointerWork) bool {
	if previous.client != next.client || !previous.replaceable() || !next.replaceable() {
		return false
	}
	if previous.pointer != nil && next.pointer != nil {
		return previous.pointer.Surface == next.pointer.Surface &&
			previous.pointer.Buttons == next.pointer.Buttons
	}
	return previous.wheel != nil && next.wheel != nil &&
		previous.wheel.Surface == next.wheel.Surface &&
		previous.wheel.Modifiers == next.wheel.Modifiers &&
		previous.wheel.Buttons == next.wheel.Buttons
}

func coalescePointerWork(previous, next *pointerWork) {
	if previous == nil || next == nil || previous.wheel == nil || next.wheel == nil {
		return
	}
	next.wheel.DeltaX = clampWheelDelta(previous.wheel.DeltaX + next.wheel.DeltaX)
	next.wheel.DeltaY = clampWheelDelta(previous.wheel.DeltaY + next.wheel.DeltaY)
}

func clampWheelDelta(value float64) float64 {
	return math.Max(-4, math.Min(4, value))
}

func (in *pointerInput) cancel(wait bool) {
	work := pointerWork{cancel: true}
	if wait {
		work.done = make(chan struct{})
	}
	in.mu.Lock()
	in.queue = append(in.queue, work)
	in.mu.Unlock()
	select {
	case in.wake <- struct{}{}:
	default:
	}
	if work.done != nil {
		select {
		case <-work.done:
		case <-time.After(2 * time.Second):
		}
	}
}

func (in *pointerInput) cancelClient(client *transport.Client) {
	if client != nil {
		in.cancel(false)
	}
}

func (in *pointerInput) close() {
	in.cancel(true)
	select {
	case <-in.stop:
	default:
		close(in.stop)
	}
}

func (in *pointerInput) run() {
	for {
		select {
		case <-in.wake:
		case <-in.stop:
			return
		}
		for {
			in.mu.Lock()
			if len(in.queue) == 0 {
				in.mu.Unlock()
				break
			}
			work := in.queue[0]
			copy(in.queue, in.queue[1:])
			in.queue = in.queue[:len(in.queue)-1]
			in.mu.Unlock()
			if work.cancel {
				in.cancelActive()
				if work.done != nil {
					close(work.done)
				}
				continue
			}
			in.process(work)
		}
	}
}

func validPointerCommand(command *protocol.PointerCommand) bool {
	if command == nil || command.Sequence == 0 || command.Surface == 0 ||
		command.TimestampNS == 0 || command.TimestampNS > math.MaxInt64 ||
		!finiteUnit(command.X) || !finiteUnit(command.Y) || command.Buttons < 0 ||
		command.Buttons > 31 || command.Modifiers < 0 || command.Modifiers > 15 ||
		command.ClickCount < 0 || command.ClickCount > 3 {
		return false
	}
	validButton := command.Button == "none" || command.Button == "left" ||
		command.Button == "middle" || command.Button == "right" ||
		command.Button == "back" || command.Button == "forward"
	if !validButton {
		return false
	}
	switch command.Phase {
	case "move", "leave":
		return command.Button == "none" && command.ClickCount == 0
	case "down", "up":
		return command.Button != "none" && command.ClickCount > 0
	default:
		return false
	}
}

func validWheelCommand(command *protocol.WheelCommand) bool {
	return command != nil && command.Sequence != 0 && command.Surface != 0 &&
		command.TimestampNS != 0 && command.TimestampNS <= math.MaxInt64 &&
		finiteUnit(command.X) && finiteUnit(command.Y) &&
		!math.IsNaN(command.DeltaX) && !math.IsInf(command.DeltaX, 0) &&
		!math.IsNaN(command.DeltaY) && !math.IsInf(command.DeltaY, 0) &&
		command.DeltaX >= -4 && command.DeltaX <= 4 &&
		command.DeltaY >= -4 && command.DeltaY <= 4 &&
		command.Buttons >= 0 && command.Buttons <= 31 &&
		command.Modifiers >= 0 && command.Modifiers <= 15
}

func (in *pointerInput) process(work pointerWork) {
	if work.command != nil {
		in.processCommand(work)
		return
	}
	if (work.pointer != nil && !validPointerCommand(work.pointer)) ||
		(work.wheel != nil && !validWheelCommand(work.wheel)) {
		in.cancelActive()
		return
	}
	select {
	case <-work.client.Closed():
		in.cancelActive()
		return
	default:
	}
	session, surface := in.b.touch.activeContext()
	sequence, clientNS := uint64(0), uint64(0)
	if work.pointer != nil {
		sequence, clientNS = work.pointer.Sequence, work.pointer.TimestampNS
	} else {
		sequence, clientNS = work.wheel.Sequence, work.wheel.TimestampNS
	}
	if session == "" || surface == 0 ||
		(work.pointer != nil && work.pointer.Surface != surface) ||
		(work.wheel != nil && work.wheel.Surface != surface) {
		in.cancelActive()
		return
	}
	if in.owner != work.client || in.session != session || in.surface != surface {
		in.cancelActive()
		viewport, err := in.b.touch.loadViewport(session)
		if err != nil {
			log.Printf("pointer: read viewport: %v", err)
			return
		}
		in.owner, in.session, in.surface = work.client, session, surface
		in.viewport = viewport
	}
	if sequence <= in.lastSeq || (in.lastClientNS != 0 && clientNS < in.lastClientNS) {
		in.cancelActive()
		return
	}
	if work.pointer != nil {
		if in.dispatchPointer(work.pointer) {
			in.noteAccepted("pointer", work.pointer, work.receivedNS)
		}
	} else {
		if in.dispatchWheel(work.wheel) {
			in.noteAccepted("wheel", work.wheel, work.receivedNS)
		}
	}
	in.lastSeq = sequence
	in.lastClientNS = clientNS
}

func (in *pointerInput) processCommand(work pointerWork) {
	select {
	case <-work.client.Closed():
		in.cancelActive()
		return
	default:
	}
	session, _ := in.b.touch.activeContext()
	if session == "" {
		return
	}
	if (in.owner != nil && in.owner != work.client) || (in.session != "" && in.session != session) {
		in.cancelActive()
	}
	if !in.b.dispatchOrderedPageInput(session, work.command) {
		return
	}
	in.noteAccepted(work.command.Kind(), work.command, work.receivedNS)
}

func (in *pointerInput) dispatchPointer(command *protocol.PointerCommand) bool {
	typ := map[string]string{"move": "mouseMoved", "down": "mousePressed", "up": "mouseReleased", "leave": "mouseMoved"}[command.Phase]
	x := in.viewport.OffsetX + clampUnit(command.X)*in.viewport.ClientWidth
	y := in.viewport.OffsetY + clampUnit(command.Y)*in.viewport.ClientHeight
	if command.Phase == "leave" {
		x, y = -1, -1
	}
	params := map[string]any{
		"type": typ, "x": x, "y": y, "button": command.Button,
		"buttons": command.Buttons, "modifiers": command.Modifiers,
		"clickCount": command.ClickCount, "pointerType": "mouse",
	}
	if err := in.b.cdp.Dispatch(in.session, "Input.dispatchMouseEvent", params); err != nil {
		log.Printf("pointer: dispatch %s: %v", command.Phase, err)
		in.cancelActive()
		return false
	}
	in.buttons = command.Buttons
	switch command.Phase {
	case "down":
		in.b.noteMotionPhase("begin")
	case "move":
		if command.Buttons != 0 {
			in.b.noteMotionPhase("move")
		}
	case "up":
		if command.Buttons == 0 {
			in.b.noteMotionPhase("end")
		}
	}
	return true
}

func (in *pointerInput) dispatchWheel(command *protocol.WheelCommand) bool {
	params := map[string]any{
		"type":   "mouseWheel",
		"x":      in.viewport.OffsetX + clampUnit(command.X)*in.viewport.ClientWidth,
		"y":      in.viewport.OffsetY + clampUnit(command.Y)*in.viewport.ClientHeight,
		"deltaX": command.DeltaX * in.viewport.ClientWidth,
		"deltaY": command.DeltaY * in.viewport.ClientHeight,
		"button": "none", "buttons": command.Buttons,
		"modifiers": command.Modifiers, "pointerType": "mouse",
	}
	if err := in.b.cdp.Dispatch(in.session, "Input.dispatchMouseEvent", params); err != nil {
		log.Printf("pointer: dispatch wheel: %v", err)
		in.cancelActive()
		return false
	}
	return true
}

func (in *pointerInput) noteAccepted(kind string, command protocol.Command, receivedNS uint64) {
	in.b.noteClientMessage(kind)
	in.b.noteRenderInput()
	iid, _ := command.Causal()
	if iid == 0 {
		return
	}
	in.b.perfMu.Lock()
	in.b.interactionID = iid
	in.b.interactionInputNS = receivedNS
	in.b.interactionCDPNS = telemetry.MonoNS()
	in.b.perfMu.Unlock()
}

func (in *pointerInput) cancelActive() {
	if in.session != "" && in.buttons != 0 {
		remaining := in.buttons
		for _, button := range pressedButtons(in.buttons) {
			remaining &^= button.mask
			_ = in.b.cdp.Dispatch(in.session, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseReleased", "x": -1, "y": -1,
				"button": button.name, "buttons": remaining,
				"clickCount": 1, "pointerType": "mouse",
			})
		}
		in.b.noteMotionPhase("end")
	}
	in.owner = nil
	in.session = ""
	in.surface = 0
	in.lastSeq = 0
	in.lastClientNS = 0
	in.viewport = cssVisualViewport{}
	in.buttons = 0
}

type pressedButton struct {
	mask int
	name string
}

func pressedButtons(buttons int) []pressedButton {
	var pressed []pressedButton
	for _, candidate := range []pressedButton{
		{1, "left"}, {2, "right"}, {4, "middle"}, {8, "back"}, {16, "forward"},
	} {
		if buttons&candidate.mask != 0 {
			pressed = append(pressed, candidate)
		}
	}
	return pressed
}
