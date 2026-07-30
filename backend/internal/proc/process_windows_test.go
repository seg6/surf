//go:build windows

package proc

import "testing"

func TestCommandSuppressesConsoleWindow(t *testing.T) {
	cmd := Command("ffmpeg.exe", "-version")
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
