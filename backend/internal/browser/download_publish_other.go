//go:build !linux && !darwin && !windows

package browser

func publishDownload(source, destination string) error {
	return linkDownload(source, destination)
}
