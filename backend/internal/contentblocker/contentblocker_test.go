package contentblocker

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestUnpackRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("bad"))
	_ = writer.Close()
	_ = file.Close()
	if err := unpack(archive, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("accepted path traversal")
	}
}

func TestValidManifest(t *testing.T) {
	root := t.TempDir()
	manifest := `{"name":"uBlock Origin Lite","version":"` + Version + `","manifest_version":3}`
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if !valid(root) {
		t.Fatal("rejected valid manifest")
	}
}
