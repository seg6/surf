//go:build !windows && !linux

package process

// MatchesExecutable is deliberately unavailable where the host cannot verify
// another process's image without platform-specific APIs.
func MatchesExecutable(int, string) bool { return false }
