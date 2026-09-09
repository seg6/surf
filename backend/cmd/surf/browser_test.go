package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"surf-backend/internal/control"
	"surf-backend/internal/testhost"
)

func TestBrowserCommandArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--status"}, {"--yes"}, {"--resume"}, {"--resume", "--force", "--yes"}} {
		if _, err := parseBrowserCommand(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"--force"}, {"--status", "--yes"}, {"--status", "--resume"}, {"--yes", "--yes"}, {"https://example.com"}} {
		if _, err := parseBrowserCommand(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestBrowserStandaloneHelperProcess(t *testing.T) {
	if os.Getenv("SURF_BROWSER_CLI_TEST_CHILD") != "1" {
		return
	}
	if err := runBrowserCommand(nil); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserStandaloneCLIIntegration(t *testing.T) {
	chrome := testhost.VisibleBrowser(t)
	home := t.TempDir()
	t.Setenv("SURF_HOME", home)
	t.Setenv("CHROME", chrome)
	t.Setenv("SURF_CONTENT_BLOCKER", "0")
	// Occupy the configured public port. Standalone must not bind it at all.
	port, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer port.Close()
	t.Setenv("PORT", strconv.Itoa(port.Addr().(*net.TCPAddr).Port))
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^TestBrowserStandaloneHelperProcess$")
	cmd.Env = append(os.Environ(), "SURF_BROWSER_CLI_TEST_CHILD=1")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var exitErr error
	go func() { exitErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
		if t.Failed() {
			t.Log(output.String())
		}
	})
	deadline := time.Now().Add(20 * time.Second)
	for {
		descriptor, err := control.Load(home)
		if err == nil {
			if !descriptor.Standalone || descriptor.PID != cmd.Process.Pid {
				t.Fatalf("wrong setup owner: %+v", descriptor)
			}
			break
		}
		select {
		case <-done:
			t.Fatalf("setup exited early: %v", exitErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("setup did not publish local control")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := runBrowserCommand([]string{"--status"}); err != nil {
		t.Fatal(err)
	}
	if err := runBrowserCommand(nil); err != nil {
		t.Fatalf("focus existing setup: %v", err)
	}
	if err := runBrowserCommand([]string{"--resume", "--yes"}); err == nil {
		t.Fatal("standalone resumed a server")
	}
	if err := runPairCommandContext(context.Background()); err == nil {
		t.Fatal("standalone offered pairing")
	}
	if err := runQuitCommand(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		if exitErr != nil {
			t.Fatal(exitErr)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("surf quit did not close standalone")
	}
	if _, err := control.Load(home); err == nil {
		t.Fatal("setup owner descriptor survived exit")
	}
}
