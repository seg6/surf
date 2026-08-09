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

func TestLiveDeviceRecordAppendsToSnapshot(t *testing.T) {
	home := t.TempDir()
	id := strings.Repeat("b", 64)
	first := []byte("{\"ts\":\"one\",\"level\":\"info\",\"component\":\"app\",\"message\":\"first\",\"fields\":{}}\n")
	if err := WriteDeviceSnapshot(home, id, first); err != nil {
		t.Fatal(err)
	}
	second := []byte("{\"ts\":\"two\",\"level\":\"warn\",\"component\":\"video\",\"message\":\"second\",\"fields\":{}}")
	if err := AppendDeviceRecord(home, id, second); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDeviceSnapshot(home, id, DeviceMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte(nil), first...), append(second, '\n')...)
	if !bytes.Equal(got, want) {
		t.Fatalf("snapshot=%q want=%q", got, want)
	}
}

func TestRepairSnapshotPreservesRecordsThatArrivedAfterCapture(t *testing.T) {
	home := t.TempDir()
	id := strings.Repeat("c", 64)
	first := []byte("{\"ts\":\"one\",\"level\":\"info\",\"component\":\"app\",\"message\":\"first\",\"fields\":{}}\n")
	second := []byte("{\"ts\":\"two\",\"level\":\"info\",\"component\":\"app\",\"message\":\"captured\",\"fields\":{}}\n")
	live := []byte("{\"ts\":\"three\",\"level\":\"info\",\"component\":\"app\",\"message\":\"live\",\"fields\":{}}")
	if err := WriteDeviceSnapshot(home, id, first); err != nil {
		t.Fatal(err)
	}
	if err := AppendDeviceRecord(home, id, live); err != nil {
		t.Fatal(err)
	}
	captured := append(append([]byte(nil), first...), second...)
	if err := WriteDeviceSnapshot(home, id, captured); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDeviceSnapshot(home, id, DeviceMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append(captured, live...), '\n')
	if !bytes.Equal(got, want) {
		t.Fatalf("snapshot=%q want=%q", got, want)
	}
}
