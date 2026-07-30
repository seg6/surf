//go:build !linux

package chromium

func cleanupProfileLocks(string) error { return nil }
