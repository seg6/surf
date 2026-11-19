package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/protocol"
)

func useMobileEmulation(t *testing.T, client *cdp.Client, session string) {
	t.Helper()
	for method, params := range map[string]map[string]any{
		"Emulation.setDeviceMetricsOverride": {
			"width": 400, "height": 600, "deviceScaleFactor": 1, "mobile": true,
			"screenWidth": 400, "screenHeight": 600,
		},
		"Emulation.setTouchEmulationEnabled": {"enabled": true, "maxTouchPoints": 5},
	} {
		if _, err := client.Call(session, method, params); err != nil {
			t.Fatal(err)
		}
	}
}

func serveFixture(t *testing.T, client *cdp.Client, session, page string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, page)
	}))
	t.Cleanup(server.Close)
	if _, err := client.Call(session, "Page.navigate", map[string]any{"url": server.URL}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
}

func dispatchTouch(t *testing.T, client *cdp.Client, session, typ string, points ...any) {
	t.Helper()
	if points == nil {
		points = []any{}
	}
	if _, err := client.Call(session, "Input.dispatchTouchEvent", map[string]any{"type": typ, "touchPoints": points}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
}

func touchFinger(id int, x, y float64) map[string]any {
	return map[string]any{"id": id, "x": x, "y": y, "radiusX": 5, "radiusY": 5, "force": .5}
}

// A body that is its own scroll container must not get overscroll-behavior:
// none, or it swallows the touch scroll meant for the viewport (DuckDuckGo).
func TestOverscrollShimKeepsTouchScrollWithHiddenBodyOverflow(t *testing.T) {
	client, session := launchInputTestBrowser(t)
	useMobileEmulation(t, client, session)
	if _, err := client.Call(session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": overscrollShim}); err != nil {
		t.Fatal(err)
	}
	serveFixture(t, client, session, `<!doctype html><meta name=viewport content="width=device-width,initial-scale=1">
<style>html{overflow-x:hidden}body{margin:0;overflow-x:hidden;overflow-y:hidden;height:3000px;background:linear-gradient(#111,#eee)}</style>`)
	dispatchTouch(t, client, session, "touchStart", touchFinger(0, 200, 500))
	for y := 480.0; y >= 200; y -= 20 {
		time.Sleep(16 * time.Millisecond)
		dispatchTouch(t, client, session, "touchMove", touchFinger(0, 200, y))
	}
	dispatchTouch(t, client, session, "touchEnd")
	time.Sleep(500 * time.Millisecond)
	got, err := client.EvaluateString(session, `String(window.scrollY)`)
	if err != nil {
		t.Fatal(err)
	}
	if y, _ := strconv.ParseFloat(got, 64); y < 100 {
		t.Fatalf("touch swipe did not scroll the page: scrollY=%s", got)
	}
}

func TestOverscrollShimPinsBodyOnlyWhenItDefinesTheViewport(t *testing.T) {
	client, session := launchInputTestBrowser(t)
	useMobileEmulation(t, client, session)
	if _, err := client.Call(session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": overscrollShim}); err != nil {
		t.Fatal(err)
	}
	for page, want := range map[string]string{
		`<!doctype html><body style="overflow-y:scroll">x`:       "none",
		`<!doctype html><html style="overflow-x:hidden"><body>x`: "",
	} {
		serveFixture(t, client, session, page)
		got, err := client.EvaluateString(session, `document.documentElement.style.overscrollBehavior+"|"+document.body.style.overscrollBehavior`)
		if err != nil {
			t.Fatal(err)
		}
		if got != "none|"+want {
			t.Fatalf("overscroll for %q = %q want none|%s", page, got, want)
		}
	}
}

// Input.dispatchTouchEvent and dispatchMouseEvent take coordinates relative
// to the visual viewport; adding its offset misses once the page is zoomed.
func TestChromiumInputCoordinatesAreVisualViewportRelativeWhenZoomed(t *testing.T) {
	client, session := launchInputTestBrowser(t)
	useMobileEmulation(t, client, session)
	serveFixture(t, client, session, `<!doctype html><meta name=viewport content="width=device-width,initial-scale=1,user-scalable=yes">
<body style=margin:0><div id=g style="position:relative;width:1200px;height:1200px"></div>
<script>window.hit=[];for(let i=0;i<12;i++)for(let j=0;j<12;j++){const b=document.createElement('button');b.style.cssText='position:absolute;width:96px;height:96px;left:'+(j*100)+'px;top:'+(i*100)+'px';b.onclick=()=>hit.push(i+','+j);g.appendChild(b)}</script>`)
	dispatchTouch(t, client, session, "touchStart", touchFinger(1, 150, 300))
	dispatchTouch(t, client, session, "touchStart", touchFinger(2, 250, 300))
	for i := 1; i <= 6; i++ {
		dispatchTouch(t, client, session, "touchMove",
			touchFinger(1, 150-float64(i)*20, 300+float64(i)*5), touchFinger(2, 250+float64(i)*20, 300+float64(i)*5))
	}
	dispatchTouch(t, client, session, "touchEnd", touchFinger(2, 370, 330))
	dispatchTouch(t, client, session, "touchEnd")
	time.Sleep(time.Second)
	raw, err := client.Call(session, "Page.getLayoutMetrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	var metrics struct {
		Viewport cssVisualViewport `json:"cssVisualViewport"`
	}
	if json.Unmarshal(raw, &metrics) != nil || metrics.Viewport.Scale < 2 || metrics.Viewport.OffsetX == 0 {
		t.Fatalf("page was not zoomed and panned: %s", raw)
	}
	v := metrics.Viewport
	// Aim at the centre of the button nearest the middle of the screen.
	row, col := int((v.OffsetY+v.ClientHeight/2)/100), int((v.OffsetX+v.ClientWidth/2)/100)
	fx, fy := (float64(col*100+48)-v.OffsetX)/v.ClientWidth, (float64(row*100+48)-v.OffsetY)/v.ClientHeight
	want := fmt.Sprintf(`["%d,%d"]`, row, col)
	mapped, err := mapTouchPoint(protocol.TouchPoint{X: fx, Y: fy, RadiusX: .01, RadiusY: .01}, v)
	if err != nil {
		t.Fatal(err)
	}
	dispatchTouch(t, client, session, "touchStart", mapped)
	time.Sleep(60 * time.Millisecond)
	dispatchTouch(t, client, session, "touchEnd")
	time.Sleep(300 * time.Millisecond)
	if got, _ := client.EvaluateString(session, `JSON.stringify(hit.splice(0))`); got != want {
		t.Fatalf("zoomed touch tap hit %s want %s (viewport %+v)", got, want, v)
	}
	for _, typ := range []string{"mouseMoved", "mousePressed", "mouseReleased"} {
		if _, err := client.Call(session, "Input.dispatchMouseEvent", map[string]any{
			"type": typ, "x": mapped["x"], "y": mapped["y"], "button": "left", "clickCount": 1, "pointerType": "mouse",
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if got, _ := client.EvaluateString(session, `JSON.stringify(hit.splice(0))`); got != want {
		t.Fatalf("zoomed mouse click hit %s want %s", got, want)
	}
}
