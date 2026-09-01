// Package updater discovers and stages official Surf releases from GitHub.
package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const DefaultManifestURL = "https://github.com/seg6/surf/releases/latest/download/update.json"

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type ClientAsset struct {
	Asset
	Compatibility string `json:"protocol"` // JSON name retained for pre-0.16 desktop updaters.
}

type Manifest struct {
	Schema        int              `json:"schema"`
	Version       string           `json:"version"`
	Compatibility string           `json:"protocol"` // Kept on the wire for manifest schema 1.
	Assets        map[string]Asset `json:"assets"`
	Packages      map[string]Asset `json:"packages,omitempty"`
	Client        *ClientAsset     `json:"client,omitempty"`
}

type Release struct {
	Manifest Manifest
	Asset    Asset
	Package  Asset
	Newer    bool
}

type Client struct {
	HTTP        *http.Client
	ManifestURL string
}

func (c Client) Check(ctx context.Context, currentVersion string) (Release, error) {
	url := c.ManifestURL
	if url == "" {
		url = DefaultManifestURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("fetch update manifest: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("fetch update manifest: HTTP %d", response.StatusCode)
	}
	var manifest Manifest
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Release{}, fmt.Errorf("decode update manifest: %w", err)
	}
	if manifest.Schema != 1 || manifest.Version == "" || manifest.Compatibility == "" {
		return Release{}, errors.New("invalid update manifest")
	}
	asset, ok := manifest.Assets[runtime.GOOS+"-"+runtime.GOARCH]
	if !ok || asset.URL == "" || asset.SHA256 == "" || asset.Size <= 0 {
		return Release{}, fmt.Errorf("release has no asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return Release{
		Manifest: manifest, Asset: asset, Package: manifest.Packages[runtime.GOOS+"-"+runtime.GOARCH],
		Newer: CompareVersions(manifest.Version, currentVersion) > 0,
	}, nil
}

// DownloadAsset downloads and verifies a release payload without unpacking it.
func (c Client) DownloadAsset(ctx context.Context, asset Asset, destination string) error {
	if asset.URL == "" || asset.Name == "" || asset.Size <= 0 || asset.SHA256 == "" {
		return errors.New("invalid release asset")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	return c.download(ctx, asset, destination)
}

func (c Client) DownloadExecutable(ctx context.Context, release Release, directory string) (string, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	archivePath := filepath.Join(directory, filepath.Base(release.Asset.Name))
	if err := c.download(ctx, release.Asset, archivePath); err != nil {
		return "", err
	}
	executablePath := filepath.Join(directory, executableName())
	if runtime.GOOS == "windows" {
		if err := extractZipExecutable(archivePath, executablePath); err != nil {
			return "", err
		}
	} else {
		if err := extractTarExecutable(archivePath, executablePath); err != nil {
			return "", err
		}
	}
	if err := os.Chmod(executablePath, 0o755); err != nil {
		return "", err
	}
	return executablePath, nil
}

func (c Client) download(ctx context.Context, asset Asset, destination string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", asset.Name, response.StatusCode)
	}
	temporary := destination + ".part"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, asset.Size+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != asset.Size {
		return fmt.Errorf("download %s: size %d, want %d", asset.Name, written, asset.Size)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.SHA256) {
		return fmt.Errorf("download %s: checksum mismatch", asset.Name)
	}
	return os.Rename(temporary, destination)
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func ReplaceExecutable(staged, target string) error {
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if err := os.Chmod(staged, info.Mode().Perm()); err != nil {
		return err
	}
	backup := target + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	return nil
}

// ReplaceExecutableCopy is used by the Windows helper while staged is itself
// running and therefore cannot be renamed.
func ReplaceExecutableCopy(staged, target string) error {
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	source, err := os.Open(staged)
	if err != nil {
		return err
	}
	defer source.Close()
	replacement := target + ".new"
	file, err := os.OpenFile(replacement, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return ReplaceExecutable(replacement, target)
}

func CompareVersions(left, right string) int {
	a, aOK := parseVersion(left)
	b, bOK := parseVersion(right)
	if !aOK || !bOK {
		return strings.Compare(left, right)
	}
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func parseVersion(value string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	if len(parts) != len(result) {
		return result, false
	}
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return result, false
		}
		result[i] = number
	}
	return result, true
}

func executableName() string {
	if runtime.GOOS == "windows" {
		return "surf.exe"
	}
	return "surf"
}

func extractTarExecutable(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == "surf" {
			return copyExecutable(destination, reader, header.Size)
		}
	}
	return errors.New("Surf executable missing from archive")
}

func extractZipExecutable(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if filepath.Base(file.Name) != "surf.exe" {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		err = copyExecutable(destination, source, int64(file.UncompressedSize64))
		_ = source.Close()
		return err
	}
	return errors.New("Surf executable missing from archive")
}

func copyExecutable(destination string, source io.Reader, size int64) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(source, size))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != size {
		return io.ErrUnexpectedEOF
	}
	return nil
}
