// clientbundlemeta validates a native Surf package and emits the metadata that
// is embedded beside it in desktop builds.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type bundleMetadata struct {
	Version        string `json:"version"`
	PackageVersion string `json:"packageVersion"`
	Compatibility  int    `json:"compatibility"`
	SHA256         string `json:"sha256"`
	Size           int    `json:"size"`
}

func main() {
	debPath := flag.String("deb", "", "native client .deb")
	wantVersion := flag.String("version", "", "Surf release version")
	wantCompatibility := flag.Int("compatibility", 0, "compatibility generation")
	output := flag.String("output", "", "metadata output path")
	flag.Parse()
	if *debPath == "" || *wantVersion == "" || *wantCompatibility < 1 || *output == "" {
		fail("usage: clientbundlemeta --deb FILE --version X.Y.Z --compatibility N --output FILE")
	}

	data, err := os.ReadFile(*debPath)
	check(err)
	control, err := debControl(data)
	check(err)
	fields := controlFields(control)
	packageVersion := fields["Version"]
	compatibility, err := strconv.Atoi(fields["X-Surf-Compatibility"])
	check(err)
	if fields["Package"] != "space.seg6.surf" || fields["Architecture"] != "iphoneos-arm" {
		fail("unexpected native package identity %q/%q", fields["Package"], fields["Architecture"])
	}
	if packageVersion != *wantVersion && !strings.HasPrefix(packageVersion, *wantVersion+"-") {
		fail("native package version %q does not match Surf %q", packageVersion, *wantVersion)
	}
	if compatibility != *wantCompatibility {
		fail("native package compatibility %d does not match backend %d", compatibility, *wantCompatibility)
	}

	sum := sha256.Sum256(data)
	metadata := bundleMetadata{
		Version: *wantVersion, PackageVersion: packageVersion,
		Compatibility: compatibility, SHA256: hex.EncodeToString(sum[:]), Size: len(data),
	}
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	check(err)
	encoded = append(encoded, '\n')
	check(os.WriteFile(*output, encoded, 0o644))
}

func debControl(data []byte) ([]byte, error) {
	if len(data) < 8 || !bytes.Equal(data[:8], []byte("!<arch>\n")) {
		return nil, fmt.Errorf("native client is not a Debian archive")
	}
	for offset := 8; offset+60 <= len(data); {
		header := data[offset : offset+60]
		if !bytes.Equal(header[58:60], []byte("`\n")) {
			return nil, fmt.Errorf("invalid Debian archive member header")
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(header[:16])), "/")
		size, err := strconv.Atoi(strings.TrimSpace(string(header[48:58])))
		if err != nil || size < 0 || offset+60+size > len(data) {
			return nil, fmt.Errorf("invalid Debian archive member size")
		}
		member := data[offset+60 : offset+60+size]
		if name == "control.tar.gz" {
			return readControlTar(member)
		}
		offset += 60 + size
		if offset%2 != 0 {
			offset++
		}
	}
	return nil, fmt.Errorf("native client has no control.tar.gz")
}

func readControlTar(data []byte) ([]byte, error) {
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open native package control archive: %w", err)
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read native package control archive: %w", err)
		}
		if strings.TrimPrefix(header.Name, "./") != "control" {
			continue
		}
		control, err := io.ReadAll(io.LimitReader(archive, 64<<10))
		if err != nil {
			return nil, err
		}
		return control, nil
	}
	return nil, fmt.Errorf("native package control file is missing")
}

func controlFields(control []byte) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(string(control), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok {
			fields[name] = strings.TrimSpace(value)
		}
	}
	return fields
}

func check(err error) {
	if err != nil {
		fail("%v", err)
	}
}

func fail(format string, values ...any) {
	fmt.Fprintf(os.Stderr, "client bundle metadata: "+format+"\n", values...)
	os.Exit(1)
}
