// Package chromium resolves and manages the Chromium browser used by Surf.
//
// Surf prefers an explicit override, then an installed Chrome/Chromium, and
// finally a managed ungoogled-chromium release stored below SURF_HOME.
package chromium

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	MinimumMajor     = 148
	maxArchiveBytes  = 400 << 20
	githubAPIVersion = "2022-11-28"
)

type installation struct {
	Version    string    `json:"version"`
	Executable string    `json:"executable"`
	CheckedAt  time.Time `json:"checkedAt"`
}

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		URL         string `json:"browser_download_url"`
		Size        int64  `json:"size"`
		Digest      string `json:"digest"`
		ContentType string `json:"content_type"`
		State       string `json:"state"`
	} `json:"assets"`
}

type artifact struct {
	Version string
	Name    string
	URL     string
	SHA256  string
	Size    int64
}

// Resolve prefers unbranded Chromium because current branded
// Google Chrome ignores command-line requests to load unpacked extensions.
func Resolve(home string) (string, string, error) {
	for _, candidate := range systemCandidates() {
		if candidate.branded {
			continue
		}
		path := candidate.path
		if !filepath.IsAbs(path) {
			resolved, err := exec.LookPath(path)
			if err != nil {
				continue
			}
			path = resolved
		}
		if compatible(path) {
			return path, candidate.label, nil
		}
	}
	path, version, err := ensureManaged(home)
	if err != nil {
		return "", "", err
	}
	return path, "managed ungoogled-chromium " + version, nil
}

func compatible(path string) bool {
	if !executableOK(path) {
		return false
	}
	major, err := browserMajor(path)
	return err == nil && major >= MinimumMajor
}

// ensureManaged installs the latest official ungoogled-chromium organization
// release if no usable managed version exists.
func ensureManaged(home string) (string, string, error) {
	root := filepath.Join(home, "runtime", "ungoogled-chromium")
	if current, err := readCurrent(root); err == nil && executableOK(filepath.Join(root, current.Executable)) {
		return filepath.Join(root, current.Executable), current.Version, nil
	}
	return installLatest(root)
}

// UpdateManaged refreshes a previously managed browser at most once per day.
// The newly installed version is selected on the next backend restart.
func UpdateManaged(home string) error {
	root := filepath.Join(home, "runtime", "ungoogled-chromium")
	current, err := readCurrent(root)
	if err != nil {
		_, _, err = installLatest(root)
		return err
	}
	if time.Since(current.CheckedAt) < 24*time.Hour {
		return nil
	}
	a, err := latestArtifact(context.Background())
	if err != nil {
		return err
	}
	if compareVersion(a.Version, current.Version) > 0 {
		_, _, err = install(root, a)
		return err
	}
	current.CheckedAt = time.Now().UTC()
	return writeCurrent(root, current)
}

func installLatest(root string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	a, err := latestArtifact(ctx)
	if err != nil {
		return "", "", err
	}
	return install(root, a)
}

func latestArtifact(ctx context.Context) (artifact, error) {
	repository, pattern, err := platformRelease()
	if err != nil {
		return artifact{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/"+repository+"/releases/latest", nil)
	if err != nil {
		return artifact{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return artifact{}, fmt.Errorf("query ungoogled-chromium release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return artifact{}, fmt.Errorf("query ungoogled-chromium release: HTTP %d", response.StatusCode)
	}
	var value release
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	if err := decoder.Decode(&value); err != nil {
		return artifact{}, err
	}
	for _, asset := range value.Assets {
		if !pattern.MatchString(asset.Name) || asset.State != "uploaded" {
			continue
		}
		digest := strings.TrimPrefix(asset.Digest, "sha256:")
		if len(digest) != sha256.Size*2 || asset.Size <= 0 || asset.Size > maxArchiveBytes {
			return artifact{}, fmt.Errorf("release asset %s has invalid integrity metadata", asset.Name)
		}
		return artifact{
			Version: strings.TrimPrefix(value.TagName, "v"), Name: asset.Name,
			URL: asset.URL, SHA256: digest, Size: asset.Size,
		}, nil
	}
	return artifact{}, fmt.Errorf("ungoogled-chromium release %s has no asset for %s/%s", value.TagName, runtime.GOOS, runtime.GOARCH)
}

func install(root string, a artifact) (string, string, error) {
	versionRoot := filepath.Join(root, a.Version)
	if current, err := locateExecutable(versionRoot); err == nil && compatible(current) {
		rel, _ := filepath.Rel(root, current)
		state := installation{Version: a.Version, Executable: rel, CheckedAt: time.Now().UTC()}
		return current, a.Version, writeCurrent(root, state)
	}
	if os.Getenv("SURF_BROWSER_DOWNLOAD") == "0" {
		return "", "", errors.New("no compatible Chrome/Chromium installation found and managed browser downloads are disabled")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", "", err
	}
	temp, err := os.MkdirTemp(root, ".install-"+safeVersion(a.Version)+"-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(temp)
	archive := filepath.Join(temp, a.Name)
	if err := download(archive, a); err != nil {
		return "", "", err
	}
	unpacked := filepath.Join(temp, "unpacked")
	if err := installArchive(archive, unpacked); err != nil {
		return "", "", fmt.Errorf("unpack ungoogled-chromium: %w", err)
	}
	executable, err := locateExecutable(unpacked)
	if err != nil {
		return "", "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(executable, 0o755)
	}
	if !compatible(executable) {
		return "", "", errors.New("downloaded ungoogled-chromium failed its version check")
	}
	_ = os.RemoveAll(versionRoot)
	if err := os.Rename(unpacked, versionRoot); err != nil {
		return "", "", err
	}
	executable, err = locateExecutable(versionRoot)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, executable)
	if err != nil {
		return "", "", err
	}
	state := installation{Version: a.Version, Executable: rel, CheckedAt: time.Now().UTC()}
	if err := writeCurrent(root, state); err != nil {
		return "", "", err
	}
	pruneOld(root, a.Version)
	return executable, a.Version, nil
}

func download(destination string, a artifact) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", a.Name, response.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, a.Size+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != a.Size {
		return fmt.Errorf("download %s: size %d, want %d", a.Name, written, a.Size)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), a.SHA256) {
		return fmt.Errorf("download %s: checksum mismatch", a.Name)
	}
	return nil
}

func readCurrent(root string) (installation, error) {
	var value installation
	data, err := os.ReadFile(filepath.Join(root, "current.json"))
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	if err != nil || value.Version == "" || value.Executable == "" {
		return installation{}, errors.New("invalid managed browser state")
	}
	return value, nil
}

func writeCurrent(root string, value installation) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp := filepath.Join(root, "current.json.tmp")
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(root, "current.json"))
}

func pruneOld(root, current string) {
	entries, _ := os.ReadDir(root)
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && entry.Name() != current {
			versions = append(versions, entry.Name())
		}
	}
	if len(versions) <= 1 {
		return
	}
	// Keep the greatest previous version as a rollback.
	for keepIndex := 0; keepIndex < len(versions); keepIndex++ {
		if compareVersion(versions[keepIndex], versions[0]) > 0 {
			versions[0], versions[keepIndex] = versions[keepIndex], versions[0]
		}
	}
	for _, version := range versions[1:] {
		_ = os.RemoveAll(filepath.Join(root, version))
	}
}

func compareVersion(left, right string) int {
	parse := func(value string) []int {
		fields := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == '-' })
		result := make([]int, len(fields))
		for i, field := range fields {
			result[i], _ = strconv.Atoi(field)
		}
		return result
	}
	a, b := parse(left), parse(right)
	for i := 0; i < len(a) || i < len(b); i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func safeVersion(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return -1
	}, value)
}

func executableOK(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

type candidate struct {
	path, label string
	branded     bool
}
