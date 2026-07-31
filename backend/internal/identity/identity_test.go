package identity

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOrCreatePersistsIdentity(t *testing.T) {
	home := t.TempDir()
	first, err := LoadOrCreate(home, "Test Surf")
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreate(home, "Different Name")
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("identity changed: %s != %s", first.Fingerprint, second.Fingerprint)
	}
	certificate, err := x509.ParseCertificate(first.DER)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("certificate key=%T, want RSA", certificate.PublicKey)
	}
	if publicKey.N.BitLen() != 2048 || certificate.SignatureAlgorithm != x509.SHA256WithRSA {
		t.Fatalf("certificate bits=%d signature=%v", publicKey.N.BitLen(), certificate.SignatureAlgorithm)
	}
	sum := sha256.Sum256(first.DER)
	if first.Fingerprint != hex.EncodeToString(sum[:]) {
		t.Fatal("fingerprint is not SHA-256 of the leaf certificate")
	}
	info, err := os.Stat(filepath.Join(home, "identity", keyFile))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("private key mode = %o", info.Mode().Perm())
	}
}

func TestLoadOrCreateNeverSilentlyReplacesInvalidIdentity(t *testing.T) {
	home := t.TempDir()
	identity, err := LoadOrCreate(home, "Test Surf")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(home, "identity", keyFile)
	if err := os.WriteFile(keyPath, []byte("invalid key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(home, "Test Surf"); err == nil {
		t.Fatal("invalid pinned identity was silently replaced")
	}
	certificate, err := os.ReadFile(filepath.Join(home, "identity", certFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(certificate), "BEGIN CERTIFICATE") || identity.Fingerprint == "" {
		t.Fatal("original certificate was not preserved")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || string(key) != "invalid key" {
		t.Fatalf("invalid key changed: %q err=%v", key, err)
	}
}
