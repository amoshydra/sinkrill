package sink

import (
	"strings"
	"time"

	"github.com/amoshydra/sinkrill/internal/capture"
)

// stallFallback is used when a capture snapshot does not carry its own timeout.
const stallFallback = 5 * time.Second

// retuneCooldown stops the keepalive loop from fighting a speaker that keeps
// stopping: Sonos reports STOPPED when a stream drops, and re-issuing
// SetAVTransportURI more often than this makes things worse, not better.
const retuneCooldown = 20 * time.Second

// effectiveBitrate is the rate advertised to Sonos. WAV is always 1536 kbps;
// --bitrate only applies to MP3.
func effectiveBitrate(format string, bitrate int) int {
	if format == "wav" {
		return 1536
	}
	return bitrate
}

// captureStatusAt projects a capture snapshot into the shape served by
// /status. A running capture that has stopped delivering bytes is reported as
// stalled; the watchdog in the capture package restarts it.
func captureStatusAt(snap capture.Snapshot, now time.Time) CaptureStatus {
	status := CaptureStatus{
		Running:   snap.Running,
		Restarts:  snap.Restarts,
		UptimeMS:  snap.Uptime.Milliseconds(),
		LastError: snap.LastError,
	}
	if !snap.Running || snap.LastDataAt.IsZero() {
		return status
	}
	age := now.Sub(snap.LastDataAt)
	status.LastDataMS = age.Milliseconds()
	timeout := snap.StallTimeout
	if timeout <= 0 {
		timeout = stallFallback
	}
	status.Stalled = age > timeout
	return status
}

// shouldRetune reports whether keepalive should re-issue SetAVTransportURI.
func shouldRetune(state string, sinceLast time.Duration) bool {
	if !strings.EqualFold(state, "STOPPED") {
		return false
	}
	return sinceLast >= retuneCooldown
}
