package browserbin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"surf-backend/internal/cdp"
)

func TestVersionComparison(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"150.0.7871.186-1", "150.0.7871.186-1", 0},
		{"150.0.7871.186-2", "150.0.7871.186-1", 1},
		{"151.0.1.0", "150.9.9.9", 1},
		{"149.0.0.0", "150.0.0.0", -1},
	} {
		if got := compareVersion(test.left, test.right); got != test.want {
			t.Errorf("compareVersion(%q,%q)=%d want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestManagedStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	want := installation{
		Version: "150.0.7871.186-1", Executable: filepath.Join("version", "chrome"),
		CheckedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := writeCurrent(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := readCurrent(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != want.Version || got.Executable != want.Executable || !got.CheckedAt.Equal(want.CheckedAt) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestFindSystemRejectsNonBrowser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(path, []byte("not a browser"), 0o755); err != nil {
		t.Fatal(err)
	}
	if compatible(path) {
		t.Fatal("accepted invalid executable")
	}
}

func TestPlatformRelease(t *testing.T) {
	repository, pattern, err := platformRelease()
	if err != nil {
		t.Fatal(err)
	}
	if repository == "" || pattern == nil {
		t.Fatalf("repository=%q pattern=%v", repository, pattern)
	}
}

func TestManagedDownload(t *testing.T) {
	if os.Getenv("SURF_TEST_BROWSER_DOWNLOAD") != "1" {
		t.Skip("set SURF_TEST_BROWSER_DOWNLOAD=1 for release-asset smoke test")
	}
	home := os.Getenv("SURF_TEST_BROWSER_HOME")
	if home == "" {
		home = t.TempDir()
	}
	path, version, err := EnsureManaged(home)
	if err != nil {
		t.Fatal(err)
	}
	if version == "" || !compatible(path) {
		t.Fatalf("path=%q version=%q", path, version)
	}
	profile := filepath.Join(home, "smoke-profile")
	client, process, err := cdp.Launch(cdp.LaunchConfig{ChromePath: path, Profile: profile, W: 800, H: 600})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Kill()
	defer client.Close()
	raw, err := client.Call("", "Browser.getVersion", nil)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil || result["product"] == "" {
		t.Fatalf("Browser.getVersion=%s err=%v", raw, err)
	}
}
