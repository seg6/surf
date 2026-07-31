package browser

import (
	"encoding/json"
	"testing"

	"surf-backend/internal/cdp"
	"surf-backend/internal/transport"
)

func TestFullscreenBindingTracksOnlyItsPageSession(t *testing.T) {
	tab := &Tab{ID: 1, Session: "active"}
	controller := &Controller{
		tabs: map[int]*Tab{1: tab}, bySession: map[string]*Tab{"active": tab},
		activeID: 1, hub: transport.New(),
	}
	params, _ := json.Marshal(map[string]string{"name": fullscreenBinding, "payload": "1"})
	controller.onFullscreenBinding(cdp.Event{SessionID: "active", Params: params})
	if !tab.Fullscreen {
		t.Fatal("active page fullscreen state was not recorded")
	}

	params, _ = json.Marshal(map[string]string{"name": "pageBinding", "payload": "0"})
	controller.onFullscreenBinding(cdp.Event{SessionID: "active", Params: params})
	if !tab.Fullscreen {
		t.Fatal("unrelated page binding changed fullscreen state")
	}
}
