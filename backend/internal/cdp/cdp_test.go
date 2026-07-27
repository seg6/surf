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
	args := LaunchConfig{Profile: "/tmp/profile", W: 1024, H: 768}.Args()
	if hasArg(args, "--no-sandbox") {
		t.Fatal("--no-sandbox present when NoSandbox=false")
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

func TestLaunchArgsPlatformArgsAppended(t *testing.T) {
	args := LaunchConfig{Profile: "/tmp/profile", W: 1024, H: 768, PlatformArgs: []string{"--ozone-platform=x11"}}.Args()
	if !hasArg(args, "--ozone-platform=x11") {
		t.Fatalf("missing platform arg: %v", args)
	}
	if args[len(args)-1] != "about:blank" {
		t.Fatalf("last arg=%q, want about:blank", args[len(args)-1])
	}
}
