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
	"surf-backend/internal/protocol"
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

func installEditableObserver(t *testing.T, client *cdp.Client, session string) string {
	t.Helper()
	event, sensor, observer := newEditableScripts()
	if _, err := client.Call(session, "Runtime.addBinding", editableBindingParams()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Page.addScriptToEvaluateOnNewDocument", editableSensorParams(sensor, true)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Page.addScriptToEvaluateOnNewDocument", editableObserverParams(observer, true)); err != nil {
		t.Fatal(err)
	}
	return event
}

func installSelectObserver(t *testing.T, client *cdp.Client, session string) {
	t.Helper()
	_, sensor, observer := newSelectScripts()
	if _, err := client.Call(session, "Runtime.addBinding", selectBindingParams()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Page.addScriptToEvaluateOnNewDocument", selectSensorParams(sensor, true)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Page.addScriptToEvaluateOnNewDocument", selectObserverParams(observer, true)); err != nil {
		t.Fatal(err)
	}
}

func TestSelectObserverBridgesTrustedTapAndChange(t *testing.T) {
	client, session := launchInputTestBrowser(t)
	type report struct {
		payload   string
		contextID int64
	}
	bindings := make(chan report, 8)
	client.OnEvent(func(event cdp.Event) {
		if event.SessionID != session || event.Method != "Runtime.bindingCalled" {
			return
		}
		var binding struct {
			Name               string `json:"name"`
			Payload            string `json:"payload"`
			ExecutionContextID int64  `json:"executionContextId"`
		}
		if json.Unmarshal(event.Params, &binding) == nil && binding.Name == selectBinding {
			bindings <- report{payload: binding.Payload, contextID: binding.ExecutionContextID}
		}
	})
	installSelectObserver(t, client, session)
	if _, err := client.Call(session, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Runtime.evaluate", map[string]any{"expression": `(function(){
		document.body.innerHTML='<label for="choice">Background</label><select id="choice" aria-label="Background" style="position:absolute;left:40px;top:80px;width:200px;height:44px"><option>None</option><option selected>Stars</option><option disabled>Disabled</option><option>Nebula</option></select>';
		window.__selectInput=0;window.__selectChange=0;
		var select=document.getElementById('choice');
		select.addEventListener('input',function(){window.__selectInput++});
		select.addEventListener('change',function(){window.__selectChange++});
	})()`}); err != nil {
		t.Fatal(err)
	}
	exposure, err := client.EvaluateString(session, `typeof window.__surfSelectOpened+','+typeof window.__surfSelectObserver+','+typeof window.__surfSelectIntentSensor`)
	if err != nil {
		t.Fatal(err)
	}
	if exposure != "undefined,undefined,boolean" {
		t.Fatalf("select observer escaped isolated world: %s", exposure)
	}
	touch := func(events ...struct {
		typ string
		x   float64
		y   float64
	}) {
		t.Helper()
		for _, event := range events {
			points := []any{}
			if event.typ != "touchEnd" {
				points = append(points, map[string]any{"id": 0, "x": event.x, "y": event.y, "radiusX": 5, "radiusY": 5, "force": .5})
			}
			if _, err := client.Call(session, "Input.dispatchTouchEvent", map[string]any{"type": event.typ, "touchPoints": points}); err != nil {
				t.Fatal(err)
			}
		}
	}
	touch(
		struct {
			typ string
			x   float64
			y   float64
		}{"touchStart", 100, 102},
		struct {
			typ string
			x   float64
			y   float64
		}{"touchMove", 100, 145},
		struct {
			typ string
			x   float64
			y   float64
		}{"touchEnd", 100, 145},
	)
	select {
	case got := <-bindings:
		t.Fatalf("drag opened select: %s", got.payload)
	case <-time.After(250 * time.Millisecond):
	}
	touch(
		struct {
			typ string
			x   float64
			y   float64
		}{"touchStart", 100, 102},
		struct {
			typ string
			x   float64
			y   float64
		}{"touchEnd", 100, 102},
	)
	var got report
	select {
	case got = <-bindings:
	case <-time.After(5 * time.Second):
		t.Fatal("trusted select tap was not bridged")
	}
	var state struct {
		ID       string                  `json:"id"`
		Title    string                  `json:"title"`
		Multiple bool                    `json:"multiple"`
		Options  []protocol.SelectOption `json:"options"`
		Rect     []float64               `json:"rect"`
	}
	if json.Unmarshal([]byte(got.payload), &state) != nil || state.ID == "" || state.Title != "Background" ||
		state.Multiple || len(state.Options) != 4 || !state.Options[1].Selected || !state.Options[2].Disabled || len(state.Rect) != 4 {
		t.Fatalf("select payload=%s", got.payload)
	}
	idJSON, _ := json.Marshal(state.ID)
	if _, err := client.Call(session, "Runtime.evaluate", map[string]any{
		"contextId":  got.contextID,
		"expression": fmt.Sprintf(`globalThis.__surfApplySelect(%s,[3])`, idJSON),
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result, err := client.EvaluateString(session, `JSON.stringify({value:document.getElementById('choice').value,index:document.getElementById('choice').selectedIndex,input:window.__selectInput,change:window.__selectChange})`)
		if err == nil && strings.Contains(result, `"value":"Nebula"`) && strings.Contains(result, `"input":1`) && strings.Contains(result, `"change":1`) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("select reply did not update the page and dispatch input/change")
}

func TestEditableObserverFollowsOOPIFFocus(t *testing.T) {
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<input id="login" type="email" autofocus>`)
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
		case "Target.attachedToTarget", "Target.detachedFromTarget", "Runtime.bindingCalled":
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
	sawProgrammaticFocus := false
	for {
		select {
		case childSession = <-children:
			lastChildState = "attached " + childSession
		case <-focusTicker.C:
			if childSession != "" {
				_, _ = client.Call(session, "Runtime.evaluate", map[string]any{
					"expression": `document.querySelector('iframe').focus()`, "userGesture": true,
				})
				lastChildState, _ = client.EvaluateString(childSession, `(function(){
					var e=document.getElementById('login');if(e){e.blur();e.focus();}
					return JSON.stringify({url:location.href,ready:document.readyState,binding:typeof window.__surfEditableChanged,observer:!!window.__surfEditableObserver,sensor:!!window.__surfKeyboardIntentSensor,hasInput:!!e,active:document.activeElement&&document.activeElement.id});
				})()`)
			}
		case got := <-reports:
			var state struct {
				On   bool   `json:"on"`
				Show bool   `json:"show"`
				Kind string `json:"kind"`
			}
			if got.session != session && json.Unmarshal([]byte(got.payload), &state) == nil && state.On {
				if state.Kind != "email" || !browser.isActiveSession(got.session) {
					t.Fatalf("OOPIF editable session=%q payload=%s", got.session, got.payload)
				}
				if !sawProgrammaticFocus {
					if state.Show {
						t.Fatalf("programmatic OOPIF focus requested keyboard: %s", got.payload)
					}
					sawProgrammaticFocus = true
					focusTicker.Stop()
					point := map[string]any{"id": 0, "x": 70, "y": 18, "radiusX": 5, "radiusY": 5, "force": .5}
					if _, err := client.Call(got.session, "Input.dispatchTouchEvent", map[string]any{"type": "touchStart", "touchPoints": []any{point}}); err != nil {
						t.Fatal(err)
					}
					if _, err := client.Call(got.session, "Input.dispatchTouchEvent", map[string]any{"type": "touchEnd", "touchPoints": []any{}}); err != nil {
						t.Fatal(err)
					}
				} else if state.Show {
					return
				}
			}
		case <-deadline:
			t.Fatalf("editable observer did not report both OOPIF focus states: %s", lastChildState)
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
	installEditableObserver(t, client, session)
	if _, err := client.Call(session, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 5}); err != nil {
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
	sawProgrammaticFocus := false
	for {
		select {
		case payload := <-bindings:
			var state struct {
				On   bool      `json:"on"`
				Show bool      `json:"show"`
				Kind string    `json:"kind"`
				Rect []float64 `json:"rect"`
			}
			if json.Unmarshal([]byte(payload), &state) == nil && state.On {
				if state.Kind != "password" || len(state.Rect) != 4 {
					t.Fatalf("editable payload=%s", payload)
				}
				if !sawProgrammaticFocus {
					if state.Show {
						t.Fatalf("programmatic shadow focus requested keyboard: %s", payload)
					}
					sawProgrammaticFocus = true
					point := map[string]any{"id": 0, "x": 100, "y": 75, "radiusX": 5, "radiusY": 5, "force": .5}
					if _, err := client.Call(session, "Input.dispatchTouchEvent", map[string]any{"type": "touchStart", "touchPoints": []any{point}}); err != nil {
						t.Fatal(err)
					}
					if _, err := client.Call(session, "Input.dispatchTouchEvent", map[string]any{"type": "touchEnd", "touchPoints": []any{}}); err != nil {
						t.Fatal(err)
					}
				} else if state.Show {
					return
				}
			}
		case <-deadline:
			t.Fatal("editable observer did not report focused shadow input")
		}
	}
}

func TestEditableObserverScopesKeyboardIntentToTrustedActivation(t *testing.T) {
	client, session := launchInputTestBrowser(t)
	bindings := make(chan string, 32)
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
	intentEvent := installEditableObserver(t, client, session)
	if _, err := client.Call(session, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Runtime.evaluate", map[string]any{"expression": `(function(){
		document.body.innerHTML='<button id="promise" style="position:absolute;left:40px;top:20px;width:120px;height:35px">Promise</button><button id="timer" style="position:absolute;left:40px;top:65px;width:120px;height:35px">Timer</button><button id="frame" style="position:absolute;left:40px;top:110px;width:120px;height:35px">Frame</button><button id="sync" style="position:absolute;left:40px;top:155px;width:120px;height:35px">Sync</button><button id="window-sync" style="position:absolute;left:200px;top:155px;width:120px;height:35px">Window sync</button><div style="position:absolute;left:40px;top:210px;width:120px;height:40px"><input id="field" style="box-sizing:border-box;width:100%;height:100%"><span id="overlay" style="position:absolute;inset:0"></span></div><label for="label-field" style="position:absolute;left:40px;top:270px;width:120px;height:35px">Label</label><input id="label-field" style="position:absolute;left:40px;top:315px;width:120px;height:40px"><input id="direct-field" style="position:absolute;left:40px;top:375px;width:120px;height:40px">';
		var field=document.getElementById('field');
		document.getElementById('promise').addEventListener('click',function(){Promise.resolve().then(function(){field.focus()})});
		document.getElementById('timer').addEventListener('click',function(){setTimeout(function(){field.focus()},0)});
		document.getElementById('frame').addEventListener('click',function(){requestAnimationFrame(function(){field.focus()})});
		document.getElementById('sync').addEventListener('click',function(){field.focus()});
		window.addEventListener('click',function(event){if(event.target.id==='window-sync')field.focus()},true);
		document.getElementById('overlay').addEventListener('click',function(){field.focus()});
	})()`}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(session, "Runtime.evaluate", map[string]any{"expression": fmt.Sprintf(`window.__intentCount=0;document.addEventListener(%q,function(){window.__intentCount++})`, intentEvent)}); err != nil {
		t.Fatal(err)
	}
	exposure, err := client.EvaluateString(session, `typeof window.__surfEditableChanged+','+typeof window.__surfEditableObserver+','+typeof window.__surfKeyboardIntentSensor`)
	if err != nil {
		t.Fatal(err)
	}
	if exposure != "undefined,undefined,boolean" {
		t.Fatalf("editable observer escaped isolated world: %s", exposure)
	}

	type editableState struct {
		On   bool `json:"on"`
		Show bool `json:"show"`
	}
	waitFor := func(description string, match func(editableState) bool) editableState {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case payload := <-bindings:
				var state editableState
				if json.Unmarshal([]byte(payload), &state) == nil && match(state) {
					return state
				}
			case <-deadline:
				diagnostic, _ := client.EvaluateString(session, `JSON.stringify({active:document.activeElement&&document.activeElement.id,intents:window.__intentCount})`)
				t.Fatalf("timed out waiting for %s: %s", description, diagnostic)
			}
		}
	}
	setFocus := func(expression string) {
		t.Helper()
		if _, err := client.Call(session, "Runtime.evaluate", map[string]any{"expression": expression}); err != nil {
			t.Fatal(err)
		}
	}
	touch := func(x, y float64) {
		t.Helper()
		point := map[string]any{"id": 0, "x": x, "y": y, "radiusX": 5, "radiusY": 5, "force": .5}
		if _, err := client.Call(session, "Input.dispatchTouchEvent", map[string]any{"type": "touchStart", "touchPoints": []any{point}}); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Call(session, "Input.dispatchTouchEvent", map[string]any{"type": "touchEnd", "touchPoints": []any{}}); err != nil {
			t.Fatal(err)
		}
	}

	setFocus(`document.getElementById('field').focus()`)
	waitFor("programmatic focus without keyboard intent", func(state editableState) bool { return state.On && !state.Show })
	setFocus(`document.getElementById('field').blur()`)
	waitFor("blur", func(state editableState) bool { return !state.On })

	// User activation does not leak into promise, timer, or animation-frame
	// work scheduled by the page.
	touch(100, 37)
	waitFor("promise focus without keyboard intent", func(state editableState) bool { return state.On && !state.Show })
	setFocus(`document.getElementById('field').blur()`)
	waitFor("second blur", func(state editableState) bool { return !state.On })
	touch(100, 82)
	waitFor("timer focus without keyboard intent", func(state editableState) bool { return state.On && !state.Show })
	setFocus(`document.getElementById('field').blur()`)
	waitFor("third blur", func(state editableState) bool { return !state.On })
	touch(100, 127)
	waitFor("animation-frame focus without keyboard intent", func(state editableState) bool { return state.On && !state.Show })
	setFocus(`document.getElementById('field').blur()`)
	waitFor("fourth blur", func(state editableState) bool { return !state.On })

	// A website may intentionally focus a field synchronously from another
	// control. That is part of the user's activation and should open the keyboard.
	touch(100, 172)
	waitFor("synchronous website focus with keyboard intent", func(state editableState) bool { return state.On && state.Show })
	setFocus(`document.getElementById('field').blur()`)
	waitFor("fifth blur", func(state editableState) bool { return !state.On })
	touch(260, 172)
	waitFor("window-capture focus with keyboard intent", func(state editableState) bool { return state.On && state.Show })
	setFocus(`document.getElementById('field').blur()`)
	waitFor("sixth blur", func(state editableState) bool { return !state.On })

	// Visible controls may route activation through overlays or wrappers.
	touch(100, 230)
	waitFor("synchronous overlay focus with keyboard intent", func(state editableState) bool { return state.On && state.Show })
	setFocus(`document.getElementById('field').blur()`)
	waitFor("seventh blur", func(state editableState) bool { return !state.On })

	// Standard labels are part of the field's activation area even when their
	// own rectangle does not overlap the input.
	touch(100, 287)
	waitFor("trusted label touch", func(state editableState) bool { return state.On && state.Show })
	setFocus(`document.getElementById('label-field').blur()`)
	waitFor("eighth blur", func(state editableState) bool { return !state.On })

	// Direct fields work, and tapping one again can reopen a keyboard the user
	// dismissed without requiring a focus transition.
	touch(100, 395)
	waitFor("trusted direct field touch", func(state editableState) bool { return state.On && state.Show })
	touch(100, 395)
	waitFor("trusted touch on already-focused field", func(state editableState) bool { return state.On && state.Show })
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
	installSelectObserver(t, client, session)
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
		return map[string]any{"id": 0, "x": 200, "y": y, "radiusX": 5, "radiusY": 5, "force": .5}
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
	time.Sleep(touchReleaseSettle)
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
