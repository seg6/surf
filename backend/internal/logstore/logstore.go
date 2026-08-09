// Package logstore owns Surf's bounded host logs and mirrored native-device
// snapshots. Clipboard contents and credentials must never be written here.
package logstore

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"surf-backend/internal/atomicfile"
)

const (
	ServerMaxBytes   = 4 << 20
	DesktopMaxBytes  = 2 << 20
	DeviceMaxBytes   = 2 << 20
	DeviceMaxLine    = 64 << 10
	DefaultReadBytes = 8 << 20
)

var deviceMu sync.Mutex

// Record is the one durable format used by server, desktop, and native logs.
// A source is intentionally not embedded: the containing file and the UI
// source selector already provide it without repeating it on every line.
type Record struct {
	Timestamp string         `json:"ts"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields"`
}

func validateRecord(record Record) error {
	if strings.TrimSpace(record.Timestamp) == "" || strings.TrimSpace(record.Level) == "" ||
		strings.TrimSpace(record.Component) == "" || strings.TrimSpace(record.Message) == "" || record.Fields == nil {
		return errors.New("log record is missing required fields")
	}
	return nil
}

func decodeRecord(line []byte) (Record, error) {
	var record Record
	if err := json.Unmarshal(line, &record); err != nil {
		return Record{}, err
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// DecodeRecords validates and parses a complete NDJSON snapshot.
func DecodeRecords(data []byte) ([]Record, error) {
	records := make([]Record, 0)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), DeviceMaxLine)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		record, err := decodeRecord(line)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func recordLine(record Record) []byte {
	data, _ := json.Marshal(record)
	return append(data, '\n')
}

func validComponent(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func typedValue(value string) any {
	value = strings.TrimRight(value, ",;)")
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	if number, err := strconv.ParseInt(value, 10, 64); err == nil {
		return number
	}
	if number, err := strconv.ParseFloat(strings.TrimSuffix(value, "ms"), 64); err == nil {
		return number
	}
	return value
}

func recordFromText(text string) Record {
	text = strings.TrimSpace(text)
	component, message := "runtime", text
	if candidate, remainder, ok := strings.Cut(text, ":"); ok && validComponent(strings.TrimSpace(candidate)) {
		component = strings.ToLower(strings.TrimSpace(candidate))
		message = strings.TrimSpace(remainder)
	}
	lower := strings.ToLower(message)
	level := "info"
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "rejected") || strings.Contains(lower, "fatal") {
		level = "error"
	} else if strings.Contains(lower, "warning") || strings.Contains(lower, "timeout") || strings.Contains(lower, "stalled") || strings.Contains(lower, "drop") || strings.Contains(lower, "unavailable") {
		level = "warn"
	}
	fields := map[string]any{}
	for _, token := range strings.Fields(message) {
		key, value, ok := strings.Cut(token, "=")
		if !ok || !validComponent(key) || value == "" {
			continue
		}
		fields[strings.ToLower(key)] = typedValue(value)
	}
	if message == "" {
		message = "Log event"
	}
	return Record{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Level: level,
		Component: component, Message: message, Fields: fields,
	}
}

func normalizeLine(line []byte) []byte {
	line = bytes.TrimSpace(line)
	if record, err := decodeRecord(line); err == nil {
		return recordLine(record)
	}
	return recordLine(recordFromText(string(line)))
}

func Root(home string) string        { return filepath.Join(home, "logs") }
func ServerPath(home string) string  { return filepath.Join(Root(home), "server.log") }
func DesktopPath(home string) string { return filepath.Join(Root(home), "desktop.log") }

func DevicePath(home, deviceID string) (string, error) {
	decoded, err := hex.DecodeString(deviceID)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("invalid device ID")
	}
	return filepath.Join(Root(home), "devices", deviceID+".ndjson"), nil
}

func deviceClearPath(home, deviceID string) (string, error) {
	path, err := DevicePath(home, deviceID)
	if err != nil {
		return "", err
	}
	return path + ".clear", nil
}

// BeginDeviceClear makes the server mirror empty immediately and leaves a
// durable marker until the native device confirms its own local file is empty.
func BeginDeviceClear(home, deviceID string) error {
	path, err := DevicePath(home, deviceID)
	if err != nil {
		return err
	}
	marker := path + ".clear"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	deviceMu.Lock()
	defer deviceMu.Unlock()
	if err := os.WriteFile(marker, []byte("pending\n"), 0o600); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func DeviceClearPending(home, deviceID string) bool {
	marker, err := deviceClearPath(home, deviceID)
	if err != nil {
		return false
	}
	deviceMu.Lock()
	defer deviceMu.Unlock()
	_, err = os.Stat(marker)
	return err == nil
}

// CompleteDeviceClear removes both the pending marker and any record that
// raced with the clear request before the device acknowledgement arrived.
func CompleteDeviceClear(home, deviceID string) error {
	path, err := DevicePath(home, deviceID)
	if err != nil {
		return err
	}
	deviceMu.Lock()
	defer deviceMu.Unlock()
	for _, candidate := range []string{path, path + ".clear"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Writer is a current file plus one predecessor. Writes are mirrored to the
// supplied destination (normally stderr for a foreground service).
type Writer struct {
	mu      sync.Mutex
	path    string
	max     int64
	mirror  io.Writer
	file    *os.File
	size    int64
	pending []byte
}

func Open(path string, max int64, mirror io.Writer) (*Writer, error) {
	if max < 256 {
		return nil, errors.New("log size must be at least 256 bytes")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// Structured logging is an exact format, not a UI parsing heuristic. Old
	// plaintext files are discarded once instead of being mixed into NDJSON.
	for _, candidate := range []string{path + ".1", path} {
		data, err := os.ReadFile(candidate)
		if err == nil && len(bytes.TrimSpace(data)) != 0 {
			if _, decodeErr := DecodeRecords(data); decodeErr != nil {
				_ = os.Remove(candidate)
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	w := &Writer{path: path, max: max, mirror: mirror}
	if info, err := os.Stat(path); err == nil {
		w.size = info.Size()
		if w.size >= max {
			if err := w.rotate(); err != nil {
				return nil, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w.file = file
	return nil
}

func (w *Writer) rotate() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	_ = os.Remove(w.path + ".1")
	if err := os.Rename(w.path, w.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	w.size = 0
	return nil
}

func (w *Writer) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.mirror != nil {
		_, _ = w.mirror.Write(data)
	}
	w.pending = append(w.pending, data...)
	for {
		newline := bytes.IndexByte(w.pending, '\n')
		if newline < 0 {
			break
		}
		line := normalizeLine(w.pending[:newline])
		w.pending = w.pending[newline+1:]
		if err := w.writeLine(line); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

func (w *Writer) writeLine(data []byte) error {
	if info, err := w.file.Stat(); err == nil && info.Size() != w.size {
		// The desktop supervisor and backend are separate processes. Detect a
		// clear performed by the other owner before applying rotation accounting.
		w.size = info.Size()
	}
	if int64(len(data)) > w.max {
		data = recordLine(Record{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Level: "warn", Component: "logging",
			Message: "Oversized log record discarded", Fields: map[string]any{"bytes": len(data)},
		})
	}
	if w.size > 0 && w.size+int64(len(data)) > w.max {
		if err := w.rotate(); err != nil {
			return err
		}
		if err := w.open(); err != nil {
			return err
		}
	}
	n, err := w.file.Write(data)
	w.size += int64(n)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

// Event writes one already-structured record without sharing the line buffer
// used for a child process's stdout/stderr stream.
func (w *Writer) Event(level, component, message string, fields map[string]any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return os.ErrClosed
	}
	if fields == nil {
		fields = map[string]any{}
	}
	record := Record{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Level: level,
		Component: component, Message: message, Fields: fields,
	}
	if err := validateRecord(record); err != nil {
		return err
	}
	return w.writeLine(recordLine(record))
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	if len(bytes.TrimSpace(w.pending)) != 0 {
		if err := w.writeLine(normalizeLine(w.pending)); err != nil {
			return err
		}
	}
	w.pending = nil
	err := w.file.Close()
	w.file = nil
	return err
}

// Clear atomically resets the active file and its predecessor while keeping
// this writer ready for subsequent live records.
func (w *Writer) Clear() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return os.ErrClosed
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	w.pending = nil
	for _, path := range []string{w.path + ".1", w.path} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	w.size = 0
	return w.open()
}

// ClearPath resets a source owned by another process. Writer.Write detects the
// changed file size before its next append so rotation accounting remains safe.
func ClearPath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.Remove(path + ".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, nil, 0o600)
}

func validateDeviceRecord(line []byte) error {
	if len(line) > DeviceMaxLine {
		return errors.New("native log contains an invalid NDJSON record")
	}
	if _, err := decodeRecord(line); err != nil {
		return errors.New("native log contains an invalid NDJSON record")
	}
	return nil
}

func ValidateDeviceSnapshot(data []byte) error {
	if len(data) > DeviceMaxBytes {
		return fmt.Errorf("native log exceeds %d bytes", DeviceMaxBytes)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), DeviceMaxLine)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if err := validateDeviceRecord(line); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan native log: %w", err)
	}
	return nil
}

// mergeDeviceSnapshot keeps records that arrived over the live socket after
// the client captured its HTTP repair snapshot. The newest common record is
// the ordering boundary: the snapshot is authoritative through that record,
// and any host-side suffix after it was written later.
func mergeDeviceSnapshot(snapshot, existing []byte) []byte {
	if len(snapshot) == 0 || len(existing) == 0 {
		return snapshot
	}
	snapshotLines := bytes.FieldsFunc(snapshot, func(r rune) bool { return r == '\n' })
	existingLines := bytes.FieldsFunc(existing, func(r rune) bool { return r == '\n' })
	positions := make(map[string]struct{}, len(snapshotLines))
	for _, line := range snapshotLines {
		positions[string(line)] = struct{}{}
	}
	lastCommon := -1
	for index, line := range existingLines {
		if _, ok := positions[string(line)]; ok {
			lastCommon = index
		}
	}
	if lastCommon < 0 || lastCommon+1 == len(existingLines) {
		return snapshot
	}
	merged := append([]byte(nil), snapshot...)
	if len(merged) > 0 && merged[len(merged)-1] != '\n' {
		merged = append(merged, '\n')
	}
	for _, line := range existingLines[lastCommon+1:] {
		merged = append(merged, line...)
		merged = append(merged, '\n')
	}
	if len(merged) <= DeviceMaxBytes {
		return merged
	}
	return readTailBytes(merged, DeviceMaxBytes)
}

func readTailBytes(data []byte, max int) []byte {
	if len(data) <= max {
		return data
	}
	data = data[len(data)-max:]
	if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
		data = data[newline+1:]
	}
	return data
}

func WriteDeviceSnapshot(home, deviceID string, data []byte) error {
	if err := ValidateDeviceSnapshot(data); err != nil {
		return err
	}
	path, err := DevicePath(home, deviceID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	deviceMu.Lock()
	defer deviceMu.Unlock()
	if len(data) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if existing, readErr := os.ReadFile(path); readErr == nil {
		data = mergeDeviceSnapshot(data, existing)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := atomicfile.Replace(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

// AppendDeviceRecord mirrors one native log record immediately over the live
// control socket while keeping the same bounded snapshot file used at startup.
func AppendDeviceRecord(home, deviceID string, record json.RawMessage) error {
	if err := validateDeviceRecord(record); err != nil {
		return err
	}
	path, err := DevicePath(home, deviceID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	line := append(append([]byte(nil), record...), '\n')
	deviceMu.Lock()
	defer deviceMu.Unlock()
	info, statErr := os.Stat(path)
	if errors.Is(statErr, os.ErrNotExist) || statErr == nil && info.Size()+int64(len(line)) <= DeviceMaxBytes {
		file, openErr := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr != nil {
			return openErr
		}
		_, writeErr := file.Write(line)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	if statErr != nil {
		return statErr
	}
	existing, err := ReadTail(path, DeviceMaxBytes-int64(len(line)))
	if err != nil {
		return err
	}
	data := append(existing, line...)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := atomicfile.Replace(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func RemoveDevice(home, deviceID string) error {
	path, err := DevicePath(home, deviceID)
	if err != nil {
		return err
	}
	deviceMu.Lock()
	defer deviceMu.Unlock()
	for _, candidate := range []string{path, path + ".clear"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func ReadDeviceSnapshot(home, deviceID string, max int64) ([]byte, error) {
	path, err := DevicePath(home, deviceID)
	if err != nil {
		return nil, err
	}
	deviceMu.Lock()
	defer deviceMu.Unlock()
	return ReadTail(path, max)
}

func ReadTail(path string, max int64) ([]byte, error) {
	if max <= 0 {
		max = DefaultReadBytes
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	start := info.Size() - max
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, max))
	if err != nil {
		return nil, err
	}
	if start > 0 {
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		}
	}
	return data, nil
}

func ReadRotated(path string, max int64) ([]byte, error) {
	older, err := ReadTail(path+".1", max)
	if err != nil {
		return nil, err
	}
	remaining := max - int64(len(older))
	if remaining <= 0 {
		return older, nil
	}
	current, err := ReadTail(path, remaining)
	if err != nil {
		return nil, err
	}
	return append(older, current...), nil
}
