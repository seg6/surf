package testhost

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// These tests belong to go test ./..., and never open windows on the user's
// desktop. CI must provide Chromium and Xvfb, as ordinary test dependencies.
func VisibleBrowser(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("visible-browser integration uses Linux Xvfb")
	}
	chrome := os.Getenv("SURF_TEST_BROWSER")
	if chrome == "" {
		chrome = os.Getenv("SURF_TEST_CHROME")
	}
	if chrome == "" {
		for _, candidate := range []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser"} {
			if path, err := exec.LookPath(candidate); err == nil {
				chrome = path
				break
			}
		}
	}
	xvfb, err := exec.LookPath("Xvfb")
	if chrome == "" || err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatal("browser setup tests require Chromium and Xvfb")
		}
		t.Skip("install Chromium and Xvfb for browser setup integration")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(xvfb, "-displayfd", "3", "-screen", "0", "1280x1024x24", "-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{writer}
	if err := cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = reader.Close() })
	display := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(reader).ReadString('\n'); display <- strings.TrimSpace(line) }()
	select {
	case number := <-display:
		if number == "" {
			t.Fatal("Xvfb did not publish a display")
		}
		t.Setenv("DISPLAY", ":"+number)
		t.Setenv("WAYLAND_DISPLAY", "")
	case <-time.After(5 * time.Second):
		t.Fatal("Xvfb startup timed out")
	}
	return chrome
}
