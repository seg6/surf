//go:build linux

package clipboard

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

type commandSystem struct {
	name      string
	readArgs  []string
	writeArgs []string
}

func (s commandSystem) Name() string { return s.name }

func (s commandSystem) Read(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, s.readArgs[0], s.readArgs[1:]...)
	data, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read %s clipboard: %w", s.name, err)
	}
	return string(data), nil
}

func (s commandSystem) Write(ctx context.Context, text string) error {
	command := exec.CommandContext(ctx, s.writeArgs[0], s.writeArgs[1:]...)
	command.Stdin = bytes.NewBufferString(text)
	if err := command.Run(); err != nil {
		return fmt.Errorf("write %s clipboard: %w", s.name, err)
	}
	return nil
}

func newSystemClipboard() systemClipboard {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			if _, err := exec.LookPath("wl-copy"); err == nil {
				return commandSystem{name: "Wayland", readArgs: []string{"wl-paste", "--no-newline"}, writeArgs: []string{"wl-copy"}}
			}
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return commandSystem{name: "X11", readArgs: []string{"xclip", "-selection", "clipboard", "-out"}, writeArgs: []string{"xclip", "-selection", "clipboard", "-in"}}
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		return commandSystem{name: "X11", readArgs: []string{"xsel", "--clipboard", "--output"}, writeArgs: []string{"xsel", "--clipboard", "--input"}}
	}
	return unavailableSystem{}
}
