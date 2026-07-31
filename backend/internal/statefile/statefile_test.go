package statefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuarantinePreservesState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := Quarantine(path, "corrupt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(backup, path+".corrupt-") {
		t.Fatalf("backup=%q", backup)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original remains: %v", err)
	}
	data, err := os.ReadFile(backup)
	if err != nil || string(data) != "broken" {
		t.Fatalf("backup=%q err=%v", data, err)
	}
}
