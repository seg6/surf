package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"
)

func TestDebControlReadsSurfCompatibilityMetadata(t *testing.T) {
	control := []byte("Package: space.seg6.surf\nVersion: 0.16.0-1\nArchitecture: iphoneos-arm\nX-Surf-Compatibility: 2\n")
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "./control", Mode: 0o644, Size: int64(len(control))}); err != nil {
		t.Fatal(err)
	}
	_, _ = tarWriter.Write(control)
	_ = tarWriter.Close()
	_ = gzipWriter.Close()

	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	writeARMember(&deb, "debian-binary", []byte("2.0\n"))
	writeARMember(&deb, "control.tar.gz", compressed.Bytes())
	got, err := debControl(deb.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	fields := controlFields(got)
	if fields["Version"] != "0.16.0-1" || fields["X-Surf-Compatibility"] != "2" {
		t.Fatalf("control fields=%v", fields)
	}
}

func writeARMember(target *bytes.Buffer, name string, data []byte) {
	header := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n", name+"/", 0, 0, 0, 0o644, len(data))
	target.WriteString(header)
	target.Write(data)
	if len(data)%2 != 0 {
		target.WriteByte('\n')
	}
}
