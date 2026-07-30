//go:build !windows

package chromium

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

func browserMajor(path string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return 0, err
	}
	match := regexp.MustCompile(`\b(\d+)\.`).FindSubmatch(output)
	if len(match) != 2 {
		return 0, fmt.Errorf("browser version missing from %q", output)
	}
	return strconv.Atoi(string(match[1]))
}
