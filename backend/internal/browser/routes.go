package browser

import (
	"net/http"
	"strconv"
	"strings"

	"surf-backend/internal/web"
)

// RegisterRoutes binds one controller directly (used by integration hosts).
// The server uses Manager.RegisterRoutes to resolve the current browser across
// setup handoffs without replacing authenticated HTTP handlers.
func (b *Controller) RegisterRoutes(server *web.Server) {
	server.Gated(web.APIRoot+"/tab-icons/", b.handleTabIcon)
	server.Gated(web.APIRoot+"/uploads", b.handleUpload)
	server.Gated(web.APIRoot+"/downloads/", b.handleDownload)
}
func (b *Controller) handleTabIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, web.APIRoot+"/tab-icons/"))
	b.mu.Lock()
	var icon *favicon
	if tab := b.tabs[id]; tab != nil {
		icon = b.icons[tab.IconKey]
	}
	b.mu.Unlock()
	if icon == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", icon.ctype)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = w.Write(icon.data)
}
