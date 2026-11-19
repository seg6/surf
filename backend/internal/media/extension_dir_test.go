package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtensionDirectoryIsStableAndContentNamed(t *testing.T) {
	first, second := extensionDirectory(), extensionDirectory()
	if first != second {
		t.Fatalf("extension directory changed between calls: %q vs %q", first, second)
	}
	if !strings.HasPrefix(first, "tab-capture-extension-") || len(first) <= len("tab-capture-extension-") {
		t.Fatalf("extension directory %q is not content-named", first)
	}
}

func TestRemoveStaleExtensionsKeepsCurrentAndUnrelated(t *testing.T) {
	runtimeDir := t.TempDir()
	for _, name := range []string{"tab-capture-extension", "tab-capture-extension-old", "tab-capture-extension-new", "ublock-origin-lite"} {
		if err := os.MkdirAll(filepath.Join(runtimeDir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	removeStaleExtensions(runtimeDir, "tab-capture-extension-new")
	for name, want := range map[string]bool{
		"tab-capture-extension": false, "tab-capture-extension-old": false,
		"tab-capture-extension-new": true, "ublock-origin-lite": true,
	} {
		_, err := os.Stat(filepath.Join(runtimeDir, name))
		if (err == nil) != want {
			t.Fatalf("%s present=%v, want %v", name, err == nil, want)
		}
	}
}

func TestExtensionMutesTabsForTheHost(t *testing.T) {
	script, err := extensionFiles.ReadFile("extension/background.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"muted: true", "tabs.onCreated", "tabs.onUpdated"} {
		if !strings.Contains(string(script), want) {
			t.Fatalf("background.js does not mute tabs on the host: missing %q", want)
		}
	}
}
