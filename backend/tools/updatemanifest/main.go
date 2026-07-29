package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"surf-backend/internal/updater"
)

var surfArchive = regexp.MustCompile(`^surf-.+-(linux|windows|darwin)-(amd64|arm64)\.(tar\.gz|zip)$`)
var surfPackage = regexp.MustCompile(`^surf-.+-(linux|windows|darwin)-(amd64|arm64)(?:-setup)?\.(AppImage|dmg|exe)$`)

var requiredArchives = []string{
	"linux-amd64",
	"linux-arm64",
	"windows-amd64",
	"darwin-amd64",
	"darwin-arm64",
}

var requiredPackages = []string{
	"linux-amd64",
	"windows-amd64",
	"darwin-amd64",
	"darwin-arm64",
}

func main() {
	if len(os.Args) != 6 {
		fmt.Fprintln(os.Stderr, "usage: updatemanifest ASSET_DIR CLIENT_DEB VERSION PROTOCOL OUTPUT")
		os.Exit(2)
	}
	directory, clientPath, version, protocol, output := os.Args[1], os.Args[2], os.Args[3], os.Args[4], os.Args[5]
	base := "https://github.com/seg6/surf/releases/download/v" + version + "/"
	manifest := updater.Manifest{
		Schema: 1, Version: version, Protocol: protocol,
		Assets: map[string]updater.Asset{}, Packages: map[string]updater.Asset{},
	}
	entries, err := os.ReadDir(directory)
	check(err)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if match := surfArchive.FindStringSubmatch(entry.Name()); match != nil {
			manifest.Assets[match[1]+"-"+match[2]] = inspect(filepath.Join(directory, entry.Name()), base)
		}
		if match := surfPackage.FindStringSubmatch(entry.Name()); match != nil {
			manifest.Packages[match[1]+"-"+match[2]] = inspect(filepath.Join(directory, entry.Name()), base)
		}
	}
	client := inspect(clientPath, base)
	manifest.Client = &updater.ClientAsset{Asset: client, Protocol: protocol}
	check(requirePlatforms("Surf archives", manifest.Assets, requiredArchives))
	check(requirePlatforms("Surf desktop packages", manifest.Packages, requiredPackages))
	data, err := json.MarshalIndent(manifest, "", "  ")
	check(err)
	data = append(data, '\n')
	check(os.WriteFile(output, data, 0o644))
}

func requirePlatforms(kind string, assets map[string]updater.Asset, required []string) error {
	var missing []string
	for _, platform := range required {
		if _, ok := assets[platform]; !ok {
			missing = append(missing, platform)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("%s missing required platforms: %s", kind, strings.Join(missing, ", "))
	}
	if len(assets) != len(required) {
		return fmt.Errorf("found %d %s, want exactly %d", len(assets), kind, len(required))
	}
	return nil
}

func inspect(path, base string) updater.Asset {
	data, err := os.ReadFile(path)
	check(err)
	sum := sha256.Sum256(data)
	name := filepath.Base(path)
	return updater.Asset{
		Name: name, URL: base + strings.ReplaceAll(name, " ", "%20"),
		SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)),
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
