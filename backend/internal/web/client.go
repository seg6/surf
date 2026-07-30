package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"surf-backend/internal/config"
)

type clientPackage struct {
	Version  string
	Protocol string
	SHA256   string
	Data     []byte
}

func embeddedClientPackage() *clientPackage {
	packageData := embeddedPackage()
	if len(packageData) < 8 || !bytes.Equal(packageData[:8], []byte("!<arch>\n")) {
		return nil
	}
	sum := sha256.Sum256(packageData)
	return &clientPackage{
		Version: config.AppVersion, Protocol: config.NativeVersion,
		SHA256: hex.EncodeToString(sum[:]), Data: packageData,
	}
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
