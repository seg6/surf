// Command machoverify validates the architecture and minimum macOS version of
// a Mach-O executable without requiring Apple's native developer tools.
package main

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	lcVersionMinMacOSX = 0x24
	lcBuildVersion     = 0x32
	platformMacOS      = 1
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: machoverify FILE ARCH MIN_MACOS")
		os.Exit(2)
	}
	minimum, err := parseVersion(os.Args[3])
	check(err)
	check(verify(os.Args[1], os.Args[2], minimum))
	printfVersion := formatVersion(minimum)
	fmt.Printf("%s: Mach-O %s, macOS %s\n", os.Args[1], os.Args[2], printfVersion)
}

func verify(path, architecture string, expectedMinimum uint32) error {
	file, err := macho.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	wantedCPU, ok := map[string]macho.Cpu{
		"amd64": macho.CpuAmd64,
		"arm64": macho.CpuArm64,
	}[architecture]
	if !ok {
		return fmt.Errorf("unsupported architecture %q", architecture)
	}
	if file.Cpu != wantedCPU {
		return fmt.Errorf("CPU is %s, want %s", file.Cpu, wantedCPU)
	}

	minimum, found, err := minimumMacOS(file.ByteOrder, file.Loads)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("Mach-O has no macOS deployment target")
	}
	if minimum != expectedMinimum {
		return fmt.Errorf("minimum macOS is %s, want %s",
			formatVersion(minimum), formatVersion(expectedMinimum))
	}
	return nil
}

func minimumMacOS(order binary.ByteOrder, loads []macho.Load) (uint32, bool, error) {
	for _, load := range loads {
		raw := load.Raw()
		if len(raw) < 8 {
			continue
		}
		switch order.Uint32(raw[:4]) {
		case lcBuildVersion:
			if len(raw) < 24 {
				return 0, false, fmt.Errorf("truncated LC_BUILD_VERSION")
			}
			if order.Uint32(raw[8:12]) == platformMacOS {
				return order.Uint32(raw[12:16]), true, nil
			}
		case lcVersionMinMacOSX:
			if len(raw) < 16 {
				return 0, false, fmt.Errorf("truncated LC_VERSION_MIN_MACOSX")
			}
			return order.Uint32(raw[8:12]), true, nil
		}
	}
	return 0, false, nil
}

func parseVersion(value string) (uint32, error) {
	parts := strings.Split(value, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid macOS version %q", value)
	}
	values := [3]uint64{}
	for index, part := range parts {
		if part == "" {
			return 0, fmt.Errorf("invalid macOS version %q", value)
		}
		parsed, err := strconv.ParseUint(part, 10, 16)
		if err != nil {
			return 0, fmt.Errorf("invalid macOS version %q", value)
		}
		values[index] = parsed
	}
	if values[1] > 255 || values[2] > 255 {
		return 0, fmt.Errorf("invalid macOS version %q", value)
	}
	return uint32(values[0]<<16 | values[1]<<8 | values[2]), nil
}

func formatVersion(value uint32) string {
	return fmt.Sprintf("%d.%d.%d", value>>16, value>>8&0xff, value&0xff)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
