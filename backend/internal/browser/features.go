package browser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/httpd"
	"surf-backend/internal/protocol"
	"surf-backend/internal/ws"
)

// ---- per-tab setup ----------------------------------------------------

// setupFeatures runs once per tab after Page/Runtime are enabled.
func (b *Controller) setupFeatures(t *Tab) {
	b.mu.Lock()
	s := t.Session
	b.mu.Unlock()
	b.setupFileChooser(s) // upload interception (M2.2)
	b.setupSecurity(s)    // TLS state events (M2.5)
}

// onURLChanged fires on every main-frame URL change: record history and
// (lazily) resolve the favicon.
func (b *Controller) onURLChanged(t *Tab, u string) {
	b.store.AddHistory(u, "")
	b.refreshFavicon(t)
}

// ---- favicons ----------------------------------------------------------

type favicon struct {
	data  []byte
	ctype string
	hash  string
}

// iconURLLocked returns the tab-strip icon URL; b.mu is held by the caller.
func (b *Controller) iconURLLocked(t *Tab) string {
	ic := b.icons[t.IconKey]
	if ic == nil {
		return ""
	}
	return fmt.Sprintf("/tabicon/%d?v=%s", t.ID, ic.hash)
}

// refreshFavicon resolves and caches the favicon for the tab's current origin.
func (b *Controller) refreshFavicon(t *Tab) {
	b.mu.Lock()
	pageURL := t.URL
	b.mu.Unlock()
	origin := pageOrigin(pageURL)
	if origin == "" {
		return
	}

	b.mu.Lock()
	cached := b.icons[origin] != nil
	if cached {
		if b.tabs[t.ID] != t || pageOrigin(t.URL) != origin {
			b.mu.Unlock()
			return
		}
		changed := t.IconKey != origin
		t.IconKey = origin
		b.mu.Unlock()
		if changed {
			b.broadcastTabs()
		}
		return
	}
	if b.iconFetching[origin] {
		b.mu.Unlock()
		return
	}
	b.iconFetching[origin] = true
	b.mu.Unlock()

	go func() {
		defer func() {
			b.mu.Lock()
			delete(b.iconFetching, origin)
			b.mu.Unlock()
		}()
		// Do not query the active renderer merely to discover a custom icon.
		// Keep renderer-side feature work out of the capture-critical path.
		// The conventional origin favicon is sufficient and can be fetched
		// outside Chromium without disturbing screencast cadence.
		href := origin + "/favicon.ico"
		ic := fetchIcon(href)
		if ic == nil {
			return
		}
		b.mu.Lock()
		b.icons[origin] = ic
		notify := false
		if b.tabs[t.ID] == t && pageOrigin(t.URL) == origin {
			t.IconKey = origin
			notify = true
		}
		b.mu.Unlock()
		if notify {
			b.broadcastTabs()
		}
	}()
}

func pageOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func fetchIcon(href string) *favicon {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(href)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil || len(data) == 0 {
		return nil
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		ct = "image/x-icon"
	}
	sum := sha256.Sum256(data)
	return &favicon{data: data, ctype: ct, hash: hex.EncodeToString(sum[:4])}
}

// ---- downloads ----------------------------------------------------------

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9._ ()\-]`)

func (b *Controller) setupDownloads() {
	_ = os.MkdirAll(b.cfg.DownloadsDir, 0o755)
	_, _ = b.cdp.Call("", "Controller.setDownloadBehavior", map[string]any{
		"behavior": "allowAndName", "downloadPath": b.cfg.DownloadsDir, "eventsEnabled": true,
	})
}

func (b *Controller) onDownloadBegin(ev cdp.Event) {
	var p struct {
		GUID              string `json:"guid"`
		SuggestedFilename string `json:"suggestedFilename"`
	}
	if json.Unmarshal(ev.Params, &p) != nil || p.GUID == "" {
		return
	}
	name := unsafeName.ReplaceAllString(filepath.Base(p.SuggestedFilename), "_")
	if name == "" || name == "." {
		name = "download"
	}
	b.dlMu.Lock()
	// Dedupe against files already on disk.
	final := name
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(b.cfg.DownloadsDir, final)); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(name)
		final = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), i, ext)
	}
	b.dlNames[p.GUID] = final
	b.dlMu.Unlock()
	b.hub.BroadcastJSON(protocol.TextEvent{Type: "toast", Text: "downloading " + final})
}

func (b *Controller) onDownloadProgress(ev cdp.Event) {
	var p struct {
		GUID          string  `json:"guid"`
		State         string  `json:"state"`
		ReceivedBytes float64 `json:"receivedBytes"`
		TotalBytes    float64 `json:"totalBytes"`
	}
	if json.Unmarshal(ev.Params, &p) != nil || p.GUID == "" {
		return
	}
	if p.State != "completed" && p.State != "canceled" {
		// In-flight: push a throttled dlprogress so the client can show a bar.
		b.verbMu.Lock()
		last := b.dlLastPush[p.GUID]
		push := time.Since(last) > 500*time.Millisecond
		if push {
			b.dlLastPush[p.GUID] = time.Now()
		}
		b.verbMu.Unlock()
		if push {
			b.dlMu.Lock()
			name := b.dlNames[p.GUID]
			b.dlMu.Unlock()
			pct := -1
			if p.TotalBytes > 0 {
				pct = int(p.ReceivedBytes / p.TotalBytes * 100)
			}
			b.hub.BroadcastJSON(protocol.DownloadProgressEvent{Type: "dlprogress", Name: name, Pct: pct})
		}
		return
	}
	b.verbMu.Lock()
	delete(b.dlLastPush, p.GUID)
	b.verbMu.Unlock()
	b.dlMu.Lock()
	name := b.dlNames[p.GUID]
	delete(b.dlNames, p.GUID)
	b.dlMu.Unlock()
	if p.State == "canceled" || name == "" {
		return
	}
	_ = os.Rename(filepath.Join(b.cfg.DownloadsDir, p.GUID), filepath.Join(b.cfg.DownloadsDir, name))
	b.hub.BroadcastJSON(protocol.NameEvent{Type: "download", Name: name})
}

func (b *Controller) downloadList() []protocol.DownloadItem {
	entries, _ := os.ReadDir(b.cfg.DownloadsDir)
	items := []protocol.DownloadItem{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		items = append(items, protocol.DownloadItem{
			Name: e.Name(), Size: info.Size(), TS: info.ModTime().Unix(),
		})
	}
	return items
}

// ---- HTTP routes ---------------------------------------------------------

// RegisterRoutes adds feature routes (all behind the auth cookie).
func (b *Controller) RegisterRoutes(srv *httpd.Server) {
	srv.Gated("/tabicon/", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/tabicon/"))
		b.mu.Lock()
		var ic *favicon
		if t := b.tabs[id]; t != nil {
			ic = b.icons[t.IconKey]
		}
		b.mu.Unlock()
		if ic == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ic.ctype)
		w.Header().Set("Cache-Control", "public, max-age=604800")
		_, _ = w.Write(ic.data)
	})
	srv.Gated("/upload", b.handleUpload)
	srv.Gated("/downloads/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(strings.TrimPrefix(r.URL.Path, "/downloads/"))
		if name == "." || name == "/" || strings.HasPrefix(name, ".") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
		http.ServeFile(w, r, filepath.Join(b.cfg.DownloadsDir, name))
	})
}

// ---- feature messages ------------------------------------------------------

// handleFeatureMessage handles message types beyond the M1 set.
func (b *Controller) handleFeatureMessage(c *ws.ClientTransport, t *Tab, session string, command protocol.Command) {
	switch command.Kind() {
	case "zoom":
		b.handleZoom(t, session, command.(*protocol.ZoomCommand))
	case "paste":
		m := command.(*protocol.TextCommand)
		if m.Text != "" {
			_ = b.cdp.Dispatch(session, "Input.insertText", map[string]any{"text": m.Text})
		}
	case "find":
		b.handleFind(c, t, session, command.(*protocol.FindCommand))
	case "suggest":
		m := command.(*protocol.QueryCommand)
		c.SendJSON(protocol.SuggestEvent{Type: "suggest", Items: b.store.Suggest(m.Q)})
	case "hist":
		b.mu.Lock()
		u := t.URL
		b.mu.Unlock()
		c.SendJSON(protocol.LibraryEvent{
			Type: "hist", History: b.store.Recent(50), Bookmarks: b.store.Bookmarks(),
			Starred: b.store.IsBookmarked(u),
		})
	case "bookmark":
		b.mu.Lock()
		u, title := t.URL, t.Title
		b.mu.Unlock()
		on := b.store.ToggleBookmark(u, title)
		msg := "bookmark removed"
		if on {
			msg = "bookmarked"
		}
		c.SendJSON(protocol.TextEvent{Type: "toast", Text: msg})
		c.SendJSON(protocol.BoolEvent{Type: "starred", On: on})
	case "downloads":
		c.SendJSON(protocol.DownloadsEvent{Type: "downloads", Items: b.downloadList()})
	case "dldel":
		m := command.(*protocol.NameCommand)
		// Delete a completed download from the server (Library UI). Name is
		// path-sanitized the same way the /downloads/ route is.
		name := filepath.Base(m.Name)
		if name != "." && name != "/" && !strings.HasPrefix(name, ".") {
			_ = os.Remove(filepath.Join(b.cfg.DownloadsDir, name))
		}
		c.SendJSON(protocol.DownloadsEvent{Type: "downloads", Items: b.downloadList()})
	case "hit":
		m := command.(*protocol.PointCommand)
		// Long-press hit-test (M2.4): coordinates arrive as fractions like all
		// input; convert to CSS px of the (possibly zoomed) viewport.
		b.mu.Lock()
		z := t.Zoom
		vw, vh := b.viewW, b.viewH
		b.mu.Unlock()
		if z < 1 {
			z = 1
		}
		b.handleHit(c, session, m.X*float64(vw)/z, m.Y*float64(vh)/z)
	case "reader":
		go b.handleReader(c, t, session) // heavy evaluate; off the read loop
	case "history":
		m := command.(*protocol.QueryCommand)
		items, total := b.store.Search(m.Q, m.Offset, 50)
		c.SendJSON(protocol.HistoryPageEvent{
			Type: "history", Query: m.Q, Items: items, Offset: m.Offset, Total: total,
		})
	case "histdel":
		m := command.(*protocol.HistoryDeleteCommand)
		b.store.DeleteHistory(m.URL, m.TS)
		c.SendJSON(protocol.TextEvent{Type: "toast", Text: "removed"})
	case "bmdel":
		m := command.(*protocol.URLCommand)
		b.store.RemoveBookmark(m.URL)
		b.mu.Lock()
		u := t.URL
		b.mu.Unlock()
		c.SendJSON(protocol.BoolEvent{Type: "starred", On: b.store.IsBookmarked(u)})
	case "clear":
		m := command.(*protocol.ClearCommand)
		b.handleClear(c, session, m.What)
	}
}

// handleZoom applies an absolute page zoom, recentering on the gesture focus
// (CX/CY are viewport fractions).
func (b *Controller) handleZoom(t *Tab, session string, m *protocol.ZoomCommand) {
	z := m.Scale
	if z < 1.05 {
		z = 1
	}
	if z > 3 {
		z = 3
	}
	b.mu.Lock()
	oldZ := t.Zoom
	if oldZ < 1 {
		oldZ = 1
	}
	t.Zoom = z
	vw, vh := b.viewW, b.viewH
	b.mu.Unlock()

	b.applyView(t)
	if z != oldZ {
		// Keep the gesture focus point on screen: scroll so it's centered.
		expr := fmt.Sprintf(
			`(function(){var px=window.pageXOffset+%f, py=window.pageYOffset+%f;`+
				`window.scrollTo(Math.max(0,px-%d/2), Math.max(0,py-%d/2));})()`,
			m.CX*float64(vw)/oldZ, m.CY*float64(vh)/oldZ, int(float64(vw)/z), int(float64(vh)/z))
		_, _ = b.cdp.Call(session, "Runtime.evaluate", map[string]any{"expression": expr})
	}
	b.hub.BroadcastJSON(protocol.ScaleEvent{Type: "zoom", Scale: z})
}

// finishLongpress runs after the long-press mouse-up. Plain press (sel=true):
// double-click-select the word underneath. After a drag: whatever the drag
// selected. Either way, ship the selection to the client for native copy.
func (b *Controller) finishLongpress(c *ws.ClientTransport, t *Tab, session string, x, y float64, selectWord bool) {
	if selectWord {
		b.mouse(session, "mousePressed", x, y, 2)
		b.mouse(session, "mouseReleased", x, y, 2)
	}
	text := ""
	if v, err := b.cdp.EvaluateString(session, "String(window.getSelection())"); err == nil {
		text = v
	}
	// A drag that selected nothing was a drag, not a copy attempt.
	if !selectWord && text == "" {
		return
	}
	c.SendJSON(protocol.TextEvent{Type: "copytext", Text: text})
}

func (b *Controller) handleFind(c *ws.ClientTransport, t *Tab, session string, m *protocol.FindCommand) {
	if strings.TrimSpace(m.Q) == "" {
		return
	}
	back := m.Dir < 0
	q, _ := json.Marshal(m.Q)
	found, err := b.cdp.EvaluateBool(session, fmt.Sprintf("window.find(%s,false,%t,true,false,true,false)", q, back))
	if err != nil {
		found = false
	}
	c.SendJSON(protocol.BoolEvent{Type: "found", On: found})
}
