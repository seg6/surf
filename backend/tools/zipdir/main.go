// zipdir creates a ZIP archive without requiring a host zip utility.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: zipdir SOURCE_DIRECTORY OUTPUT.zip")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(source, destination string) (returnErr error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); returnErr == nil {
			returnErr = err
		}
	}()
	archive := zip.NewWriter(out)
	defer func() {
		if err := archive.Close(); returnErr == nil {
			returnErr = err
		}
	}()
	parent := filepath.Dir(source)
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		name, err := filepath.Rel(parent, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(name)
		header.Method = zip.Deflate
		entry, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(entry, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
