package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/config"
	"surf-backend/internal/media"
	"surf-backend/internal/transport"
)

// Start installs the event handler before attaching the main page. Input-only
// harnesses attach first, so they cannot catch accidentally detaching that page.
func TestBrowserStartupKeepsPageSessionForCapture(t *testing.T) {
	chrome := os.Getenv("SURF_TEST_BROWSER_INPUT")
	if chrome == "" {
		t.Skip("set SURF_TEST_BROWSER_INPUT to a Chrome/Chromium executable")
	}
	requested := make(chan struct{}, 1)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/worker.js" {
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = fmt.Fprint(w, `postMessage('worker ready');`)
			return
		}
		select {
		case requested <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<output id="status">waiting</output><canvas></canvas><script>
const worker = new Worker('/worker.js');
worker.onmessage = e => document.getElementById('status').textContent = e.data;
const ctx = document.querySelector('canvas').getContext('2d');
function draw(t) { ctx.fillStyle = 'hsl(' + (t / 10 % 360) + ' 100% 50%)'; ctx.fillRect(0, 0, 300, 150); requestAnimationFrame(draw); }
requestAnimationFrame(draw);
</script>`)
	}))
	defer site.Close()
	home := t.TempDir()
	cfg := &config.Config{
		SurfHome: home, Profile: filepath.Join(home, "profile"),
		DownloadsDir: filepath.Join(home, "downloads"), UploadsDir: filepath.Join(home, "uploads"),
		ChromePath: chrome, ChromeNoSandbox: os.Geteuid() == 0,
		StartURL: site.URL, ViewW: 400, ViewH: 600, StreamQuantizer: 18,
	}
	for _, dir := range []string{cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	browser, err := New(cfg, transport.New())
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Shutdown()
	if err := browser.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("startup page never navigated after attaching its debugger session")
	}
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		browser.mu.Lock()
		session := ""
		if tab := browser.tabs[browser.activeID]; tab != nil {
			session = tab.Session
		}
		browser.mu.Unlock()
		if session != "" {
			status, err := browser.cdp.EvaluateString(session, `document.getElementById('status')?.textContent`)
			if err != nil {
				t.Fatal(err)
			}
			if status == "worker ready" {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("startup page worker did not run")
	}
	frames := make(chan media.VideoFrame, 1)
	if err := browser.capture.StartVideo(media.EncoderConfig{
		Codec: encoderCodec(400, 600, 60), Width: 400, Height: 600,
		FrameRate: 60, BitrateK: 4000, Quantizer: 18,
	}, func(frame media.VideoFrame) {
		if frame.Key && len(frame.Data) != 0 {
			select {
			case frames <- frame:
			default:
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-frames:
		if frame.Width != 400 || frame.Height != 600 {
			t.Fatalf("capture dimensions=%dx%d", frame.Width, frame.Height)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup capture produced no encoded keyframe")
	}
}

// Facebook's worker-backed video player cannot attach a media source if our
// iframe debugger setup leaves its dedicated worker paused before startup.
func TestEditableAutoAttachDoesNotStallWorkers(t *testing.T) {
	for _, embedded := range []bool{false, true} {
		for _, kind := range []string{"classic", "module"} {
			t.Run(fmt.Sprintf("iframe=%t/%s", embedded, kind), func(t *testing.T) {
				var siteURL string
				site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/worker.js" {
						w.Header().Set("Content-Type", "text/javascript")
						_, _ = fmt.Fprint(w, `postMessage('worker ready');`)
						return
					}
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					if embedded && r.URL.Path == "/" {
						_, _ = fmt.Fprintf(w, `<body>waiting<script>
addEventListener('message', e => { if (e.data === 'worker ready') document.body.textContent = e.data; });
</script><iframe src="%s/frame"></iframe>`, strings.Replace(siteURL, "127.0.0.1", "localhost", 1))
						return
					}
					_, _ = fmt.Fprintf(w, `<body>waiting<script>
const worker = new Worker('/worker.js', {type: %q});
worker.onmessage = e => { document.body.textContent = e.data; if (parent !== window) parent.postMessage(e.data, '*'); };
worker.onerror = () => { document.body.textContent = 'worker error'; };
</script>`, kind)
				}))
				defer site.Close()
				siteURL = site.URL

				client, session := launchInputTestBrowser(t)
				tab := &Tab{ID: 1, Session: session}
				browser := &Controller{
					cdp: client, hub: transport.New(), tabs: map[int]*Tab{1: tab},
					bySession: map[string]*Tab{session: tab}, activeID: 1,
				}
				client.OnEvent(func(event cdp.Event) {
					switch event.Method {
					case "Target.attachedToTarget", "Target.detachedFromTarget", "Runtime.bindingCalled":
						browser.onEvent(event)
					}
				})
				browser.setupEditableObserver(session)
				browser.setupEditableAutoAttach(session)
				if _, err := client.Call(session, "Page.navigate", map[string]any{"url": site.URL}); err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(5 * time.Second)
				last := ""
				for time.Now().Before(deadline) {
					last, _ = client.EvaluateString(session, `document.body && document.body.textContent`)
					if last == "worker ready" {
						return
					}
					time.Sleep(20 * time.Millisecond)
				}
				t.Fatalf("worker did not run after iframe auto-attach: %q", last)
			})
		}
	}
}
