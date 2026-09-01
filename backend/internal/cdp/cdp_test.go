package cdp

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestLaunchArgsSandboxDefault(t *testing.T) {
	args := LaunchConfig{Profile: "/tmp/profile", W: 1024, H: 768}.Args()
	if hasArg(args, "--no-sandbox") {
		t.Fatal("--no-sandbox present when NoSandbox=false")
	}
	if !hasArg(args, "--headless=new") {
		t.Fatalf("missing headless flag: %v", args)
	}
	if hasArg(args, "--mute-audio") {
		t.Fatalf("global mute makes tab-capture PCM silent: %v", args)
	}
	if !hasArg(args, "--disable-background-mode") {
		t.Fatalf("browser background mode was not disabled: %v", args)
	}
	if !hasArg(args, "--enable-gpu") || hasArg(args, "--disable-gpu") {
		t.Fatalf("GPU auto path not enabled: %v", args)
	}
	if hasArg(args, "--disable-dev-shm-usage") {
		t.Fatalf("shared memory redirected through /tmp: %v", args)
	}
	for _, obsolete := range []string{
		"--ignore-gpu-blocklist", "--use-gl=angle", "--use-angle=gl-egl",
	} {
		if hasArg(args, obsolete) {
			t.Fatalf("obsolete graphics bridge arg %q present: %v", obsolete, args)
		}
	}
	if hasArg(args, "--disable-smooth-scrolling") {
		t.Fatalf("obsolete scroll override present: %v", args)
	}
	if hasArg(args, "--disable-component-update") ||
		hasArg(args, "--disable-background-networking") {
		t.Fatalf("browser CDMs and components must remain available: %v", args)
	}
	if !hasArg(args, "--user-data-dir=/tmp/profile") || !hasArg(args, "--window-size=1024,768") {
		t.Fatalf("missing profile/window args: %v", args)
	}
	if args[len(args)-1] != "about:blank" {
		t.Fatalf("last arg=%q, want about:blank", args[len(args)-1])
	}
}

func TestLaunchArgsNoSandbox(t *testing.T) {
	args := LaunchConfig{Profile: "/tmp/profile", W: 1024, H: 768, NoSandbox: true}.Args()
	if !hasArg(args, "--no-sandbox") {
		t.Fatal("--no-sandbox missing when NoSandbox=true")
	}
}

func TestLaunchArgsExtraArgsAppendedBeforeURL(t *testing.T) {
	args := LaunchConfig{Profile: "/tmp/profile", W: 1024, H: 768, ExtraArgs: []string{"--foo=bar"}}.Args()
	if !hasArg(args, "--foo=bar") {
		t.Fatalf("missing extra arg: %v", args)
	}
	if args[len(args)-1] != "about:blank" {
		t.Fatalf("last arg=%q, want about:blank", args[len(args)-1])
	}
}

func TestLaunchArgsContentBlocker(t *testing.T) {
	args := LaunchConfig{
		Profile: "/tmp/profile", W: 1024, H: 768,
		ExtensionPaths: []string{"/tmp/ubol"},
	}.Args()
	if hasArg(args, "--disable-extensions-except=/tmp/ubol") ||
		!hasArg(args, "--load-extension=/tmp/ubol") ||
		!hasArg(args, "--enable-unsafe-extension-debugging") {
		t.Fatalf("missing content blocker args: %v", args)
	}
}

func TestLaunchArgsMultipleExtensions(t *testing.T) {
	args := LaunchConfig{
		Profile: "/tmp/profile", W: 1024, H: 768,
		ExtensionPaths: []string{"/tmp/ubol", "/tmp/audio"},
	}.Args()
	if hasArg(args, "--disable-extensions-except=/tmp/ubol,/tmp/audio") ||
		!hasArg(args, "--load-extension=/tmp/ubol,/tmp/audio") ||
		!hasArg(args, "--enable-unsafe-extension-debugging") {
		t.Fatalf("missing combined extension args: %v", args)
	}
}

type extensionCall struct {
	method string
	path   string
}

type fakeExtensionCaller struct {
	installed string
	calls     []extensionCall
}

func (f *fakeExtensionCaller) Call(_ string, method string, params any) (json.RawMessage, error) {
	call := extensionCall{method: method}
	if values, ok := params.(map[string]any); ok {
		call.path, _ = values["path"].(string)
	}
	f.calls = append(f.calls, call)
	if method == "Extensions.getExtensions" {
		return json.Marshal(map[string]any{
			"extensions": []map[string]string{{"path": f.installed}},
		})
	}
	return json.RawMessage(`{"id":"loaded"}`), nil
}

func TestEnsureUnpackedExtensionsLoadsOnlyMissingPaths(t *testing.T) {
	client := &fakeExtensionCaller{installed: "/tmp/already"}
	if err := ensureUnpackedExtensions(client, []string{"/tmp/already", "/tmp/missing"}); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != 2 {
		t.Fatalf("calls=%v, want list plus one load", client.calls)
	}
	if got := client.calls[1]; got.method != "Extensions.loadUnpacked" || got.path != "/tmp/missing" {
		t.Fatalf("load call=%v", got)
	}
}

func TestActivePortStateRejectsStaleEndpoint(t *testing.T) {
	stamp := time.Unix(123, 0)
	previous := activePortState{data: "9222\n/devtools/browser/old", modTime: stamp, exists: true}
	if previous.changedFrom(previous) {
		t.Fatal("unchanged DevToolsActivePort was accepted")
	}
	rewritten := previous
	rewritten.modTime = stamp.Add(time.Second)
	if !rewritten.changedFrom(previous) {
		t.Fatal("rewritten DevToolsActivePort was rejected")
	}
	fresh := activePortState{data: "9223\n/devtools/browser/new", modTime: stamp, exists: true}
	if !fresh.changedFrom(previous) {
		t.Fatal("new DevToolsActivePort was rejected")
	}
}

func TestWaitForURLAcceptsEndpointAfterLauncherExit(t *testing.T) {
	urls := make(chan string, 1)
	done := make(chan error, 1)
	done <- nil
	close(done)
	go func() {
		time.Sleep(10 * time.Millisecond)
		urls <- "ws://127.0.0.1:9222/devtools/browser/delegated"
	}()
	got, err := waitForURLWithin(urls, t.TempDir(), activePortState{}, done, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ws://127.0.0.1:9222/devtools/browser/delegated" {
		t.Fatalf("URL=%q", got)
	}
}

func TestWaitForURLAcceptsActivePortAfterLauncherExit(t *testing.T) {
	want := "ws://127.0.0.1/devtools/browser/delegated"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": want})
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	profile := t.TempDir()
	done := make(chan error, 1)
	done <- nil
	close(done)
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(profile, "DevToolsActivePort"), []byte(port+"\n/devtools/browser/delegated\n"), 0o600)
	}()
	got, err := waitForURLWithin(make(chan string), profile, activePortState{}, done, 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("URL=%q, want %q", got, want)
	}
}

func TestWaitForURLReportsLauncherResultOnlyAtDeadline(t *testing.T) {
	urls := make(chan string)
	done := make(chan error, 1)
	done <- errors.New("exit status 1")
	close(done)
	started := time.Now()
	_, err := waitForURLWithin(urls, t.TempDir(), activePortState{}, done, 25*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "launcher exited: exit status 1") {
		t.Fatalf("error=%v", err)
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond {
		t.Fatalf("launcher exit ended endpoint wait after %s", elapsed)
	}
}

func TestBoundedLineTail(t *testing.T) {
	var tail boundedLineTail
	for i := 0; i < stderrTailLines+4; i++ {
		tail.add(strings.Repeat("x", stderrTailLineBytes+20) + string(rune('a'+i)))
	}
	lines := strings.Split(tail.string(), " | ")
	if len(lines) != stderrTailLines {
		t.Fatalf("tail lines=%d, want %d", len(lines), stderrTailLines)
	}
	for _, line := range lines {
		if len(line) > stderrTailLineBytes {
			t.Fatalf("tail line has %d bytes", len(line))
		}
	}
}
