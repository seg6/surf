//go:build linux

package chromium

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ulikunitz/xz"
)

func systemCandidates() []candidate {
	return []candidate{
		{path: "google-chrome-stable", label: "system Google Chrome Stable", branded: true},
		{path: "/opt/google/chrome/chrome", label: "system Google Chrome Stable", branded: true},
		{path: "chromium", label: "system Chromium"},
		{path: "chromium-browser", label: "system Chromium"},
		{path: "/snap/bin/chromium", label: "system Chromium"},
	}
}

func platformRelease() (string, *regexp.Regexp, error) {
	if runtimeKey() != "linux/amd64" {
		return "", nil, fmt.Errorf("managed ungoogled-chromium is unavailable for %s", runtimeKey())
	}
	return "ungoogled-software/ungoogled-chromium-portablelinux",
		regexp.MustCompile(`-x86_64_linux\.tar\.xz$`), nil
}

func installArchive(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	uncompressed, err := xz.NewReader(input)
	if err != nil {
		return err
	}
	reader := tar.NewReader(uncompressed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		target := filepath.Join(destination, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, header.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, header.FileInfo().Mode().Perm())
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(file, io.LimitReader(reader, header.Size+1))
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if written != header.Size {
				return io.ErrUnexpectedEOF
			}
		case tar.TypeSymlink:
			link := filepath.Clean(filepath.FromSlash(header.Linkname))
			if filepath.IsAbs(link) || link == ".." || strings.HasPrefix(link, ".."+string(filepath.Separator)) {
				return fmt.Errorf("unsafe symlink %q", header.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		}
	}
}

func locateExecutable(root string) (string, error) {
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (info.Name() == "chrome" || info.Name() == "chromium") {
			if strings.Contains(filepath.ToSlash(path), "/locales/") {
				return nil
			}
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("ungoogled-chromium executable missing")
	}
	return found, nil
}
