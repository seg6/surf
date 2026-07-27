package browserbin

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func makeZIP(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create(name)
	if err == nil {
		_, err = w.Write([]byte(body))
	}
	if closeErr := z.Close(); err == nil {
		err = closeErr
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUnzipRejectsTraversal(t *testing.T) {
	if err := unzip(makeZIP(t, "../escape", "bad"), t.TempDir()); err == nil {
		t.Fatal("accepted traversal path")
	}
}

func TestUnzipRegularFile(t *testing.T) {
	dst := t.TempDir()
	if err := unzip(makeZIP(t, "runtime/browser", "ok"), dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "runtime", "browser"))
	if err != nil || string(data) != "ok" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestPlatformIsSupportedOnBuilder(t *testing.T) {
	archiveName, executable, err := platformFor("chrome")
	if err != nil || archiveName == "" || executable == "" {
		t.Fatalf("chrome archive=%q executable=%q err=%v", archiveName, executable, err)
	}
}
