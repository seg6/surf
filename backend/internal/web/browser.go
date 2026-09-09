package web

import "net/http"

func (s *Server) handleBrowserMode(w http.ResponseWriter, r *http.Request, admin bool) {
	w.Header().Set("Cache-Control", "no-store")
	if s.browser == nil {
		http.Error(w, "browser setup is unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.browser.Status())
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Action   string `json:"action"`
		Revision uint64 `json:"revision"`
		Force    bool   `json:"force"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if !admin && request.Action != "resume" {
		http.Error(w, "only the computer can start browser setup", http.StatusForbidden)
		return
	}
	if err := s.browser.Request(request.Action, request.Revision, request.Force); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusAccepted, s.browser.Status())
}
