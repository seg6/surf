// verbs.go implements the "browser verbs" the stream viewer lacked: JS
// dialogs, file-upload interception, link hit-testing, TLS state, native
// error surfaces, reader-mode extraction, latency echo and data clearing.
// All wire additions are additive JSON, individually listed in config.Caps.
package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"surf-backend/internal/cdp"
	"surf-backend/internal/protocol"
	"surf-backend/internal/ws"
)

const (
	uploadMaxRequestBytes = 65 << 20
	uploadMaxFileBytes    = 64 << 20
	uploadFormMemoryBytes = 8 << 20
	// Chromium consumes accepted file paths asynchronously; keep a bounded grace window.
	uploadCleanupDelay = 30 * time.Minute
)

var errUploadTooLarge = errors.New("upload file too large")

// ---- JS dialogs (M2.1) --------------------------------------------------
//
// Without this, one confirm() soft-locks the page: Chromium blocks the
// renderer until the dialog is handled, and nothing on screen says why.

func (b *Browser) onJavascriptDialog(ev cdp.Event) {
	var p struct {
		Message       string `json:"message"`
		Type          string `json:"type"` // alert|confirm|prompt|beforeunload
		DefaultPrompt string `json:"defaultPrompt"`
	}
	if json.Unmarshal(ev.Params, &p) != nil {
		return
	}
	// beforeunload: auto-accept. The user pressed back/close on purpose; a
	// "leave site?" dialog on a remote screen is pure friction.
	if p.Type == "beforeunload" {
		b.cdp.Send(ev.SessionID, "Page.handleJavaScriptDialog", map[string]any{"accept": true})
		return
	}
	b.verbMu.Lock()
	b.dialogSessions[ev.SessionID] = true
	b.verbMu.Unlock()
	b.hub.BroadcastJSON(map[string]any{
		"t": "dialog", "kind": p.Type, "text": p.Message, "def": p.DefaultPrompt,
	})
}

func (b *Browser) onJavascriptDialogClosed(ev cdp.Event) {
	b.verbMu.Lock()
	delete(b.dialogSessions, ev.SessionID)
	b.verbMu.Unlock()
	// Covers dialogs dismissed by navigation etc. so the client UI can drop.
	b.hub.BroadcastJSON(map[string]any{"t": "dialogdone"})
}

// handleDialogReply answers whichever session is showing a dialog. In
// practice that's the active tab; if several tabs dialog at once, answer all
// pending ones identically rather than wedge.
func (b *Browser) handleDialogReply(m *protocol.ClientMessage) {
	b.verbMu.Lock()
	sessions := make([]string, 0, len(b.dialogSessions))
	for s := range b.dialogSessions {
		sessions = append(sessions, s)
	}
	b.dialogSessions = map[string]bool{}
	b.verbMu.Unlock()
	for _, s := range sessions {
		params := map[string]any{"accept": m.Accept}
		if m.Accept && m.Text != "" {
			params["promptText"] = m.Text
		}
		b.cdp.Send(s, "Page.handleJavaScriptDialog", params)
	}
}

// ---- file uploads (M2.2) -------------------------------------------------
//
// Page.setInterceptFileChooserDialog suppresses Chromium's chooser and emits
// Page.fileChooserOpened with the input's backendNodeId. The client picks
// files, POSTs them to /upload, and we attach them via DOM.setFileInputFiles.

func (b *Browser) setupFileChooser(session string) {
	_, _ = b.cdp.Call(session, "DOM.enable", nil) // setFileInputFiles wants the DOM agent
	_, _ = b.cdp.Call(session, "Page.setInterceptFileChooserDialog", map[string]any{"enabled": true})
}

func (b *Browser) onFileChooserOpened(ev cdp.Event) {
	var p struct {
		Mode          string `json:"mode"` // selectSingle|selectMultiple
		BackendNodeID int64  `json:"backendNodeId"`
	}
	if json.Unmarshal(ev.Params, &p) != nil || p.BackendNodeID == 0 {
		return
	}
	b.verbMu.Lock()
	b.chooserSession = ev.SessionID
	b.chooserNode = p.BackendNodeID
	b.verbMu.Unlock()
	b.hub.BroadcastJSON(map[string]any{
		"t": "filechooser", "multiple": p.Mode == "selectMultiple",
	})
}

// handleUpload is the POST /upload route: multipart files land in UploadsDir
// and get attached to the intercepted input. An empty form cancels.
func (b *Browser) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	b.verbMu.Lock()
	session, node := b.chooserSession, b.chooserNode
	b.chooserSession, b.chooserNode = "", 0
	b.verbMu.Unlock()
	if node == 0 {
		http.Error(w, "no pending file chooser", http.StatusConflict)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxRequestBytes)
	if err := r.ParseMultipartForm(uploadFormMemoryBytes); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad multipart", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	paths, err := saveUploadedFiles(r.MultipartForm, b.cfg.UploadsDir)
	if errors.Is(err, errUploadTooLarge) {
		http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		log.Printf("upload: save: %v", err)
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}
	// Empty selection = cancel: attach nothing, page sees no change.
	if len(paths) > 0 {
		if _, err := b.cdp.Call(session, "DOM.setFileInputFiles", map[string]any{
			"files": paths, "backendNodeId": node,
		}); err != nil {
			cleanupUploadedFiles(paths)
			log.Printf("upload: setFileInputFiles: %v", err)
			http.Error(w, "attach failed", http.StatusBadGateway)
			return
		}
		scheduleUploadCleanup(paths)
		log.Printf("upload: attached %d file(s)", len(paths))
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"n":%d}`, len(paths))
}

func saveUploadedFiles(form *multipart.Form, dir string) ([]string, error) {
	if form == nil || len(form.File) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for _, fhs := range form.File {
		for _, fh := range fhs {
			path, err := saveUploadedFile(fh, dir)
			if err != nil {
				cleanupUploadedFiles(paths)
				return nil, err
			}
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func saveUploadedFile(fh *multipart.FileHeader, dir string) (string, error) {
	name := unsafeName.ReplaceAllString(filepath.Base(fh.Filename), "_")
	if name == "" || name == "." {
		name = fmt.Sprintf("upload-%d", time.Now().UnixNano())
	}
	dst := filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), name))
	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	if err := copyUploadedFile(dst, src, uploadMaxFileBytes); err != nil {
		return "", err
	}
	return dst, nil
}

func copyUploadedFile(dst string, src io.Reader, maxBytes int64) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(src, maxBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || n > maxBytes {
		_ = os.Remove(dst)
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return errUploadTooLarge
	}
	return nil
}

func cleanupUploadedFiles(paths []string) {
	for _, p := range paths {
		if p != "" {
			_ = os.Remove(p)
		}
	}
}

func scheduleUploadCleanup(paths []string) {
	keep := append([]string(nil), paths...)
	time.AfterFunc(uploadCleanupDelay, func() { cleanupUploadedFiles(keep) })
}

// ---- link hit-test (M2.4) ------------------------------------------------

// hitExpr climbs from the element under a point to the nearest link and/or
// image. Coordinates are CSS px of the emulated viewport (like all input).
const hitExpr = `(function(x,y){
  var e = document.elementFromPoint(x,y), href='', img='', text='';
  for (var n=e; n; n=n.parentElement) {
    if (!img && n.tagName==='IMG' && n.src) img=n.src;
    if (!href && n.tagName==='A' && n.href) { href=n.href; text=(n.textContent||'').trim().slice(0,120); break; }
  }
  return {href:href, img:img, text:text};
})(%d,%d)`

// handleHit answers a long-press hit-test: what's under the finger?
func (b *Browser) handleHit(c *ws.Client, session string, x, y float64) {
	res, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression": fmt.Sprintf(hitExpr, int(x), int(y)), "returnByValue": true,
	})
	if err != nil {
		return
	}
	var p struct {
		Result struct {
			Value struct {
				Href string `json:"href"`
				Img  string `json:"img"`
				Text string `json:"text"`
			} `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(res, &p) != nil {
		return
	}
	v := p.Result.Value
	c.SendJSON(map[string]any{"t": "linkinfo", "href": v.Href, "img": v.Img, "text": v.Text})
}

// openInNewTab backgrounds nothing: new tabs auto-focus by design (popups).
func (b *Browser) openInNewTab(rawURL string) {
	if rawURL == "" {
		return
	}
	_, _ = b.cdp.Call("", "Target.createTarget", map[string]any{"url": rawURL})
}

// ---- TLS state (M2.5) ----------------------------------------------------

func (b *Browser) setupSecurity(session string) {
	_, _ = b.cdp.Call(session, "Security.enable", nil)
}

func (b *Browser) onSecurityStateChanged(ev cdp.Event) {
	var p struct {
		SecurityState string `json:"securityState"` // secure|insecure|neutral|...
	}
	if json.Unmarshal(ev.Params, &p) != nil || p.SecurityState == "" {
		return
	}
	b.mu.Lock()
	t := b.bySession[ev.SessionID]
	if t != nil {
		t.Security = p.SecurityState
	}
	active := t != nil && t.ID == b.activeID
	b.mu.Unlock()
	if active {
		b.hub.BroadcastJSON(map[string]any{"t": "security", "state": p.SecurityState})
	}
}

// ---- native error surface (M2.5) ----------------------------------------

// noteNavigationError fires when a main frame lands on chrome-error://; the
// caller kept the tab's previous URL so the omnibox doesn't show garbage.
func (b *Browser) noteNavigationError(t *Tab) {
	b.mu.Lock()
	active := t.ID == b.activeID
	u := t.URL
	b.mu.Unlock()
	if active {
		b.hub.BroadcastJSON(map[string]any{"t": "pageerror", "url": u})
	}
}

// ---- reader mode (M1.5) --------------------------------------------------

// readerExpr is a compact article extractor: strip chrome-y elements, prefer
// <article>/<main>/#content, fall back to the densest text block. Not
// Readability, but right for most article pages; upgrade path is vendoring
// readability.js behind the same message.
const readerExpr = `(function(){
  function pick(){
    var sel=['article','main','[role=main]','#content','.post-content','.article-body'];
    for (var i=0;i<sel.length;i++){ var e=document.querySelector(sel[i]); if(e&&e.textContent.trim().length>200) return e; }
    var best=document.body, bestLen=0, ps=document.getElementsByTagName('p');
    var counts=[];
    for (var j=0;j<ps.length;j++){ var par=ps[j].parentElement; if(!par) continue;
      var found=null; for(var k=0;k<counts.length;k++) if(counts[k].el===par){found=counts[k];break;}
      if(!found){found={el:par,len:0};counts.push(found);}
      found.len+=ps[j].textContent.length; }
    for (var m=0;m<counts.length;m++) if(counts[m].len>bestLen){bestLen=counts[m].len;best=counts[m].el;}
    return best;
  }
  var root=pick().cloneNode(true);
  var kill=root.querySelectorAll('script,style,nav,header,footer,aside,iframe,form,noscript,svg,button,[role=navigation],[aria-hidden=true]');
  for (var i=kill.length-1;i>=0;i--) kill[i].parentNode.removeChild(kill[i]);
  var all=root.querySelectorAll('*');
  for (var j=0;j<all.length;j++){ var el=all[j], keep={href:1,src:1,alt:1,title:1,colspan:1,rowspan:1};
    for (var a=el.attributes.length-1;a>=0;a--){ var n=el.attributes[a].name; if(!keep[n]) el.removeAttribute(n); } }
  return JSON.stringify({title:document.title||'', html:root.innerHTML.slice(0,600000)});
})()`

func (b *Browser) handleReader(c *ws.Client, t *Tab, session string) {
	b.mu.Lock()
	u := t.URL
	b.mu.Unlock()
	if !b.isActiveSession(session) {
		return
	}
	res, err := b.cdp.Call(session, "Runtime.evaluate", map[string]any{
		"expression": readerExpr, "returnByValue": true,
	})
	if err != nil {
		if b.isActiveSession(session) {
			c.SendJSON(map[string]any{"t": "reader", "ok": false})
		}
		return
	}
	var p struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	var article struct {
		Title string `json:"title"`
		HTML  string `json:"html"`
	}
	if !b.isActiveSession(session) {
		return
	}
	if json.Unmarshal(res, &p) != nil || json.Unmarshal([]byte(p.Result.Value), &article) != nil || strings.TrimSpace(article.HTML) == "" {
		c.SendJSON(map[string]any{"t": "reader", "ok": false})
		return
	}
	c.SendJSON(map[string]any{
		"t": "reader", "ok": true, "title": article.Title, "html": article.HTML, "url": u,
	})
}

// ---- data clearing (M3.4) ------------------------------------------------

func (b *Browser) handleClear(c *ws.Client, session, what string) {
	switch what {
	case "history":
		b.store.ClearHistory()
		c.SendJSON(map[string]any{"t": "toast", "text": "history cleared"})
	case "cookies":
		_, err := b.cdp.Call(session, "Network.clearBrowserCookies", nil)
		if err != nil {
			log.Printf("clear cookies: %v", err)
		}
		c.SendJSON(map[string]any{"t": "toast", "text": "cookies cleared"})
	case "cache":
		_, _ = b.cdp.Call(session, "Network.clearBrowserCache", nil)
		c.SendJSON(map[string]any{"t": "toast", "text": "cache cleared"})
	}
}
