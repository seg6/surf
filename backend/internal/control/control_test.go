package control

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestDescriptorRoundTripAndOwnedRemoval(t *testing.T) {
	home := t.TempDir()
	descriptor, err := New("https://127.0.0.1:49152", strings.Repeat("a", 64), "test-protocol", 18080)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(home, descriptor); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(home)
	if err != nil || loaded != descriptor {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(Path(home))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("descriptor mode=%v", info.Mode().Perm())
		}
	}
	if err := RemoveOwned(home, "not-the-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(home); err != nil {
		t.Fatalf("wrong owner removed descriptor: %v", err)
	}
	if err := RemoveOwned(home, descriptor.AdminToken); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(home); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("load after removal=%v", err)
	}
}

func TestWriteAtomicallyReplacesDescriptor(t *testing.T) {
	home := t.TempDir()
	first, _ := New("https://127.0.0.1:49152", strings.Repeat("a", 64), "one", 18080)
	second, _ := New("https://127.0.0.1:49153", strings.Repeat("b", 64), "two", 19090)
	if err := Write(home, first); err != nil {
		t.Fatal(err)
	}
	if err := Write(home, second); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(home)
	if err != nil || loaded != second {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
}

func TestLoadDoesNotCreateState(t *testing.T) {
	home := t.TempDir()
	if _, err := Load(home); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("load=%v", err)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("load created %v", entries)
	}
}
