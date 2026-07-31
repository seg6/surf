//go:build darwin

package chromium

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
)

func systemCandidates() []candidate {
	home, _ := os.UserHomeDir()
	return []candidate{
		{path: "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", label: "system Microsoft Edge"},
		{path: filepath.Join(home, "Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"), label: "system Microsoft Edge"},
		{path: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", label: "system Google Chrome Stable", branded: true},
		{path: filepath.Join(home, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"), label: "system Google Chrome Stable", branded: true},
		{path: "/Applications/Chromium.app/Contents/MacOS/Chromium", label: "system Chromium"},
		{path: filepath.Join(home, "Applications/Chromium.app/Contents/MacOS/Chromium"), label: "system Chromium"},
	}
}

func platformRelease() (string, *regexp.Regexp, error) {
	architecture := "x86_64"
	if runtime.GOARCH == "arm64" {
		architecture = "arm64"
	}
	return "ungoogled-software/ungoogled-chromium-macos",
		regexp.MustCompile(`_` + architecture + `-macos\.dmg$`), nil
}

func installArchive(source, destination string) error {
	mount := destination + "-mount"
	if err := os.MkdirAll(mount, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(mount)
	if output, err := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mount, source).CombinedOutput(); err != nil {
		return fmt.Errorf("mount DMG: %w: %s", err, output)
	}
	defer exec.Command("hdiutil", "detach", mount).Run()
	apps, _ := filepath.Glob(filepath.Join(mount, "*.app"))
	if len(apps) != 1 {
		return fmt.Errorf("ungoogled-chromium DMG contains %d applications", len(apps))
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	target := filepath.Join(destination, filepath.Base(apps[0]))
	if output, err := exec.Command("ditto", apps[0], target).CombinedOutput(); err != nil {
		return fmt.Errorf("copy app bundle: %w: %s", err, output)
	}
	return nil
}

func locateExecutable(root string) (string, error) {
	matches, _ := filepath.Glob(filepath.Join(root, "*.app", "Contents", "MacOS", "*"))
	for _, match := range matches {
		if executableOK(match) {
			return match, nil
		}
	}
	return "", fmt.Errorf("ungoogled-chromium executable missing")
}
