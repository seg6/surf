package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/transport"
)

// launchInputTestBrowser keeps real-Chromium checks opt-in for portable CI.
// Run them locally with SURF_TEST_BROWSER_INPUT=/path/to/chromium.
func launchInputTestBrowser(t *testing.T) (*cdp.Client, string) {
	t.Helper()
	path := os.Getenv("SURF_TEST_BROWSER_INPUT")
	if path == "" {
		t.Skip("set SURF_TEST_BROWSER_INPUT to a Chrome/Chromium executable")
	}
	client, process, err := cdp.Launch(cdp.LaunchConfig{
		ChromePath: path, Profile: t.TempDir(), W: 400, H: 600,
		NoSandbox: os.Geteuid() == 0, ExtraArgs: []string{"--site-per-process"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close()
		_ = process.Kill()
	})
	raw, err := client.Call("", "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil {
		t.Fatal(err)
	}
	var target struct {
		TargetID string `json:"targetId"`
	}
	if json.Unmarshal(raw, &target) != nil || target.TargetID == "" {
		t.Fatalf("Target.createTarget=%s", raw)
	}
	raw, err = client.Call("", "Target.attachToTarget", map[string]any{
		"targetId": target.TargetID, "flatten": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(raw, &attached) != nil || attached.SessionID == "" {
		t.Fatalf("Target.attachToTarget=%s", raw)
	}
	if _, err := client.Call(attached.SessionID, "Page.enable", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(attached.SessionID, "Runtime.enable", nil); err != nil {
		t.Fatal(err)
	}
	return client, attached.SessionID
}

func TestEditableObserverFollowsOOPIFFocus(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<input id="login" type="email" autofocus><script>setInterval(function(){document.getElementById('login').focus()},50)</script>`)
	}))
	defer child.Close()
	childURL := strings.Replace(child.URL, "127.0.0.1", "localhost", 1)
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<iframe src="%s"></iframe>`, childURL)
	}))
	defer parent.Close()

	client, session := launchInputTestBrowser(t)
	tab := &Tab{ID: 1, Session: session}
	browser := &Controller{
		cdp: client, hub: transport.New(), tabs: map[int]*Tab{1: tab},
		bySession: map[string]*Tab{session: tab}, activeID: 1,
	}
	type report struct {
		session string
		payload string
	}
	reports := make(chan report, 16)
	children := make(chan string, 4)
	client.OnEvent(func(event cdp.Event) {
		if event.Method == "Target.attachedToTarget" {
			var attached struct {
				SessionID string `json:"sessionId"`
			}
			if json.Unmarshal(event.Params, &attached) == nil && attached.SessionID != "" {
				children <- attached.SessionID
			}
		}
		if event.Method == "Runtime.bindingCalled" {
			var binding struct {
				Name    string `json:"name"`
				Payload string `json:"payload"`
			}
			if json.Unmarshal(event.Params, &binding) == nil && binding.Name == editableBinding {
				reports <- report{session: event.SessionID, payload: binding.Payload}
			}
		}
		// Only route the events this focused harness has initialized Controller
		// state for. Main-frame navigation normally also owns touch, store, and
		// capture state, which is intentionally absent here.
		switch event.Method {
		case "Target.attachedToTarget", "Target.detachedFromTarget", "Runtime.executionContextCreated", "Runtime.bindingCalled":
			browser.onEvent(event)
		}
	})
	browser.setupEditableObserver(session)
	browser.setupEditableAutoAttach(session)
	if _, err := client.Call(session, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Page.navigate", map[string]any{"url": parent.URL}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	focusTicker := time.NewTicker(100 * time.Millisecond)
	defer focusTicker.Stop()
	childSession := ""
	lastChildState := "no OOPIF session"
	for {
		select {
		case childSession = <-children:
		case <-focusTicker.C:
			if childSession != "" {
				_, _ = client.Call(session, "Runtime.evaluate", map[string]any{
					"expression": `document.querySelector('iframe').focus()`, "userGesture": true,
				})
				lastChildState, _ = client.EvaluateString(childSession, `(function(){
					var e=document.getElementById('login');if(e)e.focus();
					return JSON.stringify({url:location.href,ready:document.readyState,binding:typeof window.__surfEditableChanged,observer:!!window.__surfEditableObserver,hasInput:!!e,active:document.activeElement&&document.activeElement.id});
				})()`)
			}
		case got := <-reports:
			var state struct {
				On   bool   `json:"on"`
				Kind string `json:"kind"`
			}
			if got.session != session && json.Unmarshal([]byte(got.payload), &state) == nil && state.On {
				if state.Kind != "email" || !browser.isActiveSession(got.session) {
					t.Fatalf("OOPIF editable session=%q payload=%s", got.session, got.payload)
				}
				return
			}
		case <-deadline:
			t.Fatalf("editable observer did not report focused OOPIF input: %s", lastChildState)
		}
	}
}

func TestEditableObserverFollowsOpenShadowFocus(t *testing.T) {
	client, session := launchInputTestBrowser(t)
	bindings := make(chan string, 8)
	client.OnEvent(func(event cdp.Event) {
		if event.SessionID != session || event.Method != "Runtime.bindingCalled" {
			return
		}
		var binding struct {
			Name    string `json:"name"`
			Payload string `json:"payload"`
		}
		if json.Unmarshal(event.Params, &binding) == nil && binding.Name == editableBinding {
			bindings <- binding.Payload
		}
	})
	if _, err := client.Call(session, "Runtime.addBinding", map[string]any{"name": editableBinding}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Runtime.evaluate", map[string]any{"expression": editableObserver}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Runtime.evaluate", map[string]any{"expression": `(function(){
		var host=document.createElement('div'),root=host.attachShadow({mode:'open'}),input=document.createElement('input');
		input.type='password';input.style.cssText='position:absolute;left:40px;top:60px;width:120px;height:30px';
		root.appendChild(input);document.body.appendChild(host);input.focus();
	})()`}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case payload := <-bindings:
			var state struct {
				On   bool      `json:"on"`
				Kind string    `json:"kind"`
				Rect []float64 `json:"rect"`
			}
			if json.Unmarshal([]byte(payload), &state) == nil && state.On {
				if state.Kind != "password" || len(state.Rect) != 4 {
					t.Fatalf("editable payload=%s", payload)
				}
				return
			}
		case <-deadline:
			t.Fatal("editable observer did not report focused shadow input")
		}
	}
}

func TestChromiumTouchSequenceSupportsPartialLiftAndPinch(t *testing.T) {
	client, session := launchInputTestBrowser(t)
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
	setup := `(function(){
		var meta=document.createElement('meta');meta.name='viewport';meta.content='width=device-width,initial-scale=1,user-scalable=yes';document.head.appendChild(meta);
		window.__touchEvents=[];
		for(var i=0,n=['touchstart','touchmove','touchend','touchcancel'];i<n.length;i++)document.addEventListener(n[i],function(e){
			window.__touchEvents.push({type:e.type,touches:Array.prototype.map.call(e.touches,function(t){return t.identifier}),changed:Array.prototype.map.call(e.changedTouches,function(t){return t.identifier})});
		},true);
	})()`
	if _, err := client.Call(session, "Runtime.evaluate", map[string]any{"expression": setup}); err != nil {
		t.Fatal(err)
	}
	point := func(id int, x, y float64) map[string]any {
		return map[string]any{"id": id, "x": x, "y": y, "radiusX": 5, "radiusY": 5, "force": .5}
	}
	sequence := []struct {
		typ    string
		points []any
	}{
		{"touchStart", []any{point(1, 100, 150)}},
		{"touchStart", []any{point(2, 300, 150)}},
		{"touchMove", []any{point(1, 70, 150), point(2, 330, 150)}},
		{"touchMove", []any{point(1, 40, 150), point(2, 360, 150)}},
		{"touchEnd", []any{point(2, 360, 150)}},
		{"touchMove", []any{point(1, 45, 155)}},
		{"touchEnd", []any{point(1, 45, 155)}},
	}
	for _, event := range sequence {
		if _, err := client.Call(session, "Input.dispatchTouchEvent", map[string]any{
			"type": event.typ, "touchPoints": event.points,
		}); err != nil {
			t.Fatalf("%s: %v", event.typ, err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	result, err := client.EvaluateString(session, `JSON.stringify({scale:visualViewport.scale,events:window.__touchEvents})`)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Scale  float64 `json:"scale"`
		Events []struct {
			Type    string `json:"type"`
			Touches []int  `json:"touches"`
			Changed []int  `json:"changed"`
		} `json:"events"`
	}
	if json.Unmarshal([]byte(result), &state) != nil {
		t.Fatalf("touch result=%s", result)
	}
	if state.Scale <= 1.1 {
		t.Fatalf("pinch scale=%f want > 1.1", state.Scale)
	}
	partialLift := false
	for _, event := range state.Events {
		if event.Type == "touchend" && len(event.Touches) == 1 && len(event.Changed) == 1 && event.Changed[0] == 2 {
			partialLift = true
		}
	}
	if !partialLift {
		t.Fatalf("partial touch lift missing: %s", result)
	}
}

func TestChromiumTouchSequenceCommitsFling(t *testing.T) {
	client, session := launchInputTestBrowser(t)
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
	if _, err := client.Call(session, "Runtime.evaluate", map[string]any{"expression": `(function(){
		document.documentElement.style.cssText='margin:0;min-height:12000px';
		document.body.style.cssText='margin:0;min-height:12000px;background:linear-gradient(#111,#eee)';
		window.scrollTo(0,0);
	})()`}); err != nil {
		t.Fatal(err)
	}
	point := func(y float64) map[string]any {
		return map[string]any{"id": 1, "x": 200, "y": y, "radiusX": 5, "radiusY": 5, "force": .5}
	}
	start := time.Now()
	dispatch := func(typ string, points []any) {
		t.Helper()
		if _, err := client.Call(session, "Input.dispatchTouchEvent", map[string]any{
			"type": typ, "touchPoints": points,
			"timestamp": float64(time.Now().UnixNano()) / float64(time.Second),
		}); err != nil {
			t.Fatalf("%s after %s: %v", typ, time.Since(start), err)
		}
	}
	dispatch("touchStart", []any{point(520)})
	for _, y := range []float64{485, 440, 385, 320, 245, 160, 75} {
		time.Sleep(16 * time.Millisecond)
		dispatch("touchMove", []any{point(y)})
	}
	time.Sleep(12 * time.Millisecond)
	dispatch("touchEnd", []any{})
	immediate, err := client.EvaluateString(session, `String(window.scrollY)`)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	later, err := client.EvaluateString(session, `String(window.scrollY)`)
	if err != nil {
		t.Fatal(err)
	}
	var immediateY, laterY float64
	if _, err := fmt.Sscan(immediate, &immediateY); err != nil {
		t.Fatalf("immediate scrollY=%q: %v", immediate, err)
	}
	if _, err := fmt.Sscan(later, &laterY); err != nil {
		t.Fatalf("later scrollY=%q: %v", later, err)
	}
	if laterY-immediateY < 100 {
		t.Fatalf("fling did not continue: scrollY immediate=%f later=%f", immediateY, laterY)
	}
}
