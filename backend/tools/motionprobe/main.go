// motionprobe injects a deterministic compositor animation into the active
// managed-Chrome page. It exercises capture, encode, transport, decode, and
// presentation without pretending to measure the separate touch-input path.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type target struct {
	Type   string `json:"type"`
	Socket string `json:"webSocketDebuggerUrl"`
}

func main() {
	duration := flag.Duration("duration", 30*time.Second, "probe duration")
	profile := flag.String("profile", defaultProfile(), "managed Chrome profile")
	inspectMedia := flag.Bool("inspect-media", false, "print active page media state and exit")
	expression := flag.String("eval", "", "evaluate JavaScript in the active page and print its value")
	scroll := flag.Bool("scroll", false, "run a deterministic compositor scroll instead of the block animation")
	touchFling := flag.Bool("touch-fling", false, "dispatch one touch flick, report post-lift travel, and restore scroll position")
	touchID := flag.Int("touch-id", 1, "contact ID used by -touch-fling")
	flag.Parse()

	socket, err := pageSocket(*profile)
	if err != nil {
		fatal(err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(socket, nil)
	if err != nil {
		fatal(err)
	}
	defer conn.Close()
	if *expression != "" {
		value, err := evaluateValue(conn, 1, *expression)
		if err != nil {
			fatal(err)
		}
		fmt.Println(value)
		return
	}
	if *inspectMedia {
		const expression = `JSON.stringify([...document.querySelectorAll("video,audio")].map(v => ({
		  tag:v.tagName, paused:v.paused, muted:v.muted, volume:v.volume,
		  currentTime:v.currentTime, readyState:v.readyState, networkState:v.networkState
		})))`
		value, err := evaluateValue(conn, 1, expression)
		if err != nil {
			fatal(err)
		}
		fmt.Println(value)
		return
	}
	if *touchFling {
		if err := runTouchFlingProbe(conn, *touchID); err != nil {
			fatal(err)
		}
		return
	}
	if *scroll {
		if err := runScrollProbe(conn, *duration); err != nil {
			fatal(err)
		}
		return
	}

	const install = `(() => {
	  document.getElementById('__surf_probe')?.remove();
	  const p = document.createElement('div');
	  p.id = '__surf_probe';
	  p.style.cssText = 'position:fixed;z-index:2147483647;left:0;top:20%;width:160px;height:160px;background:#1974d2;pointer-events:none';
	  document.documentElement.appendChild(p);
	  const start = performance.now();
	  function tick(now) {
	    const x = ((now - start) * .45) % Math.max(1, innerWidth - 160);
	    p.style.transform = 'translate3d(' + x + 'px,0,0) rotate(' + x + 'deg)';
	    p.style.background = 'hsl(' + (x % 360) + ' 80% 50%)';
	    p.__surfRAF = requestAnimationFrame(tick);
	  }
	  p.__surfRAF = requestAnimationFrame(tick);
	  return true;
	})()`
	if err := evaluate(conn, 1, install); err != nil {
		fatal(err)
	}
	fmt.Printf("motion probe active for %s\n", *duration)
	time.Sleep(*duration)
	const remove = `(() => {
	  const p = document.getElementById('__surf_probe');
	  if (p) { cancelAnimationFrame(p.__surfRAF); p.remove(); }
	})()`
	if err := evaluate(conn, 2, remove); err != nil {
		fatal(err)
	}
}

func runTouchFlingProbe(conn *websocket.Conn, touchID int) error {
	before, err := evaluateValue(conn, 1, `String(scrollY)`)
	if err != nil {
		return err
	}
	point := func(y float64) []map[string]any {
		return []map[string]any{{"id": touchID, "x": 384, "y": y, "radiusX": 8, "radiusY": 8, "force": .5}}
	}
	id := 2
	dispatch := func(kind string, points []map[string]any) error {
		params := map[string]any{
			"type": kind, "touchPoints": points,
			"timestamp": float64(time.Now().UnixNano()) / float64(time.Second),
		}
		if err := conn.WriteJSON(map[string]any{
			"id": id, "method": "Input.dispatchTouchEvent", "params": params,
		}); err != nil {
			return err
		}
		id++
		return nil
	}
	if err := dispatch("touchStart", point(763)); err != nil {
		return err
	}
	for _, y := range []float64{732, 695, 558, 460, 353, 235} {
		time.Sleep(16 * time.Millisecond)
		if err := dispatch("touchMove", point(y)); err != nil {
			return err
		}
	}
	time.Sleep(12 * time.Millisecond)
	if err := dispatch("touchEnd", []map[string]any{}); err != nil {
		return err
	}
	immediate, err := evaluateValue(conn, id, `String(scrollY)`)
	if err != nil {
		return err
	}
	id++
	time.Sleep(400 * time.Millisecond)
	later, err := evaluateValue(conn, id, `String(scrollY)`)
	if err != nil {
		return err
	}
	id++
	if err := evaluate(conn, id, `scrollTo(0, `+before+`)`); err != nil {
		return err
	}
	fmt.Printf("touch fling scrollY before=%s immediate=%s after400ms=%s\n", before, immediate, later)
	return nil
}

func runScrollProbe(conn *websocket.Conn, duration time.Duration) error {
	const page = `(() => {
	  document.open();
	  document.write('<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1">' +
	    '<style>html,body{margin:0}body{height:12000px;background:repeating-linear-gradient(' +
	    '180deg,#0b1220 0,#0b1220 120px,#1d4ed8 120px,#1d4ed8 240px)}' +
	    '.mark{position:fixed;left:24px;top:24px;padding:12px 16px;color:white;background:#111827;' +
	    'font:700 18px sans-serif;border-radius:8px}</style><div class="mark">Surf scroll probe</div>');
	  document.close();
	  scrollTo(0, 0);
	  return true;
	})()`
	if err := evaluate(conn, 1, page); err != nil {
		return err
	}
	fmt.Printf("scroll probe active for %s\n", duration)
	deadline := time.Now().Add(duration)
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()
	delta := 20.0
	frames := 0
	id := 2
	for time.Now().Before(deadline) {
		<-ticker.C
		err := call(conn, id, "Input.dispatchMouseEvent", map[string]any{
			"type":   "mouseWheel",
			"x":      384,
			"y":      700,
			"deltaX": 0,
			"deltaY": delta,
		})
		if err != nil {
			return err
		}
		id++
		frames++
		// Reverse every two seconds, well before either document boundary.
		// That keeps the viewport changing for the full probe instead of
		// measuring a correctly static frame after reaching the top/bottom.
		if frames%120 == 0 {
			delta = -delta
		}
	}
	return nil
}

func call(conn *websocket.Conn, id int, method string, params any) error {
	if err := conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var message struct {
			ID    int             `json:"id"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &message) == nil && message.ID == id {
			if len(message.Error) != 0 {
				return fmt.Errorf("%s: %s", method, message.Error)
			}
			return nil
		}
	}
}

func defaultProfile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".surf/profile"
	}
	return filepath.Join(home, ".surf", "profile")
}

func pageSocket(profile string) (string, error) {
	data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		return "", fmt.Errorf("read DevToolsActivePort: %w", err)
	}
	port := strings.Split(strings.TrimSpace(string(data)), "\n")[0]
	response, err := http.Get("http://127.0.0.1:" + port + "/json/list")
	if err != nil {
		return "", fmt.Errorf("list targets: %w", err)
	}
	defer response.Body.Close()
	var targets []target
	if err := json.NewDecoder(response.Body).Decode(&targets); err != nil {
		return "", fmt.Errorf("decode targets: %w", err)
	}
	for _, candidate := range targets {
		if candidate.Type == "page" && candidate.Socket != "" {
			return candidate.Socket, nil
		}
	}
	return "", fmt.Errorf("no page target in profile %s", profile)
}

func evaluate(conn *websocket.Conn, id int, expression string) error {
	if err := conn.WriteJSON(map[string]any{
		"id": id, "method": "Runtime.evaluate",
		"params": map[string]any{"expression": expression, "awaitPromise": true},
	}); err != nil {
		return err
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var message struct {
			ID    int             `json:"id"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &message) == nil && message.ID == id {
			if len(message.Error) != 0 {
				return fmt.Errorf("CDP evaluate: %s", message.Error)
			}
			return nil
		}
	}
}

func evaluateValue(conn *websocket.Conn, id int, expression string) (string, error) {
	if err := conn.WriteJSON(map[string]any{
		"id": id, "method": "Runtime.evaluate",
		"params": map[string]any{"expression": expression, "returnByValue": true},
	}); err != nil {
		return "", err
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return "", err
		}
		var message struct {
			ID     int `json:"id"`
			Result struct {
				Result struct {
					Value string `json:"value"`
				} `json:"result"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &message) == nil && message.ID == id {
			if len(message.Error) != 0 {
				return "", fmt.Errorf("CDP evaluate: %s", message.Error)
			}
			return message.Result.Result.Value, nil
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "motionprobe:", err)
	os.Exit(1)
}
