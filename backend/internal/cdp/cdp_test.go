package cdp

import "testing"

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
	if !hasArg(args, "--mute-audio") {
		t.Fatalf("missing host-audio isolation flag: %v", args)
	}
	if !hasArg(args, "--enable-gpu") || hasArg(args, "--disable-gpu") {
		t.Fatalf("GPU auto path not enabled: %v", args)
	}
	if hasArg(args, "--disable-smooth-scrolling") {
		t.Fatalf("obsolete scroll override present: %v", args)
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
	if !hasArg(args, "--disable-extensions-except=/tmp/ubol") ||
		!hasArg(args, "--load-extension=/tmp/ubol") {
		t.Fatalf("missing content blocker args: %v", args)
	}
}

func TestLaunchArgsMultipleExtensions(t *testing.T) {
	args := LaunchConfig{
		Profile: "/tmp/profile", W: 1024, H: 768,
		ExtensionPaths: []string{"/tmp/ubol", "/tmp/audio"},
	}.Args()
	if !hasArg(args, "--disable-extensions-except=/tmp/ubol,/tmp/audio") ||
		!hasArg(args, "--load-extension=/tmp/ubol,/tmp/audio") {
		t.Fatalf("missing combined extension args: %v", args)
	}
}
