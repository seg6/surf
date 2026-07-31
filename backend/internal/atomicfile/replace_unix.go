//go:build !windows

package atomicfile

import "os"

// Replace atomically installs an already-written temporary file.
func Replace(temporary, target string) error { return os.Rename(temporary, target) }
