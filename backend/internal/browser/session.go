package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"surf-backend/internal/atomicfile"
)

const browserSessionVersion = 1

type browserSession struct {
	Version int      `json:"version"`
	Tabs    []string `json:"tabs"`
	Active  int      `json:"active"`
	Mobile  bool     `json:"mobile"`
}

func browserSessionPath(home string) string {
	return filepath.Join(home, "browser-session.json")
}

func loadBrowserSession(home string) browserSession {
	data, err := os.ReadFile(browserSessionPath(home))
	if err != nil {
		return browserSession{}
	}
	var session browserSession
	if json.Unmarshal(data, &session) != nil || session.Version != browserSessionVersion {
		return browserSession{}
	}
	if session.Active < 0 || session.Active >= len(session.Tabs) {
		session.Active = 0
	}
	return session
}

// SaveSession records browser UI state outside Chromium's profile. Chromium
// normally restores its own tabs, while this small file supplies a reliable
// fallback after a clean idle shutdown or an unexpected browser exit.
func (b *Controller) SaveSession() error {
	b.mu.Lock()
	ids := make([]int, 0, len(b.tabs))
	for id := range b.tabs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	session := browserSession{Version: browserSessionVersion, Mobile: b.mobile}
	for _, id := range ids {
		tab := b.tabs[id]
		if tab == nil || !restorableURL(tab.URL) {
			continue
		}
		if id == b.activeID {
			session.Active = len(session.Tabs)
		}
		session.Tabs = append(session.Tabs, tab.URL)
	}
	b.mu.Unlock()
	// A controller which failed before target discovery must not erase the
	// last useful session.
	if len(session.Tabs) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	path := browserSessionPath(b.cfg.SurfHome)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write browser session: %w", err)
	}
	if err := atomicfile.Replace(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("install browser session: %w", err)
	}
	return nil
}

func restorableURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || raw == "about:blank"
}

// prepareStartupSession lets Chromium's native profile restore win when it
// produced real page targets. If it did not, recreate the last explicit Surf
// session before target discovery. In either case, target attachment is
// treated as a batch so asynchronous discovery cannot accidentally focus a
// different tab.
func (b *Controller) prepareStartupSession() {
	session := b.startupSession
	if len(session.Tabs) == 0 {
		return
	}
	raw, err := b.cdp.Call("", "Target.getTargets", nil)
	if err != nil {
		return
	}
	var result struct {
		Targets []targetInfo `json:"targetInfos"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return
	}
	var pages, realPages []targetInfo
	for _, target := range result.Targets {
		if target.Type != "page" {
			continue
		}
		pages = append(pages, target)
		if restorableURL(target.URL) && target.URL != "about:blank" {
			realPages = append(realPages, target)
		}
	}
	activeURL := session.Tabs[session.Active]
	if len(realPages) > 0 {
		// Target.getTargets does not promise tab order. Match the explicit Surf
		// session first, including duplicate URLs, then append native-only tabs.
		ordered := make([]targetInfo, 0, len(realPages))
		used := make(map[string]bool, len(realPages))
		activeTarget := ""
		for index, pageURL := range session.Tabs {
			for _, target := range realPages {
				if used[target.TargetID] || target.URL != pageURL {
					continue
				}
				used[target.TargetID] = true
				ordered = append(ordered, target)
				if index == session.Active {
					activeTarget = target.TargetID
				}
				break
			}
		}
		for _, target := range realPages {
			if !used[target.TargetID] {
				ordered = append(ordered, target)
			}
		}
		if activeTarget == "" {
			for _, target := range ordered {
				if target.URL == activeURL {
					activeTarget = target.TargetID
					break
				}
			}
		}
		if activeTarget == "" {
			activeTarget = ordered[0].TargetID
		}
		b.beginRestore(ordered, activeTarget)
		return
	}

	created := make([]targetInfo, 0, len(session.Tabs))
	activeTarget := ""
	for index, pageURL := range session.Tabs {
		if !restorableURL(pageURL) {
			continue
		}
		response, createErr := b.cdp.Call("", "Target.createTarget", b.newTargetParams(pageURL))
		if createErr != nil {
			continue
		}
		var target struct {
			TargetID string `json:"targetId"`
		}
		if json.Unmarshal(response, &target) != nil || target.TargetID == "" {
			continue
		}
		created = append(created, targetInfo{TargetID: target.TargetID, Type: "page", URL: pageURL})
		if index == session.Active {
			activeTarget = target.TargetID
		}
	}
	if len(created) == 0 {
		return
	}
	if activeTarget == "" {
		activeTarget = created[0].TargetID
	}
	for _, target := range pages {
		_, _ = b.cdp.Call("", "Target.closeTarget", map[string]any{"targetId": target.TargetID})
	}
	b.beginRestore(created, activeTarget)
}

func (b *Controller) beginRestore(targets []targetInfo, activeTarget string) {
	b.mu.Lock()
	b.restorePending = make(map[string]struct{}, len(targets))
	b.restoreOrder = make(map[string]int, len(targets))
	for index, target := range targets {
		b.restorePending[target.TargetID] = struct{}{}
		b.restoreOrder[target.TargetID] = index + 1
	}
	if b.seq < len(targets) {
		b.seq = len(targets)
	}
	b.restoreActiveTarget = activeTarget
	b.restoreActiveID = 0
	b.mu.Unlock()
}

// finishRestoreTarget returns handled=true for startup targets. activate is
// non-zero only when the complete restored batch is ready to be focused.
func (b *Controller) finishRestoreTarget(targetID string, tabID int) (handled bool, activate int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.restorePending[targetID]; !ok {
		return false, 0
	}
	delete(b.restorePending, targetID)
	if targetID == b.restoreActiveTarget {
		b.restoreActiveID = tabID
	}
	if len(b.restorePending) != 0 {
		return true, 0
	}
	activate = b.restoreActiveID
	if activate == 0 {
		for id := range b.tabs {
			if id > activate {
				activate = id
			}
		}
	}
	b.restorePending = nil
	b.restoreOrder = nil
	b.restoreActiveTarget = ""
	b.restoreActiveID = 0
	return true, activate
}
