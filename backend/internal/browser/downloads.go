package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/protocol"
	"surf-backend/internal/web"
)

// Chromium writes GUID-named files into a private staging directory. Only
// successfully published files live in the Library directory. Keep abandoned
// staging files after a crash for recovery; never mistake them for downloads.
type downloadStore struct {
	mu           sync.Mutex
	dir, staging string
	active       map[string]*pendingDownload
}

type pendingDownload struct {
	name     string
	lastPush time.Time
}

var unsafeName = regexp.MustCompile("[^a-zA-Z0-9._ ()-]")

func downloadName(raw string) string {
	raw = filepath.Base(strings.ReplaceAll(raw, "\\", "/"))
	name := strings.Trim(unsafeName.ReplaceAllString(raw, "_"), " .")
	if name == "" {
		name = "download"
	}
	// Leave room for collision suffixes on filesystems with 255-byte limits.
	if len(name) > 180 {
		name = name[:180]
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}
	if strings.HasSuffix(strings.ToLower(name), ".crdownload") {
		name += ".download"
	}
	return name
}

func validDownloadName(name string) bool {
	return name != "" && !strings.HasPrefix(name, ".") &&
		!strings.ContainsAny(name, "/\\\x00:") &&
		!strings.HasSuffix(strings.ToLower(name), ".crdownload")
}

func newDownloadStore(dir string) (*downloadStore, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Join(dir, ".incomplete"), 0o700); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(filepath.Join(dir, ".incomplete"), "session-")
	if err != nil {
		return nil, err
	}
	return &downloadStore{dir: dir, staging: staging, active: make(map[string]*pendingDownload)}, nil
}

// availableLocked checks both disk and in-flight reservations. A bounded loop
// and explicit errors ensure inaccessible directories cannot wedge the event loop.
func (d *downloadStore) availableLocked(name string) (string, error) {
	ext := filepath.Ext(name)
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), i, ext)
		}
		reserved := false
		for _, pending := range d.active {
			if strings.EqualFold(pending.name, candidate) {
				reserved = true
				break
			}
		}
		if reserved {
			continue
		}
		_, err := os.Lstat(filepath.Join(d.dir, candidate))
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", errors.New("too many downloads with this filename")
}

func validDownloadGUID(guid string) bool {
	if guid == "" {
		return false
	}
	for _, c := range guid {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return len(guid) <= 128
}

func (d *downloadStore) begin(guid, suggested string) (string, error) {
	if !validDownloadGUID(guid) {
		return "", errors.New("invalid download identifier")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.active[guid]; ok {
		return "", nil
	} // duplicate event
	name, err := d.availableLocked(downloadName(suggested))
	if err != nil {
		return "", err
	}
	d.active[guid] = &pendingDownload{name: name}
	return name, nil
}

func (d *downloadStore) progress(guid string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.active[guid]
	if p == nil || time.Since(p.lastPush) < 500*time.Millisecond {
		return "", false
	}
	p.lastPush = time.Now()
	return p.name, true
}

// finish uses the platform's atomic no-replace move to publish a complete file,
// without copying large files on the event loop. On failure the original stays
// staged for recovery; no existing download is overwritten.
func (d *downloadStore) finish(guid string, canceled bool) (string, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.active[guid]
	if p == nil {
		return "", "", nil
	}
	delete(d.active, guid)
	src := filepath.Join(d.staging, guid)
	if canceled {
		for _, path := range []string{src, src + ".crdownload"} {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return p.name, "", err
			}
		}
		return p.name, "", nil
	}
	info, err := os.Lstat(src)
	if err != nil {
		return p.name, "", err
	}
	if !info.Mode().IsRegular() {
		return p.name, "", errors.New("download is not a regular file")
	}
	for i := 0; i < 100; i++ {
		name, err := d.availableLocked(p.name)
		if err != nil {
			return p.name, "", err
		}
		err = publishDownload(src, filepath.Join(d.dir, name))
		if errors.Is(err, os.ErrExist) {
			continue
		} // another writer won the race
		if err != nil {
			return p.name, "", fmt.Errorf("publish download (original retained at %s): %w", src, err)
		}
		return p.name, name, nil
	}
	return p.name, "", errors.New("download destination kept changing")
}

// Fallback for platforms without an exclusive rename primitive. Linking is
// atomic and never replaces a destination; cleanup failure does not undo success.
func linkDownload(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		log.Printf("download staging cleanup: %v", err)
	}
	return nil
}

func (d *downloadStore) list() ([]protocol.DownloadItem, error) {
	items := []protocol.DownloadItem{}
	if d == nil {
		return items, errors.New("downloads are unavailable")
	}
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return items, err
	}
	for _, entry := range entries {
		if !validDownloadName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		items = append(items, protocol.DownloadItem{Name: entry.Name(), Size: info.Size(), TS: info.ModTime().Unix()})
	}
	return items, nil
}

func (d *downloadStore) open(name string) (*os.File, error) {
	if d == nil || !validDownloadName(name) {
		return nil, os.ErrNotExist
	}
	root, err := os.OpenRoot(d.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	return root.Open(name)
}

func (d *downloadStore) remove(name string) error {
	if d == nil || !validDownloadName(name) {
		return os.ErrNotExist
	}
	root, err := os.OpenRoot(d.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrNotExist
	}
	return root.Remove(name)
}

func (b *Controller) setupDownloads() error {
	dir := b.cfg.DownloadsDir
	if dir == "" {
		dir = filepath.Join(b.cfg.SurfHome, "downloads")
	}
	store, err := newDownloadStore(dir)
	if err != nil {
		return err
	}
	b.downloads = store
	_, err = b.cdp.Call("", "Browser.setDownloadBehavior", map[string]any{
		"behavior": "allowAndName", "downloadPath": store.staging, "eventsEnabled": true,
	})
	return err
}

func (b *Controller) downloadError(message string, err error) {
	log.Printf("downloads: %s: %v", message, err)
	b.broadcast(protocol.TextEvent{Type: "toast", Text: message + ". Check the Surf server log."})
}

func (b *Controller) onDownloadBegin(ev cdp.Event) {
	var p struct {
		GUID              string
		SuggestedFilename string
	}
	if json.Unmarshal(ev.Params, &p) != nil || b.downloads == nil {
		return
	}
	name, err := b.downloads.begin(p.GUID, p.SuggestedFilename)
	if err != nil {
		b.downloadError("Could not start download", err)
		// Do not block the controller event loop waiting for Chromium.
		_ = b.cdp.Dispatch("", "Browser.cancelDownload", map[string]any{"guid": p.GUID})
		return
	}
	if name != "" {
		b.broadcast(protocol.DownloadProgressEvent{Type: "dlprogress", Name: name, Pct: -1})
	}
}

func (b *Controller) onDownloadProgress(ev cdp.Event) {
	var p struct {
		GUID          string
		State         string
		ReceivedBytes float64
		TotalBytes    float64
	}
	if json.Unmarshal(ev.Params, &p) != nil || b.downloads == nil {
		return
	}
	switch p.State {
	case "inProgress":
		if name, push := b.downloads.progress(p.GUID); push {
			pct := -1
			if p.TotalBytes > 0 {
				pct = int(min(99, max(0, p.ReceivedBytes/p.TotalBytes*100)))
			}
			b.broadcast(protocol.DownloadProgressEvent{Type: "dlprogress", Name: name, Pct: pct})
		}
	case "completed", "canceled":
		oldName, name, err := b.downloads.finish(p.GUID, p.State == "canceled")
		if oldName == "" {
			return
		}
		// 100 clears progress on existing clients too. Only "download" means
		// success; cancellations and failures use the existing toast channel.
		b.broadcast(protocol.DownloadProgressEvent{Type: "dlprogress", Name: oldName, Pct: 100})
		if err != nil {
			b.downloadError("Could not save "+oldName, err)
		} else if p.State == "canceled" {
			b.broadcast(protocol.TextEvent{Type: "toast", Text: "Download canceled: " + oldName})
		} else {
			b.broadcast(protocol.NameEvent{Type: "download", Name: name})
		}
		b.broadcast(protocol.DownloadsEvent{Type: "downloads", Items: b.downloadList()})
	}
}

func (b *Controller) downloadList() []protocol.DownloadItem {
	items, err := b.downloads.list()
	if err != nil {
		b.downloadError("Could not read downloads", err)
	}
	return items
}

func (b *Controller) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, web.APIRoot+"/downloads/")
	file, err := b.downloads.open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "download unavailable", http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	// Completed downloads are immutable to Surf. Pin multi-request transfers
	// to this file generation so delete/recreate cannot splice different files.
	w.Header().Set("ETag", fmt.Sprintf("\"%x-%x\"", info.ModTime().UnixNano(), info.Size()))
	http.ServeContent(w, r, name, info.ModTime(), file)
}
