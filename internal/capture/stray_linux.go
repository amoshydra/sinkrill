//go:build linux

package capture

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// KillStraySinkReaders terminates capture processes left behind when a sinkrill
// process was killed before it could stop its child. A killed parent leaves
// parec running, still attached to the sink's monitor, which is why the device
// can look busy after sinkrill is gone.
//
// It only matches processes this program started: the client name plus the
// exact monitor device.
func KillStraySinkReaders(sinkName string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	device := "--device=" + sinkName + ".monitor"
	self := os.Getpid()
	killed := 0

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == self {
			continue
		}
		data, err := os.ReadFile("/proc/" + entry.Name() + "/cmdline")
		if err != nil {
			continue
		}

		hasClient, hasDevice := false, false
		for _, arg := range strings.Split(string(data), "\x00") {
			switch arg {
			case "--client-name=sinkrill":
				hasClient = true
			case device:
				hasDevice = true
			}
		}
		if !hasClient || !hasDevice {
			continue
		}

		if proc, err := os.FindProcess(pid); err == nil {
			if proc.Signal(syscall.SIGTERM) == nil {
				killed++
			}
		}
	}
	return killed
}
