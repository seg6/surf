// Package identity owns Surf's persistent TLS server identity.
package identity

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"surf-backend/internal/atomicfile"
)

const (
	certFile = "server.crt"
	keyFile  = "server.key"
)

// Identity is the certificate Surf presents directly to paired clients.
type Identity struct {
	Certificate tls.Certificate
	DER         []byte
	Fingerprint string
	Directory   string
}

// LoadOrCreate loads a stable identity from SURF_HOME or creates it once.
func LoadOrCreate(home, serverName string) (*Identity, error) {
	if strings.TrimSpace(home) == "" {
		return nil, fmt.Errorf("SURF_HOME is empty")
	}
	dir := filepath.Join(home, "identity")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create identity directory: %w", err)
	}
	certPath, keyPath := filepath.Join(dir, certFile), filepath.Join(dir, keyFile)
	if _, certErr := os.Stat(certPath); certErr == nil {
		if _, keyErr := os.Stat(keyPath); keyErr == nil {
			return load(certPath, keyPath, dir)
		}
	}
	if err := create(certPath, keyPath, serverName); err != nil {
		return nil, err
	}
	return load(certPath, keyPath, dir)
}

func create(certPath, keyPath, serverName string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate server key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return fmt.Errorf("generate certificate serial: %w", err)
	}
	if serverName == "" {
		serverName = "Surf"
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: serverName},
		// iOS 6's certificate parser rejects otherwise-valid certificates whose
		// not-before date predates the OS by many years. Backdate new identities
		// just enough to tolerate modest clock skew instead.
		NotBefore:             time.Now().UTC().Add(-24 * time.Hour),
		NotAfter:              time.Date(2049, 12, 31, 23, 59, 59, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create server certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := writeAtomic(certPath, certPEM, 0o644); err != nil {
		return err
	}
	if err := writeAtomic(keyPath, keyPEM, 0o600); err != nil {
		return err
	}
	return nil
}

func load(certPath, keyPath, dir string) (*Identity, error) {
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load server identity: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return nil, fmt.Errorf("server identity has no certificate")
	}
	if _, err := x509.ParseCertificate(pair.Certificate[0]); err != nil {
		return nil, fmt.Errorf("parse server certificate: %w", err)
	}
	sum := sha256.Sum256(pair.Certificate[0])
	return &Identity{
		Certificate: pair,
		DER:         append([]byte(nil), pair.Certificate[0]...),
		Fingerprint: hex.EncodeToString(sum[:]),
		Directory:   dir,
	}, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	_ = os.Chmod(tmp, mode)
	if err := atomicfile.Replace(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install %s: %w", filepath.Base(path), err)
	}
	return nil
}
