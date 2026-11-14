package tui

import (
	"fmt"
	"strings"

	"github.com/amoshydra/sinkrill/internal/sink"
)

// Model is everything the panel draws. It is a plain value, so both the key
// mapping and the renderer work without a terminal, a speaker, or a context.
type Model struct {
	Room    string       // display name of the room, as resolved
	Station string       // the stream's title
	Volume  int          // -1 until the room answers
	Muted   bool         //
	State   string       // PLAYING, PAUSED_PLAYBACK, STOPPED, ...
	Playing string       // URI the room is on right now
	Stream  string       // URI this instance advertises
	Local   *sink.Status // /status of the local instance, nil when it is not running
	Message string       // result of the last action, or the last error
	Failed  bool         // Message is an error
	Log     string       // most recent stream log line, in stream mode

	// RoutedApps is how many playback streams point at the virtual sink.
	// Zero is the most common reason a fresh setup is silent, so the panel
	// says it out loud instead of leaving people to guess.
	RoutedApps  int
	RoutedKnown bool
}

// stateLabel turns a UPnP transport state into something readable.
func (m Model) stateLabel() string {
	switch m.State {
	case "":
		return "unknown"
	case "PLAYING":
		return "playing"
	case "PAUSED_PLAYBACK":
		return "paused"
	case "STOPPED":
		return "stopped"
	case "TRANSITIONING":
		return "buffering"
	}
	return strings.ToLower(m.State)
}

// playingLabel says whether the room is on this stream, without printing a
// URI nobody can read at a glance.
func (m Model) playingLabel() string {
	switch {
	case m.Stream == "":
		return "no stream advertised yet"
	case m.Playing == "":
		return "unknown"
	case m.Playing == m.Stream:
		return "this stream"
	}
	return "another source"
}

// volumeLine is the bar, the number, and the mute marker.
func (m Model) volumeLine(barWidth int) string {
	level := "--"
	if m.Volume >= 0 {
		level = fmt.Sprintf("%d", m.Volume)
	}
	line := fmt.Sprintf("%s  %s", volumeBar(m.Volume, barWidth), level)
	if m.Muted {
		line += "  (muted)"
	}
	return line
}

func (m Model) streamLine() string {
	if m.Local == nil {
		return "no running instance on this port"
	}
	return fmt.Sprintf("%s, %d kbps", m.Local.Format, m.Local.Bitrate)
}

func (m Model) captureLine() string {
	if m.Local == nil {
		return "-"
	}
	c := m.Local.Capture
	switch {
	case c.LastError != "" && !c.Running:
		return "stopped: " + c.LastError
	case !c.Running:
		return "stopped"
	case c.Stalled:
		return fmt.Sprintf("stalled, last data %d ms ago", c.LastDataMS)
	}
	return fmt.Sprintf("running, last data %d ms ago, %d restarts", c.LastDataMS, c.Restarts)
}

func (m Model) bufferLine() string {
	if m.Local == nil {
		return "-"
	}
	noun := "listeners"
	if m.Local.Listeners == 1 {
		noun = "listener"
	}
	return fmt.Sprintf("%d B queued, %d dropped, %d %s",
		m.Local.QueuedBytes, m.Local.DroppedChunks, m.Local.Listeners, noun)
}

// routingHint explains the one failure the numbers cannot: the stream is
// running and the room is playing it, but no application is sending audio into
// the virtual device, so everything is silence.
func (m Model) routingHint() string {
	if !m.RoutedKnown || m.RoutedApps > 0 {
		return ""
	}
	if m.Local == nil || m.Local.VirtualSink == nil {
		return ""
	}
	return fmt.Sprintf("no app is using %q yet; pick it as your output device",
		m.Local.VirtualSink.Name)
}

// summaryLines is what gets printed after the panel closes, restoring the
// facts a scrolling log would have left on screen: which room, which URI, and
// how much was actually served.
func summaryLines(m Model, owned bool) []string {
	lines := make([]string, 0, 4)
	switch {
	case owned && m.Room != "":
		lines = append(lines, "sinkrill stopped on "+m.Room)
	case owned:
		lines = append(lines, "sinkrill stopped")
	default:
		lines = append(lines, "panel closed; the stream is still running")
	}
	if m.Station != "" {
		lines = append(lines, "  station  "+m.Station)
	}
	if m.Stream != "" {
		lines = append(lines, "  uri      "+m.Stream)
	}
	if m.Local != nil && m.Local.ListenerBytes > 0 {
		noun := "listeners"
		if m.Local.Listeners == 1 {
			noun = "listener"
		}
		lines = append(lines, fmt.Sprintf("  captured %s, %d %s",
			humanBytes(m.Local.ListenerBytes), m.Local.Listeners, noun))
	}
	return lines
}

// humanBytes keeps the served total readable without a units dependency.
func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value, exp := float64(n), 0
	for value >= unit && exp < 3 {
		value /= unit
		exp++
	}
	suffix := []string{"kB", "MB", "GB", "TB"}[exp-1]
	if value < 10 {
		return fmt.Sprintf("%.1f %s", value, suffix)
	}
	return fmt.Sprintf("%.0f %s", value, suffix)
}

// volumeBar draws a fixed-width level bar. A negative volume (device not
// answering yet) draws as empty.
func volumeBar(volume, width int) string {
	if width < 6 {
		width = 6
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	filled := volume * width / 100
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
}
