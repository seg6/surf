package ffmpegbin

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulikunitz/xz"
)

func TestPlatformArtifactsArePinned(t *testing.T) {
	for _, platform := range [][2]string{
		{"linux", "amd64"}, {"windows", "amd64"},
		{"darwin", "amd64"}, {"darwin", "arm64"},
	} {
		a, err := platformArtifact(platform[0], platform[1])
		if err != nil || a.name == "" || len(a.sha256) != 64 {
			t.Fatalf("%s/%s: artifact=%+v err=%v", platform[0], platform[1], a, err)
		}
	}
	if _, err := platformArtifact("plan9", "amd64"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}

func TestUnpackTarXZMember(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "ffmpeg.tar.xz"), filepath.Join(dir, "ffmpeg")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	z, err := xz.NewWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	tarball := tar.NewWriter(z)
	body := []byte("binary")
	err = tarball.WriteHeader(&tar.Header{Name: "runtime/bin/ffmpeg", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	if err == nil {
		_, err = tarball.Write(body)
	}
	if closeErr := tarball.Close(); err == nil {
		err = closeErr
	}
	if closeErr := z.Close(); err == nil {
		err = closeErr
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := unpackExecutable(src, dst, "runtime/bin/ffmpeg"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != string(body) {
		t.Fatalf("data=%q err=%v", got, err)
	}
}

func TestGunzipFile(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "ffmpeg.gz"), filepath.Join(dir, "ffmpeg")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	z := gzip.NewWriter(f)
	if _, err = z.Write([]byte("binary")); err == nil {
		err = z.Close()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := gunzipFile(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "binary" {
		t.Fatalf("data=%q err=%v", got, err)
	}
}
