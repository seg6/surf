//go:build windows

package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandSuppressesConsoleWindow(t *testing.T) {
	cmd := hiddenCommand("chromium.exe", "--version")
	if cmd.SysProcAttr == nil {
		t.Fatal("Command left SysProcAttr unset")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("Command does not hide the child window")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("CreationFlags=%#x, missing CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}

func TestMatchesExecutableChecksExactProcessImage(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !MatchesExecutable(os.Getpid(), executable) {
		t.Fatal("current executable did not match itself")
	}
	if MatchesExecutable(os.Getpid(), filepath.Join(filepath.Dir(executable), "different.exe")) {
		t.Fatal("different executable matched current process")
	}
}

func TestProtectChildrenCleanupDoesNotTerminateOwner(t *testing.T) {
	cleanup := ProtectChildren()
	cleanup()
	// Reaching this line is the regression assertion: closing a kill-on-close
	// job containing the current process terminates the test before it returns.
}
