package sink

import (
	"os"
	"strings"
)

// DefaultStation is the name shown on the speaker and in the ICY header when
// --station is not given: "<hostname> streaming audio". Sonos shows it as the
// track title the moment it tunes in.
func DefaultStation() string {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	return stationForHost(host)
}

// stationForHost strips a DNS domain (most hostnames have none) so the name
// reads "homedesktop streaming audio" rather than "homedesktop.lan streaming
// audio".
func stationForHost(host string) string {
	host = strings.TrimSpace(host)
	if i := strings.IndexByte(host, '.'); i > 0 {
		host = host[:i]
	}
	if host == "" {
		return "Sinkrill streaming audio"
	}
	return host + " streaming audio"
}
