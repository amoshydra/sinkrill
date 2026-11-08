package sonos

import (
	"strconv"
	"strings"
)

// ParseRelTime converts Sonos' "H:MM:SS" into whole seconds.
func ParseRelTime(value string) int {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 {
		return 0
	}
	hours, err1 := strconv.Atoi(parts[0])
	minutes, err2 := strconv.Atoi(parts[1])
	seconds, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0
	}
	total := hours*3600 + minutes*60 + seconds
	if total < 0 {
		return 0
	}
	return total
}
