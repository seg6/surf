package browser

import (
	"encoding/json"
	"testing"

	"surf-backend/internal/cdp"
)

func TestLoadingGenerationCannotClearNewerNavigation(t *testing.T) {
	tab := &Tab{ID: 1}
	controller := &Controller{tabs: map[int]*Tab{1: tab}}

	controller.setTabLoading(tab, true)
	first := tab.loadingGeneration
	controller.setTabLoading(tab, true)
	second := tab.loadingGeneration

	if first == second {
		t.Fatal("a newer navigation reused the previous loading generation")
	}
	if controller.finishTabLoadingGeneration(tab, first) || !tab.loading {
		t.Fatal("an old completion cleared the current navigation")
	}
	if controller.finishTabLoadingGeneration(tab, second) || tab.loading {
		t.Fatal("inactive tab loading state was not cleared")
	}
}

func TestClosedTabCannotMutateLoadingState(t *testing.T) {
	tab := &Tab{ID: 1, loading: true, loadingGeneration: 4}
	controller := &Controller{tabs: map[int]*Tab{}}

	controller.setTabLoading(tab, false)
	if !tab.loading || tab.loadingGeneration != 4 {
		t.Fatal("a detached tab accepted a late loading transition")
	}
}

func TestActiveLoadingFollowsSelectedTab(t *testing.T) {
	loading := &Tab{ID: 1, loading: true}
	complete := &Tab{ID: 2}
	controller := &Controller{
		tabs:     map[int]*Tab{1: loading, 2: complete},
		activeID: 1,
	}

	if !controller.activeTabLoading() {
		t.Fatal("loading active tab was reported complete")
	}
	controller.activeID = 2
	if controller.activeTabLoading() {
		t.Fatal("complete replacement tab inherited the previous tab's loading state")
	}
}

func TestActiveLoadingDeadlineRequestsChromeUpdate(t *testing.T) {
	tab := &Tab{ID: 1, loading: true, loadingGeneration: 7}
	controller := &Controller{tabs: map[int]*Tab{1: tab}, activeID: 1}

	if !controller.finishTabLoadingGeneration(tab, 7) {
		t.Fatal("active completion did not request a native loading update")
	}
	if tab.loading {
		t.Fatal("active completion left the tab loading")
	}
}

func TestChildFrameCannotRestartTabLoading(t *testing.T) {
	tab := &Tab{ID: 1, Session: "page", mainFrameID: "main"}
	controller := &Controller{
		tabs:      map[int]*Tab{1: tab},
		bySession: map[string]*Tab{"page": tab, "iframe": tab},
	}

	controller.onEvent(cdp.Event{
		Method:    "Page.frameStartedLoading",
		SessionID: "page",
		Params:    json.RawMessage(`{"frameId":"main"}`),
	})
	if !tab.loading {
		t.Fatal("the main frame did not start the tab-wide loading state")
	}
	controller.setTabLoading(tab, false)

	controller.onEvent(cdp.Event{
		Method:    "Page.frameStartedLoading",
		SessionID: "page",
		Params:    json.RawMessage(`{"frameId":"embedded-pdf"}`),
	})
	if tab.loading {
		t.Fatal("a child frame restarted the completed tab-wide loading state")
	}
}

func TestIframeSessionCannotReplaceMainFrame(t *testing.T) {
	tab := &Tab{ID: 1, Session: "page", mainFrameID: "main"}
	controller := &Controller{
		tabs:      map[int]*Tab{1: tab},
		bySession: map[string]*Tab{"page": tab, "iframe": tab},
	}

	if controller.rememberMainFrame("iframe", "iframe-root") {
		t.Fatal("an iframe session was accepted as the tab's primary session")
	}
	if tab.mainFrameID != "main" {
		t.Fatal("an iframe session replaced the tab's main frame")
	}
}
