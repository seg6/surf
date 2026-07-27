//go:build windows

package browserbin

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func browserMajor(path string) (int, error) {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil {
		return 0, err
	}
	if size == 0 {
		return 0, fmt.Errorf("browser has no file-version resource")
	}
	data := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&data[0])); err != nil {
		return 0, err
	}
	var info *windows.VS_FIXEDFILEINFO
	infoSize := uint32(unsafe.Sizeof(*info))
	if err := windows.VerQueryValue(
		unsafe.Pointer(&data[0]), `\`, unsafe.Pointer(&info), &infoSize,
	); err != nil {
		return 0, err
	}
	if info == nil || info.Signature != 0xFEEF04BD {
		return 0, fmt.Errorf("browser has an invalid file-version resource")
	}
	return int(info.FileVersionMS >> 16), nil
}
