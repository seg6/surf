// Package ffmpegbin installs the pinned FFmpeg executable used by Surf.
// Artifacts are stored below SURF_HOME so encoding behavior does not depend
// on a host package manager or an unrelated FFmpeg upgrade.
package ffmpegbin

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

const Version = "2026.07.27"

const (
	staticReleaseBase = "https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1/"
	linuxReleaseBase  = "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-07-27-14-00/"
	maxDownloadBytes  = 160 << 20
	maxBinaryBytes    = 192 << 20
)

type artifact struct {
	name   string
	sha256 string
	member string
}

func platformArtifact(goos, goarch string) (artifact, error) {
	switch goos + "/" + goarch {
	case "linux/amd64":
		const root = "ffmpeg-n7.1.5-10-g2aefd64d48-linux64-gpl-7.1"
		return artifact{
			linuxReleaseBase + root + ".tar.xz",
			"e891a9be20a37d4c82bb1b803f09ba93739a3b523b872c1033f76f1a92e6b46b",
			root + "/bin/ffmpeg",
		}, nil
	case "linux/arm64":
		const root = "ffmpeg-n7.1.5-10-g2aefd64d48-linuxarm64-gpl-7.1"
		return artifact{
			linuxReleaseBase + root + ".tar.xz",
			"7a010ee13ada24442814569564f432ef376eb3c95b568de04a897dd52b89ef6e",
			root + "/bin/ffmpeg",
		}, nil
	case "windows/amd64":
		return artifact{staticReleaseBase + "ffmpeg-win32-x64.gz", "8883a3dffbd0a16cf4ef95206ea05283f78908dbfb118f73c83f4951dcc06d77", ""}, nil
	case "darwin/amd64":
		return artifact{staticReleaseBase + "ffmpeg-darwin-x64.gz", "929b375c1182d956c51f7ac25e0b2b0411fb01f6f407aa15c9758efeb4242106", ""}, nil
	case "darwin/arm64":
		return artifact{staticReleaseBase + "ffmpeg-darwin-arm64.gz", "8923876afa8db5585022d7860ec7e589af192f441c56793971276d450ed3bbfa", ""}, nil
	default:
		return artifact{}, fmt.Errorf("automatic FFmpeg runtime is unavailable for %s/%s; set FFMPEG", goos, goarch)
	}
}

// Ensure returns the pinned executable, downloading and verifying it once.
func Ensure(home string) (string, error) {
	a, err := platformArtifact(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	executable := "ffmpeg"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	root := filepath.Join(home, "runtime", "ffmpeg", Version, runtime.GOOS+"-"+runtime.GOARCH)
	path := filepath.Join(root, executable)
	if executableOK(path) {
		return path, nil
	}
	if data := embeddedExecutable(); len(data) != 0 {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
		temp := path + ".part"
		if err := os.WriteFile(temp, data, 0o755); err != nil {
			return "", err
		}
		if err := os.Rename(temp, path); err != nil {
			return "", err
		}
		return path, nil
	}
	if os.Getenv("SURF_FFMPEG_DOWNLOAD") == "0" {
		return "", fmt.Errorf("FFmpeg runtime is not installed at %s and SURF_FFMPEG_DOWNLOAD=0; set FFMPEG", path)
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(root), ".install-"+Version+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	archive := filepath.Join(tmp, filepath.Base(a.name))
	if err := download(archive, a.name); err != nil {
		return "", fmt.Errorf("download FFmpeg runtime: %w", err)
	}
	if err := verifySHA256(archive, a.sha256); err != nil {
		return "", fmt.Errorf("verify FFmpeg runtime: %w", err)
	}
	unpacked := filepath.Join(tmp, executable)
	if err := unpackExecutable(archive, unpacked, a.member); err != nil {
		return "", fmt.Errorf("unpack FFmpeg runtime: %w", err)
	}
	if err := os.Chmod(unpacked, 0o755); err != nil {
		return "", err
	}
	if executableOK(path) {
		return path, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(unpacked, path); err != nil {
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
	if resp.ContentLength > maxDownloadBytes {
		return fmt.Errorf("FFmpeg archive is unexpectedly large: %d bytes", resp.ContentLength)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(resp.Body, maxDownloadBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if written > maxDownloadBytes {
		return fmt.Errorf("FFmpeg archive exceeds %d bytes", maxDownloadBytes)
	}
	return closeErr
}

func verifySHA256(path, want string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, in); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != want {
		return fmt.Errorf("SHA-256 is %s, want %s", got, want)
	}
	return nil
}

func gunzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	z, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer z.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(z, maxBinaryBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if written > maxBinaryBytes {
		return fmt.Errorf("FFmpeg binary exceeds %d bytes", maxBinaryBytes)
	}
	return closeErr
}

func unpackExecutable(src, dst, member string) error {
	if member == "" {
		return gunzipFile(src, dst)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	uncompressed, err := xz.NewReader(in)
	if err != nil {
		return err
	}
	files := tar.NewReader(uncompressed)
	for {
		header, err := files.Next()
		if err == io.EOF {
			return fmt.Errorf("FFmpeg archive did not contain %s", member)
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		if name == ".." || strings.HasPrefix(name, "../") || filepath.IsAbs(header.Name) {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if name != member {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxBinaryBytes {
			return fmt.Errorf("invalid FFmpeg archive member %q", header.Name)
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(out, io.LimitReader(files, maxBinaryBytes+1))
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if written != header.Size {
			return fmt.Errorf("bad uncompressed size for %q", header.Name)
		}
		return closeErr
	}
}
