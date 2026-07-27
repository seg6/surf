package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestRunPreservesTopLevelDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "surf-backend-test")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "surf-backend.exe"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "package.zip")
	if err := run(source, dst); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 1 || archive.File[0].Name != "surf-backend-test/surf-backend.exe" {
		t.Fatalf("entries=%v", archive.File)
	}
}
