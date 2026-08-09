//go:build !linux && !darwin && !windows

package clipboard

func newSystemClipboard() systemClipboard { return unavailableSystem{} }
