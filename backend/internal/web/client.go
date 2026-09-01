package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"surf-backend/internal/config"
)

type clientPackage struct {
	Version        string
	PackageVersion string
	Compatibility  int
	SHA256         string
	Data           []byte
}

type clientPackageMetadata struct {
	Version        string `json:"version"`
	PackageVersion string `json:"packageVersion"`
	Compatibility  int    `json:"compatibility"`
	SHA256         string `json:"sha256"`
	Size           int    `json:"size"`
}

func (b *clientPackage) updatePayload(required bool) map[string]any {
	return map[string]any{
		"version": b.Version, "packageVersion": b.PackageVersion,
		"compatibilityVersion": b.Compatibility, "required": required,
		"size": len(b.Data), "sha256": b.SHA256,
		"url": APIRoot + "/updates/client",
	}
}

func embeddedClientPackage() (*clientPackage, error) {
	return clientPackageFromData(embeddedPackage(), embeddedPackageMetadata())
}

func clientPackageFromData(packageData, metadataData []byte) (*clientPackage, error) {
	if len(packageData) == 0 && len(metadataData) == 0 {
		return nil, nil
	}
	if len(packageData) < 8 || !bytes.Equal(packageData[:8], []byte("!<arch>\n")) {
		return nil, fmt.Errorf("invalid embedded Debian package")
	}
	var metadata clientPackageMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		return nil, fmt.Errorf("invalid embedded client metadata: %w", err)
	}
	if metadata.Version != config.AppVersion || metadata.Compatibility != config.CompatibilityGeneration() {
		return nil, fmt.Errorf("embedded client %s compatibility %d does not match backend %s compatibility %s",
			metadata.Version, metadata.Compatibility, config.AppVersion, config.CompatibilityVersion)
	}
	if metadata.PackageVersion == "" || metadata.Size != len(packageData) {
		return nil, fmt.Errorf("embedded client metadata does not match package size")
	}
	sum := sha256.Sum256(packageData)
	actualSHA256 := hex.EncodeToString(sum[:])
	if metadata.SHA256 != actualSHA256 {
		return nil, fmt.Errorf("embedded client metadata does not match package checksum")
	}
	return &clientPackage{
		Version: metadata.Version, PackageVersion: metadata.PackageVersion,
		Compatibility: metadata.Compatibility, SHA256: actualSHA256, Data: packageData,
	}, nil
}

func (b *clientPackage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if b == nil || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.debian.binary-package")
	w.Header().Set("Content-Disposition", `attachment; filename="surf-client.deb"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "surf-client.deb", time.Time{}, bytes.NewReader(b.Data))
}
