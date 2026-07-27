// Package browserbin installs the Chromium runtime that Surf is tested
// against. Keeping the browser beside Surf avoids silently changing capture
// semantics when a distribution upgrades its system Chromium.
package browserbin

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const Version = "151.0.7922.47"

const (
	maxArchiveBytes  = 500 << 20
	maxUnpackedBytes = 2 << 30
)

func platformFor(kind string) (archiveName, executable string, err error) {
	var p string
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		p = "linux64"
	case "windows/amd64":
		p = "win64"
	case "darwin/amd64":
		p = "mac-x64"
	case "darwin/arm64":
		p = "mac-arm64"
	default:
		return "", "", fmt.Errorf("automatic Chromium runtime is unavailable for %s/%s; set CHROME", runtime.GOOS, runtime.GOARCH)
	}
	root := kind + "-" + p
	switch kind {
	case "chrome":
		switch runtime.GOOS {
		case "windows":
			executable = filepath.Join(root, "chrome.exe")
		case "darwin":
			executable = filepath.Join(root, "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
		default:
			executable = filepath.Join(root, "chrome")
		}
	default:
		return "", "", fmt.Errorf("unknown Chromium runtime kind %q", kind)
	}
	return root + ".zip", executable, nil
}

// EnsureChrome returns pinned full Chrome for Testing. Surf launches it with
// --headless=new and continues to capture exclusively through CDP.
func EnsureChrome(home string) (string, error) {
	return ensure(home, "chrome")
}

func ensure(home, kind string) (string, error) {
	archiveName, executable, err := platformFor(kind)
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, "runtime", kind, Version)
	path := filepath.Join(root, executable)
	if executableOK(path) {
		return path, nil
	}
	if os.Getenv("SURF_BROWSER_DOWNLOAD") == "0" {
		return "", fmt.Errorf("Chromium runtime is not installed at %s and SURF_BROWSER_DOWNLOAD=0; set CHROME", path)
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(root), ".install-"+Version+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	// The archive URL uses the platform name as a path component.
	platformName := strings.TrimSuffix(strings.TrimPrefix(archiveName, kind+"-"), ".zip")
	url := "https://storage.googleapis.com/chrome-for-testing-public/" + Version + "/" +
		platformName + "/" + archiveName
	zipPath := filepath.Join(tmp, archiveName)
	if err := download(zipPath, url); err != nil {
		return "", fmt.Errorf("download Chromium runtime: %w", err)
	}
	unpacked := filepath.Join(tmp, "unpacked")
	if err := unzip(zipPath, unpacked); err != nil {
		return "", fmt.Errorf("unpack Chromium runtime: %w", err)
	}
	if !executableOK(filepath.Join(unpacked, executable)) {
		return "", fmt.Errorf("Chromium archive did not contain %s", executable)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(unpacked, executable), 0o755); err != nil {
			return "", err
		}
	}
	// Another process may have completed the same install while we downloaded.
	if executableOK(path) {
		return path, nil
	}
	if err := os.Rename(unpacked, root); err != nil {
		if executableOK(path) {
			return path, nil
		}
		return "", err
	}
	return path, nil
}

func executableOK(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func download(dst, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", url, resp.Status)
	}
	if resp.ContentLength > maxArchiveBytes {
		return fmt.Errorf("Chromium archive is unexpectedly large: %d bytes", resp.ContentLength)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxArchiveBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if written > maxArchiveBytes {
		return fmt.Errorf("Chromium archive exceeds %d bytes", maxArchiveBytes)
	}
	return closeErr
}

func unzip(src, dst string) error {
	z, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer z.Close()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	var unpacked int64
	for _, entry := range z.File {
		clean := filepath.Clean(filepath.FromSlash(entry.Name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe ZIP path %q", entry.Name)
		}
		target := filepath.Join(dst, clean)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in Chromium ZIP: %q", entry.Name)
		}
		unpacked += int64(entry.UncompressedSize64)
		if unpacked > maxUnpackedBytes {
			return fmt.Errorf("Chromium ZIP expands beyond %d bytes", maxUnpackedBytes)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entry.Mode().Perm())
		if err != nil {
			in.Close()
			return err
		}
		written, copyErr := io.Copy(out, io.LimitReader(in, int64(entry.UncompressedSize64)+1))
		inErr, outErr := in.Close(), out.Close()
		if copyErr != nil {
			return copyErr
		}
		if written != int64(entry.UncompressedSize64) {
			return fmt.Errorf("bad uncompressed size for %q", entry.Name)
		}
		if inErr != nil {
			return inErr
		}
		if outErr != nil {
			return outErr
		}
	}
	return nil
}
