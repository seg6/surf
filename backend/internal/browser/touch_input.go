package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"surf-backend/internal/protocol"
	"surf-backend/internal/telemetry"
	"surf-backend/internal/transport"
)

const (
	maxTouchContacts = 5
	touchQueueLimit  = 64
	// A renderer can consume several asynchronously-written CDP commands in
	// one task. Keep the final move and release in separate turns so mobile
	// gesture recognition can commit velocity before touchEnd clears it.
	touchReleaseSettle = 12 * time.Millisecond
)

type cssVisualViewport struct {
	OffsetX      float64 `json:"offsetX"`
	OffsetY      float64 `json:"offsetY"`
	ClientWidth  float64 `json:"clientWidth"`
	ClientHeight float64 `json:"clientHeight"`
	Scale        float64 `json:"scale"`
}

func (v cssVisualViewport) valid() bool {
	return v.ClientWidth > 0 && v.ClientHeight > 0 &&
		!math.IsNaN(v.ClientWidth) && !math.IsNaN(v.ClientHeight)
}

func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func finiteUnit(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func mapTouchPoint(point protocol.TouchPoint, viewport cssVisualViewport) (map[string]any, error) {
	if point.ID < 0 || point.ID > math.MaxInt32 || !finiteUnit(point.X) || !finiteUnit(point.Y) ||
		!finiteUnit(point.RadiusX) || !finiteUnit(point.RadiusY) || !finiteUnit(point.Force) {
		return nil, errors.New("invalid touch point")
	}
	radiusX := math.Max(1, clampUnit(point.RadiusX)*viewport.ClientWidth)
	radiusY := math.Max(1, clampUnit(point.RadiusY)*viewport.ClientHeight)
	force := point.Force
	if force <= 0 {
		force = 0.5
	}
	force = clampUnit(force)
	return map[string]any{
		"id":      point.ID,
		"x":       viewport.OffsetX + clampUnit(point.X)*viewport.ClientWidth,
		"y":       viewport.OffsetY + clampUnit(point.Y)*viewport.ClientHeight,
		"radiusX": radiusX, "radiusY": radiusY, "force": force,
	}, nil
}

func touchPositionChanged(before, after protocol.TouchPoint, viewport cssVisualViewport) bool {
	return math.Abs(before.X-after.X)*viewport.ClientWidth >= 0.5 ||
		math.Abs(before.Y-after.Y)*viewport.ClientHeight >= 0.5
}

// finalMovePoints turns UIKit's lift coordinates into a complete active-touch
// snapshot. UITouch can advance between the last touchesMoved callback and
// touchesEnded; CDP does not use touchEnd coordinates as a motion sample.
func (in *touchInput) finalMovePoints(ended []protocol.TouchPoint) ([]any, bool, error) {
	snapshot := make(map[int]protocol.TouchPoint, len(in.active))
	for id, point := range in.active {
		snapshot[id] = point
	}
	moved := false
	for _, point := range ended {
		if before, ok := snapshot[point.ID]; ok && touchPositionChanged(before, point, in.viewport) {
			moved = true
		}
		snapshot[point.ID] = point
	}
	if !moved {
		return nil, false, nil
	}
	ids := make([]int, 0, len(snapshot))
	for id := range snapshot {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	points := make([]any, 0, len(ids))
	for _, id := range ids {
		mapped, err := mapTouchPoint(snapshot[id], in.viewport)
		if err != nil {
			return nil, false, err
		}
		points = append(points, mapped)
	}
	return points, true, nil
}

type touchWork struct {
	client     *transport.Client
	command    *protocol.TouchCommand
	receivedNS uint64
	cancel     bool
	done       chan struct{}
}

// touchInput is the sole owner of Chromium touch state. Its queue is separate
// from browser chrome commands: lifecycle edges remain reliable while stale
// move samples can be replaced before they reach Chromium. CDP writes stay in
// this worker's order, but never wait for a renderer acknowledgement: pages
// with expensive touch listeners must not hold later moves or the final lift
// hostage and destroy Chromium's gesture velocity.
type touchInput struct {
	b *Controller

	mu    sync.Mutex
	queue []touchWork
	wake  chan struct{}
	stop  chan struct{}

	owner        *transport.Client
	session      string
	surface      uint32
	lastSeq      uint64
	active       map[int]protocol.TouchPoint
	viewport     cssVisualViewport
	clockOffset  int64
	lastEventNS  int64
	lastClientNS uint64
	lastMoveSent time.Time
}

func newTouchInput(b *Controller) *touchInput {
	in := &touchInput{b: b, wake: make(chan struct{}, 1), stop: make(chan struct{}), active: map[int]protocol.TouchPoint{}}
	go in.run()
	return in
}

func (in *touchInput) enqueue(client *transport.Client, command *protocol.TouchCommand, receivedNS uint64) {
	work := touchWork{client: client, command: command, receivedNS: receivedNS}
	in.mu.Lock()
	if command.Phase == "move" && len(in.queue) > 0 {
		last := &in.queue[len(in.queue)-1]
		if last.command != nil && last.client == client && last.command.Phase == "move" &&
			last.command.Surface == command.Surface {
			*last = work
			in.mu.Unlock()
			return
		}
	}
	if len(in.queue) >= touchQueueLimit {
		removed := false
		for i, queued := range in.queue {
			if queued.command != nil && queued.command.Phase == "move" {
				copy(in.queue[i:], in.queue[i+1:])
				in.queue = in.queue[:len(in.queue)-1]
				removed = true
				break
			}
		}
		if !removed {
			in.mu.Unlock()
			client.Close()
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

func (in *touchInput) cancel(wait bool) {
	work := touchWork{cancel: true}
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

func (in *touchInput) cancelClient(client *transport.Client) {
	if client == nil {
		return
	}
	// Surf exposes one shared browser surface. A disconnect is therefore a
	// cancellation boundary even if another idle client happened to be present.
	in.cancel(false)
}

func (in *touchInput) close() {
	in.cancel(true)
	select {
	case <-in.stop:
	default:
		close(in.stop)
	}
}

func (in *touchInput) run() {
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
			} else {
				in.process(work)
			}
			if work.done != nil {
				close(work.done)
			}
		}
	}
}

func (in *touchInput) activeContext() (session string, surface uint32) {
	in.b.mu.Lock()
	if tab := in.b.tabs[in.b.activeID]; tab != nil {
		session = tab.Session
	}
	in.b.mu.Unlock()
	return session, in.b.video.Generation()
}

func (in *touchInput) loadViewport(session string) (cssVisualViewport, error) {
	raw, err := in.b.cdp.Call(session, "Page.getLayoutMetrics", nil)
	if err != nil {
		return cssVisualViewport{}, err
	}
	var metrics struct {
		CSSVisualViewport cssVisualViewport `json:"cssVisualViewport"`
	}
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return cssVisualViewport{}, err
	}
	if !metrics.CSSVisualViewport.valid() {
		return cssVisualViewport{}, errors.New("invalid CSS visual viewport")
	}
	return metrics.CSSVisualViewport, nil
}

func validTouchCommand(command *protocol.TouchCommand) bool {
	if command == nil || command.Sequence == 0 || command.Surface == 0 ||
		command.TimestampNS == 0 || command.TimestampNS > math.MaxInt64 {
		return false
	}
	switch command.Phase {
	case "start", "move", "end":
		return len(command.Points) > 0 && len(command.Points) <= maxTouchContacts
	case "cancel":
		return len(command.Points) == 0
	default:
		return false
	}
}

func (in *touchInput) process(work touchWork) {
	command := work.command
	if !validTouchCommand(command) {
		in.cancelActive()
		return
	}
	select {
	case <-work.client.Closed():
		in.cancelActive()
		return
	default:
	}
	session, surface := in.activeContext()
	if session == "" || surface != command.Surface {
		in.cancelActive()
		return
	}
	if in.owner == nil {
		if command.Phase != "start" {
			return
		}
		viewport, err := in.loadViewport(session)
		if err != nil {
			log.Printf("touch: read viewport: %v", err)
			return
		}
		in.owner, in.session, in.surface = work.client, session, surface
		in.viewport = viewport
		in.active = map[int]protocol.TouchPoint{}
		in.lastSeq = 0
		in.clockOffset = time.Now().UnixNano() - int64(command.TimestampNS)
		in.lastEventNS = 0
		in.lastMoveSent = time.Time{}
		in.b.noteMotionPhase("begin")
	}
	if in.owner != work.client || in.session != session || in.surface != surface || command.Sequence <= in.lastSeq {
		in.cancelActive()
		return
	}
	if in.lastClientNS != 0 && command.TimestampNS < in.lastClientNS {
		in.cancelActive()
		return
	}
	if len(in.active) > 1 || (command.Phase == "start" && len(in.active)+len(command.Points) > 1) {
		viewport, err := in.loadViewport(session)
		if err != nil {
			in.cancelActive()
			return
		}
		in.viewport = viewport
	}
	seen := map[int]bool{}
	for _, point := range command.Points {
		if seen[point.ID] {
			in.cancelActive()
			return
		}
		seen[point.ID] = true
		_, exists := in.active[point.ID]
		if (command.Phase == "start" && exists) || (command.Phase != "start" && !exists) {
			in.cancelActive()
			return
		}
	}
	if command.Phase == "move" && len(seen) != len(in.active) {
		in.cancelActive()
		return
	}
	points := make([]any, 0, len(command.Points))
	for _, point := range command.Points {
		mapped, err := mapTouchPoint(point, in.viewport)
		if err != nil {
			in.cancelActive()
			return
		}
		points = append(points, mapped)
	}
	eventNS := in.clockOffset + int64(command.TimestampNS)
	nowNS := time.Now().UnixNano()
	if eventNS > nowNS {
		eventNS = nowNS
	}
	if eventNS <= in.lastEventNS {
		eventNS = in.lastEventNS + 1
	}
	if command.Phase == "end" {
		finalPoints, moved, err := in.finalMovePoints(command.Points)
		if err != nil {
			in.cancelActive()
			return
		}
		if moved {
			if err := in.b.cdp.Dispatch(session, "Input.dispatchTouchEvent", map[string]any{
				"type": "touchMove", "touchPoints": finalPoints,
				"timestamp": float64(eventNS) / float64(time.Second),
			}); err != nil {
				log.Printf("touch: dispatch final move: %v", err)
				in.b.noteMotionPhase("end")
				in.reset()
				return
			}
			in.lastMoveSent = time.Now()
		}
		if !in.lastMoveSent.IsZero() {
			if remaining := touchReleaseSettle - time.Since(in.lastMoveSent); remaining > 0 {
				time.Sleep(remaining)
			}
		}
		// Chromium's final release is an empty active-contact snapshot. Keep
		// ended points only for partial multi-touch lifts, which Chromium uses
		// to identify the changed contact while others remain active.
		if len(command.Points) == len(in.active) {
			points = []any{}
		}
		if moved {
			eventNS++
		}
	}
	params := map[string]any{
		"type": "touch" + stringsTitle(command.Phase), "touchPoints": points,
		"timestamp": float64(eventNS) / float64(time.Second),
	}
	if command.Phase == "cancel" {
		params["touchPoints"] = []any{}
	}
	if err := in.b.cdp.Dispatch(session, "Input.dispatchTouchEvent", params); err != nil {
		log.Printf("touch: dispatch %s: %v", command.Phase, err)
		in.b.noteMotionPhase("end")
		in.reset()
		return
	}
	if command.Phase == "move" {
		in.lastMoveSent = time.Now()
	}
	in.lastSeq = command.Sequence
	in.lastEventNS = eventNS
	in.lastClientNS = command.TimestampNS
	for _, point := range command.Points {
		if command.Phase == "end" {
			delete(in.active, point.ID)
		} else if command.Phase != "cancel" {
			in.active[point.ID] = point
		}
	}
	in.noteAccepted(work)
	if command.Phase == "move" {
		in.b.noteMotionPhase("move")
	}
	if command.Phase == "cancel" || len(in.active) == 0 {
		in.b.noteMotionPhase("end")
		in.reset()
	}
}

func stringsTitle(phase string) string {
	switch phase {
	case "start":
		return "Start"
	case "move":
		return "Move"
	case "end":
		return "End"
	case "cancel":
		return "Cancel"
	default:
		panic(fmt.Sprintf("invalid touch phase %q", phase))
	}
}

func (in *touchInput) noteAccepted(work touchWork) {
	in.b.noteClientMessage("touch")
	in.b.noteRenderInput()
	iid, _ := work.command.Causal()
	if iid == 0 {
		return
	}
	in.b.perfMu.Lock()
	in.b.interactionID = iid
	in.b.interactionInputNS = work.receivedNS
	in.b.interactionCDPNS = telemetry.MonoNS()
	in.b.perfMu.Unlock()
}

func (in *touchInput) cancelActive() {
	if in.owner != nil && in.session != "" && len(in.active) > 0 {
		_ = in.b.cdp.Dispatch(in.session, "Input.dispatchTouchEvent", map[string]any{
			"type": "touchCancel", "touchPoints": []any{},
		})
		in.b.noteMotionPhase("end")
	}
	in.reset()
}

func (in *touchInput) reset() {
	in.owner = nil
	in.session = ""
	in.surface = 0
	in.lastSeq = 0
	in.active = map[int]protocol.TouchPoint{}
	in.viewport = cssVisualViewport{}
	in.clockOffset = 0
	in.lastEventNS = 0
	in.lastClientNS = 0
	in.lastMoveSent = time.Time{}
}
