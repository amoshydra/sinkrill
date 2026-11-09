//go:build !windows

package sink

import (
	"errors"
	"syscall"
)

// isAddrInUse reports whether a listen failed because something already holds
// the port. Windows uses a different errno, hence the split.
func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
