package browser

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store keeps browsing history (append-only JSONL) and bookmarks (one JSON
// file) in the profile dir, so they survive restarts alongside cookies.
type Store struct {
	mu        sync.Mutex
	histPath  string
	bmPath    string
	hist      []Entry
	bookmarks []Entry
}

type Entry struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	TS    int64  `json:"ts"`
}

// histKeep bounds RAM and the on-disk file after rotation; at ~100 visits/day
// this is roughly a year and a half of history.
const histKeep = 50000

func NewStore(profile string) *Store {
	s := &Store{
		histPath: filepath.Join(profile, "history.jsonl"),
		bmPath:   filepath.Join(profile, "bookmarks.json"),
	}
	if f, err := os.Open(s.histPath); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			var e Entry
			if json.Unmarshal(sc.Bytes(), &e) == nil && e.URL != "" {
				s.hist = append(s.hist, e)
			}
		}
		_ = f.Close()
		if len(s.hist) > histKeep {
			s.hist = s.hist[len(s.hist)-histKeep:]
		}
	}
	if b, err := os.ReadFile(s.bmPath); err == nil {
		_ = json.Unmarshal(b, &s.bookmarks)
	}
	return s
}

func historyWorthy(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// AddHistory records a visit; consecutive duplicates are collapsed and the
// title of the latest entry is patched in later when it becomes known.
func (s *Store) AddHistory(url, title string) {
	if !historyWorthy(url) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.hist); n > 0 && s.hist[n-1].URL == url {
		if title != "" && s.hist[n-1].Title != title {
			s.hist[n-1].Title = title
		}
		return
	}
	e := Entry{URL: url, Title: title, TS: time.Now().Unix()}
	s.hist = append(s.hist, e)
	if len(s.hist) > histKeep {
		s.hist = s.hist[len(s.hist)-histKeep:]
	}
	if b, err := json.Marshal(e); err == nil {
		if f, err := os.OpenFile(s.histPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			_, _ = f.Write(append(b, '\n'))
			_ = f.Close()
		}
	}
}

// SetTitle patches the newest history entry for url once the page title lands.
func (s *Store) SetTitle(url, title string) {
	if title == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.hist) - 1; i >= 0 && i >= len(s.hist)-10; i-- {
		if s.hist[i].URL == url {
			s.hist[i].Title = title
			return
		}
	}
}

// Suggest returns up to 6 history/bookmark matches for an address-bar prefix,
// newest first, deduped by URL.
func (s *Store) Suggest(q string) []Entry {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var out []Entry
	add := func(e Entry) bool {
		if seen[e.URL] {
			return false
		}
		if strings.Contains(strings.ToLower(e.URL), q) || strings.Contains(strings.ToLower(e.Title), q) {
			seen[e.URL] = true
			out = append(out, e)
		}
		return len(out) >= 6
	}
	for _, e := range s.bookmarks {
		if add(e) {
			return out
		}
	}
	for i := len(s.hist) - 1; i >= 0; i-- {
		if add(s.hist[i]) {
			break
		}
	}
	return out
}

// Recent returns the newest n history entries, newest first.
func (s *Store) Recent(n int) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, n)
	for i := len(s.hist) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, s.hist[i])
	}
	return out
}

// Search pages through history newest-first, optionally filtered by a
// case-insensitive substring of URL or title. Returns the page and the total
// match count (for "showing X of Y" UI).
func (s *Store) Search(q string, offset, limit int) (page []Entry, total int) {
	q = strings.ToLower(strings.TrimSpace(q))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	page = []Entry{}
	for i := len(s.hist) - 1; i >= 0; i-- {
		e := s.hist[i]
		if q != "" && !strings.Contains(strings.ToLower(e.URL), q) && !strings.Contains(strings.ToLower(e.Title), q) {
			continue
		}
		if total >= offset && len(page) < limit {
			page = append(page, e)
		}
		total++
	}
	return page, total
}

// DeleteHistory removes every entry matching url (and ts when nonzero).
func (s *Store) DeleteHistory(url string, ts int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.hist[:0]
	for _, e := range s.hist {
		if e.URL == url && (ts == 0 || e.TS == ts) {
			continue
		}
		kept = append(kept, e)
	}
	s.hist = kept
	s.rewriteHistoryLocked()
}

// ClearHistory wipes history in RAM and on disk.
func (s *Store) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hist = nil
	_ = os.Remove(s.histPath)
}

// rewriteHistoryLocked persists the full in-RAM history (used after deletes;
// appends stay cheap for the common path). s.mu held by caller.
func (s *Store) rewriteHistoryLocked() {
	tmp := s.histPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	for _, e := range s.hist {
		if b, err := json.Marshal(e); err == nil {
			_, _ = w.Write(append(b, '\n'))
		}
	}
	_ = w.Flush()
	_ = f.Close()
	_ = os.Rename(tmp, s.histPath)
}

// ToggleBookmark adds url to bookmarks, or removes it if already present.
// Returns true when the result is "bookmarked".
func (s *Store) ToggleBookmark(url, title string) bool {
	if !historyWorthy(url) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.bookmarks {
		if e.URL == url {
			s.bookmarks = append(s.bookmarks[:i], s.bookmarks[i+1:]...)
			s.saveBookmarksLocked()
			return false
		}
	}
	s.bookmarks = append([]Entry{{URL: url, Title: title, TS: time.Now().Unix()}}, s.bookmarks...)
	s.saveBookmarksLocked()
	return true
}

// RemoveBookmark deletes a bookmark by URL (bookmarks manager, M3.2).
func (s *Store) RemoveBookmark(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.bookmarks {
		if e.URL == url {
			s.bookmarks = append(s.bookmarks[:i], s.bookmarks[i+1:]...)
			s.saveBookmarksLocked()
			return
		}
	}
}

func (s *Store) IsBookmarked(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.bookmarks {
		if e.URL == url {
			return true
		}
	}
	return false
}

func (s *Store) Bookmarks() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.bookmarks))
	copy(out, s.bookmarks)
	return out
}

func (s *Store) saveBookmarksLocked() {
	if b, err := json.Marshal(s.bookmarks); err == nil {
		_ = os.WriteFile(s.bmPath, b, 0o600)
	}
}
