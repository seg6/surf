//go:build windows

package chromium

// Chromium's Windows singleton is a kernel object. Launch also verifies a
// fresh DevTools endpoint and never attaches to an already-running browser.
func checkProfileOwner(profile string) error { return nil }
