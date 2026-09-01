package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"surf-backend/internal/config"
)

func TestClientPackageMetadataMustMatchPayloadAndBackend(t *testing.T) {
	originalVersion, originalCompatibility := config.AppVersion, config.CompatibilityVersion
	defer func() {
		config.AppVersion, config.CompatibilityVersion = originalVersion, originalCompatibility
	}()
	config.AppVersion, config.CompatibilityVersion = "0.16.0", "2"
	data := []byte("!<arch>\nverified native package")
	sum := sha256.Sum256(data)
	metadata, _ := json.Marshal(clientPackageMetadata{
		Version: "0.16.0", PackageVersion: "0.16.0-1", Compatibility: 2,
		SHA256: hex.EncodeToString(sum[:]), Size: len(data),
	})
	bundle, err := clientPackageFromData(data, metadata)
	if err != nil || bundle.Version != "0.16.0" || bundle.Compatibility != 2 {
		t.Fatalf("bundle=%+v err=%v", bundle, err)
	}

	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-1] ^= 1
	if _, err := clientPackageFromData(tampered, metadata); err == nil {
		t.Fatal("tampered embedded package was accepted")
	}
	config.CompatibilityVersion = "3"
	if _, err := clientPackageFromData(data, metadata); err == nil {
		t.Fatal("mismatched embedded compatibility was accepted")
	}
}
