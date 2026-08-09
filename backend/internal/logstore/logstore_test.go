package logstore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterRotatesAndMirrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	var mirror bytes.Buffer
	w, err := Open(path, 8, &mirror)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("first\n"))
	_, _ = w.Write([]byte("second\n"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path + ".1"); string(got) != "first\n" {
		t.Fatalf("predecessor=%q", got)
	}
	if got, _ := os.ReadFile(path); string(got) != "second\n" {
		t.Fatalf("current=%q", got)
	}
	if mirror.String() != "first\nsecond\n" {
		t.Fatalf("mirror=%q", mirror.String())
	}
}

func TestDeviceSnapshotIsValidatedAndReplaced(t *testing.T) {
	home := t.TempDir()
	id := strings.Repeat("a", 64)
	valid := []byte("{\"ts\":\"now\",\"level\":\"info\",\"component\":\"app\",\"message\":\"ready\",\"fields\":{}}\n")
	if err := WriteDeviceSnapshot(home, id, valid); err != nil {
		t.Fatal(err)
	}
	path, _ := DevicePath(home, id)
	if got, _ := os.ReadFile(path); !bytes.Equal(got, valid) {
		t.Fatalf("snapshot=%q", got)
	}
	if err := WriteDeviceSnapshot(home, id, []byte("not-json\n")); err == nil {
		t.Fatal("invalid snapshot was accepted")
	}
	if err := WriteDeviceSnapshot(home, id, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cleared snapshot remains: %v", err)
	}
}

func TestWriterBoundsSingleOversizedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	w, err := Open(path, 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte("0123456789abcdef")
	if n, err := w.Write(input); err != nil || n != len(input) {
		t.Fatalf("write = %d, %v", n, err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "89abcdef" {
		t.Fatalf("bounded record = %q", got)
	}
}
