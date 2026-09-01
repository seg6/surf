package browser

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"surf-backend/internal/web"
)

// RegisterRoutes binds browser-owned HTTP resources to this server/browser
// generation. A browser crash ends the generation, so handlers never need to
// resolve a second independently supervised browser lifetime.
func (b *Controller) RegisterRoutes(server *web.Server) {
	server.Gated(web.APIRoot+"/tab-icons/", func(w http.ResponseWriter, r *http.Request) {
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
	})
	server.Gated(web.APIRoot+"/uploads", b.handleUpload)
	server.Gated(web.APIRoot+"/downloads/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(strings.TrimPrefix(r.URL.Path, web.APIRoot+"/downloads/"))
		if name == "." || name == "/" || strings.HasPrefix(name, ".") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
		http.ServeFile(w, r, filepath.Join(b.cfg.DownloadsDir, name))
	})
}
