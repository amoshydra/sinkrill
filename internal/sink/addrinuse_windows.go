//go:build windows

package sink

import (
	"errors"
	"syscall"
)

// wsaEADDRINUSE is Winsock's "address already in use". Go's syscall package
// defines EADDRINUSE as an invented APPLICATION_ERROR value, not the Winsock
// one, so errors.Is against syscall.EADDRINUSE never matches a real bind
// failure on Windows.
const wsaEADDRINUSE syscall.Errno = 10048

// isAddrInUse reports whether a listen failed because something already holds
// the port.
func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == wsaEADDRINUSE
}
