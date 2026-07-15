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

	"rbrowser/internal/adblock"
	"rbrowser/internal/cdp"
	"rbrowser/internal/httpd"
	"rbrowser/internal/protocol"
	"rbrowser/internal/ws"
)

// ---- per-tab setup ----------------------------------------------------

// setupFeatures runs once per tab after Page/Runtime are enabled.
func (b *Browser) setupFeatures(t *Tab) {
	b.mu.Lock()
	s := t.Session
	b.mu.Unlock()
	if b.cfg.Adblock {
		// Intercept every request at the Request stage; onRequestPaused
		// fails the listed ones and continues the rest.
		_, _ = b.cdp.Call(s, "Fetch.enable", map[string]any{})
	}
}

// onURLChanged fires on every main-frame URL change: record history and
// (lazily) resolve the favicon.
func (b *Browser) onURLChanged(t *Tab, u string) {
	b.mu.Lock()
	title := t.Title
	b.mu.Unlock()
	b.store.AddHistory(u, title)
	b.refreshFavicon(t)
}

// ---- adblock ----------------------------------------------------------

func (b *Browser) onRequestPaused(ev cdp.Event) {
	var p struct {
		RequestID string `json:"requestId"`
		Request   struct {
			URL string `json:"url"`
		} `json:"request"`
	}
	if json.Unmarshal(ev.Params, &p) != nil {
		return
	}
	if adblock.BlockedURL(p.Request.URL) {
		b.cdp.Send(ev.SessionID, "Fetch.failRequest", map[string]any{
			"requestId": p.RequestID, "errorReason": "BlockedByClient",
		})
		return
	}
	b.cdp.Send(ev.SessionID, "Fetch.continueRequest", map[string]any{"requestId": p.RequestID})
}

// ---- favicons ----------------------------------------------------------

type favicon struct {
	data  []byte
	ctype string
	hash  string
}

// iconURLLocked returns the tab-strip icon URL; b.mu is held by the caller.
func (b *Browser) iconURLLocked(t *Tab) string {
	ic := b.icons[t.IconKey]
	if ic == nil {
		return ""
	}
	return fmt.Sprintf("/tabicon/%d?v=%s", t.ID, ic.hash)
}

// refreshFavicon resolves and caches the favicon for the tab's current origin.
func (b *Browser) refreshFavicon(t *Tab) {
	b.mu.Lock()
	pageURL := t.URL
	s := t.Session
	b.mu.Unlock()
	u, err := url.Parse(pageURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return
	}
	origin := u.Scheme + "://" + u.Host

	b.mu.Lock()
	cached := b.icons[origin] != nil
	if cached {
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
		href := origin + "/favicon.ico"
		res, err := b.cdp.Call(s, "Runtime.evaluate", map[string]any{
			"expression":    `(function(){var l=document.querySelector('link[rel~="icon"],link[rel="shortcut icon"]');return l&&l.href?l.href:'';})()`,
			"returnByValue": true,
		})
		if err == nil {
			var p struct {
				Result struct {
					Value string `json:"value"`
				} `json:"result"`
			}
			if json.Unmarshal(res, &p) == nil && strings.HasPrefix(p.Result.Value, "http") {
				href = p.Result.Value
			}
		}
		ic := fetchIcon(href)
		if ic == nil {
			return
		}
		b.mu.Lock()
		b.icons[origin] = ic
		t.IconKey = origin
		b.mu.Unlock()
		b.broadcastTabs()
	}()
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

func (b *Browser) setupDownloads() {
	_ = os.MkdirAll(b.cfg.DownloadsDir, 0o755)
	_, _ = b.cdp.Call("", "Browser.setDownloadBehavior", map[string]any{
		"behavior": "allowAndName", "downloadPath": b.cfg.DownloadsDir, "eventsEnabled": true,
	})
}

func (b *Browser) onDownloadBegin(ev cdp.Event) {
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
	b.hub.BroadcastJSON(map[string]any{"t": "toast", "text": "downloading " + final})
}

func (b *Browser) onDownloadProgress(ev cdp.Event) {
	var p struct {
		GUID  string `json:"guid"`
		State string `json:"state"`
	}
	if json.Unmarshal(ev.Params, &p) != nil || p.GUID == "" {
		return
	}
	if p.State != "completed" && p.State != "canceled" {
		return
	}
	b.dlMu.Lock()
	name := b.dlNames[p.GUID]
	delete(b.dlNames, p.GUID)
	b.dlMu.Unlock()
	if p.State == "canceled" || name == "" {
		return
	}
	_ = os.Rename(filepath.Join(b.cfg.DownloadsDir, p.GUID), filepath.Join(b.cfg.DownloadsDir, name))
	b.hub.BroadcastJSON(map[string]any{"t": "download", "name": name})
}

func (b *Browser) downloadList() []map[string]any {
	entries, _ := os.ReadDir(b.cfg.DownloadsDir)
	items := []map[string]any{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		items = append(items, map[string]any{
			"name": e.Name(), "size": info.Size(), "ts": info.ModTime().Unix(),
		})
	}
	return items
}

// ---- HTTP routes ---------------------------------------------------------

// RegisterRoutes adds feature routes (all behind the auth cookie).
func (b *Browser) RegisterRoutes(srv *httpd.Server) {
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
func (b *Browser) handleFeatureMessage(c *ws.Client, t *Tab, session string, m *protocol.ClientMessage) {
	switch m.T {
	case "zoom":
		b.handleZoom(t, session, m)
	case "paste":
		if m.Text != "" {
			b.beginMotion(t)
			_, _ = b.cdp.Call(session, "Input.insertText", map[string]any{"text": m.Text})
		}
	case "find":
		b.handleFind(c, t, session, m)
	case "suggest":
		c.SendJSON(map[string]any{"t": "suggest", "items": b.store.Suggest(m.Q)})
	case "hist":
		b.mu.Lock()
		u := t.URL
		b.mu.Unlock()
		c.SendJSON(map[string]any{
			"t": "hist", "hist": b.store.Recent(50), "bookmarks": b.store.Bookmarks(),
			"starred": b.store.IsBookmarked(u),
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
		c.SendJSON(map[string]any{"t": "toast", "text": msg})
		c.SendJSON(map[string]any{"t": "starred", "on": on})
	case "downloads":
		c.SendJSON(map[string]any{"t": "downloads", "items": b.downloadList()})
	}
}

// handleZoom applies an absolute page zoom, recentering on the gesture focus
// (CX/CY are viewport fractions).
func (b *Browser) handleZoom(t *Tab, session string, m *protocol.ClientMessage) {
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
	b.hub.BroadcastJSON(map[string]any{"t": "zoom", "scale": z})
	go b.sendFreshFrame(nil, b.cfg.SharpQuality)
}

// finishLongpress runs after the long-press mouse-up. Plain press (sel=true):
// double-click-select the word underneath. After a drag: whatever the drag
// selected. Either way, ship the selection to the client for native copy.
func (b *Browser) finishLongpress(c *ws.Client, t *Tab, session string, x, y float64, selectWord bool) {
	if selectWord {
		b.mouse(session, "mousePressed", x, y, 2)
		b.mouse(session, "mouseReleased", x, y, 2)
	}
	res, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression": "String(window.getSelection())", "returnByValue": true,
	})
	text := ""
	if err == nil {
		var p struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		if json.Unmarshal(res, &p) == nil {
			text = p.Result.Value
		}
	}
	// A drag that selected nothing was a drag, not a copy attempt.
	if !selectWord && text == "" {
		return
	}
	c.SendJSON(map[string]any{"t": "copytext", "text": text})
	go b.sendFreshFrame(nil, b.cfg.SharpQuality)
}

func (b *Browser) handleFind(c *ws.Client, t *Tab, session string, m *protocol.ClientMessage) {
	if strings.TrimSpace(m.Q) == "" {
		return
	}
	back := m.Dir < 0
	q, _ := json.Marshal(m.Q)
	res, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression":    fmt.Sprintf("window.find(%s,false,%t,true,false,true,false)", q, back),
		"returnByValue": true,
	})
	found := false
	if err == nil {
		var p struct {
			Result struct {
				Value bool `json:"value"`
			} `json:"result"`
		}
		if json.Unmarshal(res, &p) == nil {
			found = p.Result.Value
		}
	}
	c.SendJSON(map[string]any{"t": "found", "on": found})
	go b.sendFreshFrame(nil, b.cfg.SharpQuality)
}
