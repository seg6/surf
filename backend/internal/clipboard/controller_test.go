package clipboard

import (
	"context"
	"os"
	"strings"
	"testing"
)

type memorySystem struct {
	text   string
	writes []string
}

func (s *memorySystem) Name() string { return "test" }
func (s *memorySystem) Read(context.Context) (string, error) {
	return s.text, nil
}
func (s *memorySystem) Write(_ context.Context, text string) error {
	s.text = text
	s.writes = append(s.writes, text)
	return nil
}

func TestControllerPersistsOnlySyncPreference(t *testing.T) {
	home := t.TempDir()
	system := &memorySystem{text: "host value"}
	controller, err := newController(home, system)
	if err != nil {
		t.Fatal(err)
	}
	state, err := controller.SetEnabled(context.Background(), true)
	if err != nil || !state.Enabled || state.Text != "host value" {
		t.Fatalf("enabled state=%+v err=%v", state, err)
	}
	if _, _, err := controller.SetRemote(context.Background(), "device secret"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(controller.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "host value") || strings.Contains(string(data), "device secret") {
		t.Fatalf("clipboard contents persisted: %q", data)
	}
	reloaded, err := newController(home, &memorySystem{})
	if err != nil || !reloaded.State().Enabled || reloaded.State().Known {
		t.Fatalf("reloaded state=%+v err=%v", reloaded.State(), err)
	}
}

func TestControllerSynchronizesBothDirectionsAndClearsMemoryWhenDisabled(t *testing.T) {
	system := &memorySystem{text: "first"}
	controller, err := newController(t.TempDir(), system)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	state, changed, err := controller.SetRemote(context.Background(), "from device")
	if err != nil || !changed || state.Text != "from device" || system.text != "from device" {
		t.Fatalf("remote state=%+v changed=%v system=%q err=%v", state, changed, system.text, err)
	}
	system.text = "from host"
	state, changed, err = controller.Refresh(context.Background())
	if err != nil || !changed || state.Text != "from host" {
		t.Fatalf("host state=%+v changed=%v err=%v", state, changed, err)
	}
	state, err = controller.SetEnabled(context.Background(), false)
	if err != nil || state.Known || state.Text != "" {
		t.Fatalf("disabled state=%+v err=%v", state, err)
	}
}

func TestValidateAllowsClearButBoundsAndRejectsNUL(t *testing.T) {
	if err := Validate(""); err != nil {
		t.Fatal(err)
	}
	if err := Validate("a\x00b"); err == nil {
		t.Fatal("NUL clipboard was accepted")
	}
	if err := Validate(strings.Repeat("x", MaxBytes+1)); err == nil {
		t.Fatal("oversized clipboard was accepted")
	}
}
