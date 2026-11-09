//go:build !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package capture

// No capture backend yet on Windows, macOS, or plan9. The rest of the program
// (Sonos discovery, SOAP control, HTTP streaming) is portable, so a port only
// needs a new file here plus a build tag.
//
// A Windows backend would use WASAPI loopback on the default render endpoint;
// a macOS backend needs either CoreAudio process taps (14.2+) or a virtual
// device such as BlackHole.
func platformBackends() []backend { return nil }
