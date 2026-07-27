package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"surf-backend/internal/ffmpegbin"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: fetchffmpeg OUTPUT")
		os.Exit(2)
	}
	if info, err := os.Stat(os.Args[1]); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		return
	}
	home, err := os.MkdirTemp("", "surf-fetch-ffmpeg-")
	check(err)
	defer os.RemoveAll(home)
	source, err := ffmpegbin.Ensure(home)
	check(err)
	input, err := os.Open(source)
	check(err)
	defer input.Close()
	check(os.MkdirAll(filepath.Dir(os.Args[1]), 0o755))
	output, err := os.OpenFile(os.Args[1], os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	check(err)
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	check(copyErr)
	check(closeErr)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
