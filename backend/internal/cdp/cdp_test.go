package cdp

import (
	"encoding/json"
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
	args := LaunchConfig{Profile: "/tmp/profile", W: 1024, H: 768, EnableGPU: true}.Args()
	if hasArg(args, "--no-sandbox") {
		t.Fatal("--no-sandbox present when NoSandbox=false")
	}
	if !hasArg(args, "--headless=new") {
		t.Fatalf("missing headless flag: %v", args)
	}
	if hasArg(args, "--mute-audio") {
		t.Fatalf("global mute makes tab-capture PCM silent: %v", args)
	}
	if !hasArg(args, "--enable-gpu") || hasArg(args, "--disable-gpu") {
		t.Fatalf("GPU auto path not enabled: %v", args)
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
