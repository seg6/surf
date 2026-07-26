package browser

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveUploadedFiles(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", "../../hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if err := req.ParseMultipartForm(uploadFormMemoryBytes); err != nil {
		t.Fatal(err)
	}
	defer req.MultipartForm.RemoveAll()

	paths, err := saveUploadedFiles(req.MultipartForm, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("saved %d files, want 1", len(paths))
	}
	if !strings.HasSuffix(filepath.Base(paths[0]), "hello.txt") {
		t.Fatalf("saved path %q did not preserve sanitized filename", paths[0])
	}
	b, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Fatalf("saved body = %q, want hello", string(b))
	}
}

func TestCopyUploadedFileCapsSize(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "upload.bin")
	err := copyUploadedFile(dst, strings.NewReader("1234"), 3)
	if !errors.Is(err, errUploadTooLarge) {
		t.Fatalf("copy error = %v, want %v", err, errUploadTooLarge)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("oversized upload was not removed; stat err = %v", statErr)
	}
}
