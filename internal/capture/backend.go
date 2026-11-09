package capture

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// backend is one way of getting raw audio out of the host's sound server.
//
// Backends are selected at startup, never per-read: the hot path (Read ->
// copy -> OnData) must stay identical regardless of which backend produced the
// process. Adding a backend means adding one file with a build tag and
// returning it from platformBackends; it must not introduce buffering between
// the process's stdout and OnData.
type backend interface {
	// name identifies the backend in diagnostics.
	name() string
	// command returns the executable and argv that write audio to stdout.
	command(opts Options) (string, []string, error)
}

// selectBackend walks the backends available on this platform in preference
// order and returns the first one that can run here.
func selectBackend(opts Options) (string, []string, error) {
	var reasons []string
	for _, b := range platformBackends() {
		path, args, err := b.command(opts)
		if err == nil {
			return path, args, nil
		}
		reasons = append(reasons, err.Error())
	}
	if len(reasons) == 0 {
		return "", nil, fmt.Errorf(
			"no audio capture backend for %s: Sinkrill reads from a PulseAudio or PipeWire server, which is Linux-only for now",
			runtime.GOOS,
		)
	}
	return "", nil, errors.New(strings.Join(reasons, "; "))
}
