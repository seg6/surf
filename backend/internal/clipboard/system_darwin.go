//go:build darwin

package clipboard

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type pasteboardSystem struct{}

func (pasteboardSystem) Name() string { return "macOS" }

func (pasteboardSystem) Read(ctx context.Context) (string, error) {
	data, err := exec.CommandContext(ctx, "/usr/bin/pbpaste").Output()
	if err != nil {
		return "", fmt.Errorf("read macOS clipboard: %w", err)
	}
	return string(data), nil
}

func (pasteboardSystem) Write(ctx context.Context, text string) error {
	command := exec.CommandContext(ctx, "/usr/bin/pbcopy")
	command.Stdin = bytes.NewBufferString(text)
	if err := command.Run(); err != nil {
		return fmt.Errorf("write macOS clipboard: %w", err)
	}
	return nil
}

func newSystemClipboard() systemClipboard { return pasteboardSystem{} }
