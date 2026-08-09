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
	"sync"

	"surf-backend/internal/atomicfile"
)

const (
	ServerMaxBytes   = 4 << 20
	DesktopMaxBytes  = 2 << 20
	DeviceMaxBytes   = 2 << 20
	DeviceMaxLine    = 64 << 10
	DefaultReadBytes = 8 << 20
)

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

// Writer is a current file plus one predecessor. Writes are mirrored to the
// supplied destination (normally stderr for a foreground service).
type Writer struct {
	mu     sync.Mutex
	path   string
	max    int64
	mirror io.Writer
	file   *os.File
	size   int64
}

func Open(path string, max int64, mirror io.Writer) (*Writer, error) {
	if max <= 0 {
		return nil, errors.New("log size must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
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
	originalLength := len(data)
	if int64(len(data)) > w.max {
		data = data[len(data)-int(w.max):]
	}
	if w.size > 0 && w.size+int64(len(data)) > w.max {
		if err := w.rotate(); err != nil {
			return 0, err
		}
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(data)
	w.size += int64(n)
	if w.mirror != nil {
		_, _ = w.mirror.Write(data[:n])
	}
	if err == nil && n == len(data) {
		return originalLength, nil
	}
	return n, err
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
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
		var record struct {
			Timestamp string         `json:"ts"`
			Level     string         `json:"level"`
			Component string         `json:"component"`
			Message   string         `json:"message"`
			Fields    map[string]any `json:"fields"`
		}
		if json.Unmarshal(line, &record) != nil || record.Level == "" || record.Component == "" || record.Message == "" || record.Fields == nil {
			return errors.New("native log contains an invalid NDJSON record")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan native log: %w", err)
	}
	return nil
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
	if len(data) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
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

func RemoveDevice(home, deviceID string) error {
	path, err := DevicePath(home, deviceID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
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
