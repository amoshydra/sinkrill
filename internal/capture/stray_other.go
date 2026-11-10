//go:build !linux

package capture

// KillStraySinkReaders is a no-op where capture backends are not implemented
// yet; there is nothing to leave behind.
func KillStraySinkReaders(string) int { return 0 }
