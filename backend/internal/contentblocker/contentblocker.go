// Package contentblocker installs Surf's managed browser content blocker.
package contentblocker

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	Version         = "2026.723.1724"
	archiveURL      = "https://github.com/uBlockOrigin/uBOL-home/releases/download/2026.723.1724/uBOLite_2026.723.1724.chromium.zip"
	archiveSHA256   = "363745573fe193c47793dc9233c0d1d08f54d2ed2421f3297b815d2ce13cab79"
	maxArchiveBytes = 32 << 20
)

// Ensure returns the unpacked, verified uBlock Origin Lite extension directory.
func Ensure(home string) (string, error) {
	root := filepath.Join(home, "runtime", "ublock-origin-lite", Version)
	if valid(root) {
		return root, nil
	}
	if os.Getenv("SURF_CONTENT_BLOCKER_DOWNLOAD") == "0" {
		return "", fmt.Errorf("uBlock Origin Lite is not installed at %s and downloads are disabled", root)
	}
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp(parent, ".install-"+Version+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)

	archive := filepath.Join(temp, "ubol.zip")
	if err := download(archive); err != nil {
		return "", err
	}
	unpacked := filepath.Join(temp, "unpacked")
	if err := unpack(archive, unpacked); err != nil {
		return "", err
	}
	if !valid(unpacked) {
		return "", fmt.Errorf("downloaded uBlock Origin Lite has an invalid manifest")
	}
	_ = os.RemoveAll(root)
	if err := os.Rename(unpacked, root); err != nil {
		return "", err
	}
	return root, nil
}

func download(destination string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	response, err := client.Get(archiveURL)
	if err != nil {
		return fmt.Errorf("download uBlock Origin Lite: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download uBlock Origin Lite: HTTP %d", response.StatusCode)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(response.Body, maxArchiveBytes+1))
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > maxArchiveBytes {
		return fmt.Errorf("uBlock Origin Lite archive exceeds %d bytes", maxArchiveBytes)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), archiveSHA256) {
		return fmt.Errorf("uBlock Origin Lite archive checksum mismatch")
	}
	return nil
}

func unpack(source, destination string) error {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return fmt.Errorf("open uBlock Origin Lite archive: %w", err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		name := filepath.Clean(filepath.FromSlash(entry.Name))
		if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe extension archive path %q", entry.Name)
		}
		target := filepath.Join(destination, name)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func valid(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return false
	}
	var manifest struct {
		Name            string `json:"name"`
		Version         string `json:"version"`
		ManifestVersion int    `json:"manifest_version"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return false
	}
	return manifest.ManifestVersion == 3 && manifest.Version == Version && manifest.Name != ""
}
