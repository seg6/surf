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
	w, err := Open(path, 320, &mirror)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("runtime: first value=1\n"))
	_, _ = w.Write([]byte("runtime: second value=2\n"))
	_, _ = w.Write([]byte("runtime: third value=3\n"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	older, _ := os.ReadFile(path + ".1")
	current, _ := os.ReadFile(path)
	olderRecords, olderErr := DecodeRecords(older)
	currentRecords, currentErr := DecodeRecords(current)
	if olderErr != nil || currentErr != nil || len(olderRecords) == 0 || len(currentRecords) == 0 {
		t.Fatalf("records older=%q (%v) current=%q (%v)", older, olderErr, current, currentErr)
	}
	if currentRecords[len(currentRecords)-1].Message != "third value=3" {
		t.Fatalf("current=%+v", currentRecords)
	}
	if mirror.String() != "runtime: first value=1\nruntime: second value=2\nruntime: third value=3\n" {
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
	w, err := Open(path, 256, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := bytes.Repeat([]byte("x"), 512)
	if n, err := w.Write(input); err != nil || n != len(input) {
		t.Fatalf("write = %d, %v", n, err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	records, err := DecodeRecords(got)
	if err != nil || len(records) != 1 || records[0].Component != "logging" {
		t.Fatalf("bounded record = %q (%v)", got, err)
	}
}

func TestWriterClearKeepsStructuredStreamWritable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	w, err := Open(path, 1024, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("runtime: before\n"))
	if err := w.Clear(); err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("runtime: after\n"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	records, err := DecodeRecords(data)
	if err != nil || len(records) != 1 || records[0].Message != "after" {
		t.Fatalf("records=%+v err=%v", records, err)
	}
}

func TestExternalClearResetsOpenDesktopWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.log")
	w, err := Open(path, 1024, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Event("info", "desktop", "before", nil); err != nil {
		t.Fatal(err)
	}
	if err := ClearPath(path); err != nil {
		t.Fatal(err)
	}
	if err := w.Event("info", "desktop", "after", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	records, err := DecodeRecords(data)
	if err != nil || len(records) != 1 || records[0].Message != "after" {
		t.Fatalf("records=%+v err=%v", records, err)
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
