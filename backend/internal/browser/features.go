package browser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"surf-backend/internal/protocol"
	"surf-backend/internal/transport"
	"surf-backend/internal/web"
)

// ---- per-tab setup ----------------------------------------------------

// setupFeatures runs once per tab after Page/Runtime are enabled.
func (b *Controller) setupFeatures(t *Tab) {
	b.mu.Lock()
	s := t.Session
	b.mu.Unlock()
	b.setupFileChooser(s) // upload interception (M2.2)
	b.setupSecurity(s)    // TLS state events (M2.5)
	b.setupPageFullscreen(s)
	b.setupEditableObserver(s)
}

// onURLChanged fires on every main-frame URL change and records history.
// Favicon discovery waits for the document to load so declared icon links
// belong to the new page rather than the document being replaced.
func (b *Controller) onURLChanged(t *Tab, u string) {
	b.store.AddHistory(u, "")
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
	return fmt.Sprintf("%s/tab-icons/%d?v=%s", web.APIRoot, t.ID, ic.hash)
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
		b.mu.Lock()
		session := t.Session
		b.mu.Unlock()
		// Prefer the page's declared touch icon: it is usually a PNG and works
		// on old ImageIO versions that cannot render modern SVG favicons.
		href, _ := b.cdp.EvaluateString(session, `(function(){
			var selectors=['link[rel~="apple-touch-icon"]','link[rel~="icon"]','link[rel="shortcut icon"]'];
			for(var i=0;i<selectors.length;i++){var n=document.querySelector(selectors[i]);if(n&&n.href)return n.href;}
			return '';
		})()`)
		if href == "" {
			href = origin + "/favicon.ico"
		}
		ic := fetchIcon(href)
		if ic == nil && href != origin+"/favicon.ico" {
			ic = fetchIcon(origin + "/favicon.ico")
		}
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

// handleFeatureMessage handles message types beyond the M1 set.
func (b *Controller) handleFeatureMessage(c *transport.Client, t *Tab, session string, command protocol.Command) {
	kind := command.Kind()
	switch m := command.(type) {
	case *protocol.TextCommand:
		if kind != "paste" {
			return
		}
		if m.Text != "" {
			_ = b.cdp.Dispatch(session, "Input.insertText", map[string]any{"text": m.Text})
		}
	case *protocol.FindCommand:
		if kind == "find" {
			b.handleFind(c, t, session, m)
		}
	case *protocol.QueryCommand:
		switch kind {
		case "suggest":
			b.send(c, protocol.SuggestEvent{Type: "suggest", Items: b.store.Suggest(m.Q)})
		case "history":
			items, total := b.store.Search(m.Q, m.Offset, 50)
			b.send(c, protocol.HistoryPageEvent{
				Type: "history", Query: m.Q, Items: items, Offset: m.Offset, Total: total,
			})
		}
	case *protocol.NameCommand:
		if kind != "dldel" {
			return
		}
		if err := b.downloads.remove(m.Name); err != nil {
			b.downloadError("Could not delete download", err)
		}
		b.send(c, protocol.DownloadsEvent{Type: "downloads", Items: b.downloadList()})
	case *protocol.HistoryDeleteCommand:
		if kind == "histdel" {
			b.store.DeleteHistory(m.URL, m.TS)
			b.send(c, protocol.TextEvent{Type: "toast", Text: "removed"})
		}
	case *protocol.URLCommand:
		if kind == "bmdel" {
			b.store.RemoveBookmark(m.URL)
			b.mu.Lock()
			u := t.URL
			b.mu.Unlock()
			b.send(c, protocol.BoolEvent{Type: "starred", On: b.store.IsBookmarked(u)})
		}
	case *protocol.ClearCommand:
		if kind == "clear" {
			b.handleClear(c, session, m.What)
		}
	case *protocol.EmptyCommand:
		switch kind {
		case "hist":
			b.mu.Lock()
			u := t.URL
			b.mu.Unlock()
			b.send(c, protocol.LibraryEvent{
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
			b.send(c, protocol.TextEvent{Type: "toast", Text: msg})
			b.send(c, protocol.BoolEvent{Type: "starred", On: on})
		case "downloads":
			b.send(c, protocol.DownloadsEvent{Type: "downloads", Items: b.downloadList()})
		case "reader":
			go b.handleReader(c, t, session) // heavy evaluate; off the controller loop
		}
	}
}

func (b *Controller) handleFind(c *transport.Client, t *Tab, session string, m *protocol.FindCommand) {
	if strings.TrimSpace(m.Q) == "" {
		return
	}
	back := m.Dir < 0
	q, _ := json.Marshal(m.Q)
	found, err := b.cdp.EvaluateBool(session, fmt.Sprintf("window.find(%s,false,%t,true,false,true,false)", q, back))
	if err != nil {
		found = false
	}
	b.send(c, protocol.BoolEvent{Type: "found", On: found})
}
