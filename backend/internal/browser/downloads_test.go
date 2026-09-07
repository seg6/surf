package browser

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"surf-backend/internal/cdp"
)

func testDownloads(t *testing.T) *downloadStore {
	t.Helper()
	d, err := newDownloadStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func stageDownload(t *testing.T, d *downloadStore, guid, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(d.staging, guid), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadNames(t *testing.T) {
	for _, raw := range []string{"", ".", "..", ".hidden", "../../book.epub", "C:\\books\\book.epub", "a\r\nb.txt", "CON", "LPT1.txt", "a.crdownload", strings.Repeat("a", 500)} {
		name := downloadName(raw)
		if !validDownloadName(name) || len(name) > 181 {
			t.Fatalf("%q -> unsafe %q", raw, name)
		}
	}
	d := testDownloads(t)
	for _, guid := range []string{"", "../outside", "/absolute", "a/b", "a\\b", ".."} {
		if _, err := d.begin(guid, "file"); err == nil {
			t.Fatalf("accepted guid %q", guid)
		}
	}
}

func TestDownloadsLifecycleAndCollisions(t *testing.T) {
	d := testDownloads(t)
	for _, guid := range []string{"first", "second"} {
		if _, err := d.begin(guid, "book.epub"); err != nil {
			t.Fatal(err)
		}
		stageDownload(t, d, guid, guid)
	}
	if d.active["first"].name == d.active["second"].name {
		t.Fatal("names not reserved")
	}
	if name, err := d.begin("first", "different"); err != nil || name != "" {
		t.Fatal("duplicate begin not ignored")
	}
	if name, push := d.progress("missing"); push || name != "" {
		t.Fatal("unknown progress accepted")
	}
	if _, push := d.progress("first"); !push {
		t.Fatal("missing first progress")
	}
	if _, push := d.progress("first"); push {
		t.Fatal("progress not throttled")
	}
	if list, err := d.list(); err != nil || len(list) != 0 {
		t.Fatalf("unfinished downloads leaked: %v %v", list, err)
	}
	// An external writer creates the reserved name after downloadWillBegin.
	if err := os.WriteFile(filepath.Join(d.dir, "book.epub"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, guid := range []string{"first", "second"} {
		_, name, err := d.finish(guid, false)
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(filepath.Join(d.dir, name))
		if err != nil || string(body) != guid {
			t.Fatalf("wrong published data: %q %v", body, err)
		}
	}
	body, _ := os.ReadFile(filepath.Join(d.dir, "book.epub"))
	if string(body) != "existing" {
		t.Fatal("overwrote existing file")
	}
	if list, err := d.list(); err != nil || len(list) != 3 {
		t.Fatalf("list: %v %v", list, err)
	}
	if old, name, err := d.finish("first", false); old != "" || name != "" || err != nil {
		t.Fatal("duplicate completion not ignored")
	}
	if len(d.active) != 0 {
		t.Fatal("leaked active downloads")
	}
}

func TestDownloadFailureAndCancellation(t *testing.T) {
	d := testDownloads(t)
	if _, err := d.begin("missing", "missing.txt"); err != nil {
		t.Fatal(err)
	}
	if old, name, err := d.finish("missing", false); old == "" || name != "" || err == nil {
		t.Fatal("missing file reported success")
	}
	if _, err := d.begin("cancel", "cancel.txt"); err != nil {
		t.Fatal(err)
	}
	stageDownload(t, d, "cancel.crdownload", "partial")
	if _, name, err := d.finish("cancel", true); name != "" || err != nil {
		t.Fatal(name, err)
	}
	if _, err := os.Stat(filepath.Join(d.staging, "cancel.crdownload")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial not removed")
	}
	if _, err := d.begin("empty", "empty.txt"); err != nil {
		t.Fatal(err)
	}
	stageDownload(t, d, "empty", "")
	if _, name, err := d.finish("empty", false); name != "empty.txt" || err != nil {
		t.Fatal("empty download failed", err)
	}
}

func TestDownloadPublicationIsExclusive(t *testing.T) {
	d := testDownloads(t)
	stageDownload(t, d, "guid", "new")
	existing := filepath.Join(d.dir, "existing")
	if err := os.WriteFile(existing, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(d.staging, "guid")
	if err := publishDownload(source, existing); !errors.Is(err, os.ErrExist) {
		t.Fatalf("no-clobber: %v", err)
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "original" {
		t.Fatal("existing file replaced")
	}
	data, _ = os.ReadFile(source)
	if string(data) != "new" {
		t.Fatal("source lost")
	}
	if err := publishDownload(source, filepath.Join(d.dir, "missing", "file")); err == nil {
		t.Fatal("missing destination directory accepted")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("failed publication lost original")
	}
}

func TestDownloadsFilesystemErrors(t *testing.T) {
	d := testDownloads(t)
	// ENOTDIR is deterministic even when tests run as root.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	d.dir = filepath.Join(blocked, "child")
	if _, err := d.begin("guid", "file"); err == nil {
		t.Fatal("stat error swallowed")
	}
	if _, err := d.list(); err == nil {
		t.Fatal("list error swallowed")
	}
	if _, err := newDownloadStore(d.dir); err == nil {
		t.Fatal("setup error swallowed")
	}
}

func TestDownloadHTTPAndVisibility(t *testing.T) {
	d := testDownloads(t)
	for _, name := range []string{"book.txt", ".hidden", "file.crdownload"} {
		if err := os.WriteFile(filepath.Join(d.dir, name), []byte("abcdef"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(d.dir, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(d.dir, "link")); err != nil {
		t.Log("symlinks unavailable:", err)
	}
	list, err := d.list()
	if err != nil || len(list) != 1 || list[0].Name != "book.txt" {
		t.Fatalf("list leaked non-downloads: %v %v", list, err)
	}
	b := &Controller{downloads: d}
	for _, name := range []string{".hidden", "file.crdownload", "directory", "link", "../secret", "nested/book.txt", ""} {
		w := httptest.NewRecorder()
		b.handleDownload(w, httptest.NewRequest("GET", "/api/v1/downloads/"+name, nil))
		if w.Code != 404 {
			t.Fatalf("%q status %d", name, w.Code)
		}
		if err := d.remove(name); err == nil {
			t.Fatalf("deleted invalid %q", name)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/downloads/book.txt", nil)
	r.Header.Set("Range", "bytes=1-3")
	b.handleDownload(w, r)
	if w.Code != http.StatusPartialContent || w.Body.String() != "bcd" {
		t.Fatalf("range: %d %q", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("missing attachment header")
	}
	w = httptest.NewRecorder()
	b.handleDownload(w, httptest.NewRequest("HEAD", "/api/v1/downloads/book.txt", nil))
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD failed")
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing file generation tag")
	}
	r = httptest.NewRequest("GET", "/api/v1/downloads/book.txt", nil)
	r.Header.Set("If-Match", "\"different-generation\"")
	w = httptest.NewRecorder()
	b.handleDownload(w, r)
	if w.Code != http.StatusPreconditionFailed {
		t.Fatal("file change not rejected")
	}
	r = httptest.NewRequest("GET", "/api/v1/downloads/book.txt", nil)
	r.Header.Set("If-Match", etag)
	r.Header.Set("Range", "bytes=0-2")
	w = httptest.NewRecorder()
	b.handleDownload(w, r)
	if w.Code != 206 || w.Body.String() != "abc" {
		t.Fatal("pinned range failed")
	}
	w = httptest.NewRecorder()
	b.handleDownload(w, httptest.NewRequest("POST", "/api/v1/downloads/book.txt", nil))
	if w.Code != 405 {
		t.Fatal("POST allowed")
	}
	if err := d.remove("book.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadEventRouting(t *testing.T) {
	d := testDownloads(t)
	b := &Controller{downloads: d}
	event := func(method string, params any) {
		raw, _ := json.Marshal(params)
		b.onEvent(cdp.Event{Method: method, Params: raw})
	}
	event("Browser.downloadWillBegin", map[string]any{"guid": "test", "suggestedFilename": "test.txt"})
	stageDownload(t, d, "test", "contents")
	event("Browser.downloadProgress", map[string]any{"guid": "test", "state": "completed"})
	if list, err := d.list(); err != nil || len(list) != 1 {
		t.Fatalf("Chromium events not routed: %v %v", list, err)
	}
}
