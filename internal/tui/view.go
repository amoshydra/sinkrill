package tui

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	styleDim   = "\x1b[2m"
	styleBold  = "\x1b[1m"
	styleRed   = "\x1b[31m"
	styleWarn  = "\x1b[33m"
	styleReset = "\x1b[0m"
)

// render draws the whole panel. It is a pure function of the model and the
// terminal width, so the layout can be checked in tests.
func render(w io.Writer, m Model, width int) {
	if width < 44 {
		width = 44
	}

	head := " " + styleBold + "sinkrill" + styleReset
	if room := width - utf8.RuneCountInString(" sinkrill") - 2; m.Station != "" && room > 4 {
		head += "  " + clip(m.Station, room)
	}
	fmt.Fprintln(w, head)

	rule := " " + strings.Repeat("-", width-1)
	fmt.Fprintln(w, styleDim+rule+styleReset)

	line := func(label, value string) {
		fmt.Fprintf(w, "  %s%s%s %s\n", styleDim, pad(label, 9), styleReset, clip(value, width-13))
	}

	room := m.Room
	if room == "" {
		room = "not tuned"
	}
	line("Room", room)
	line("State", m.stateLabel())
	line("Playing", m.playingLabel())
	line("Volume", m.volumeLine(barWidth(width)))
	line("Stream", m.streamLine())
	line("Capture", m.captureLine())
	line("Buffer", m.bufferLine())
	if hint := m.routingHint(); hint != "" {
		fmt.Fprintf(w, "  %s%s%s %s\n", styleDim, pad("Hint", 9), styleReset,
			styleWarn+clip(hint, width-13)+styleReset)
	}
	if m.Log != "" {
		fmt.Fprintf(w, "  %s%s%s %s\n", styleDim, pad("Log", 9), styleReset, clip(m.Log, width-13))
	}

	fmt.Fprintln(w, styleDim+rule+styleReset)

	footer := []string{
		"up/down 2   left/right 10   m mute   space play/pause   s stop",
		"r retune    n/b track       q quit",
	}
	for _, f := range footer {
		fmt.Fprintln(w, styleDim+clip("  "+f, width)+styleReset)
	}

	switch {
	case m.Message == "":
		fmt.Fprintln(w, styleDim+"  ready"+styleReset)
	case m.Failed:
		fmt.Fprintln(w, styleRed+clip("  "+m.Message, width)+styleReset)
	default:
		fmt.Fprintln(w, clip("  "+m.Message, width))
	}
}

// barWidth keeps the volume bar from crowding out the numbers on a narrow
// terminal.
func barWidth(width int) int {
	if width < 60 {
		return width - 32
	}
	return 24
}

func pad(s string, width int) string {
	if n := width - utf8.RuneCountInString(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// clip shortens s to at most width runes, marking the cut with ".." so a
// truncated value is not mistaken for the whole one.
func clip(s string, width int) string {
	if width < 0 {
		width = 0
	}
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	if width <= 2 {
		return string([]rune(s)[:width])
	}
	return string([]rune(s)[:width-2]) + ".."
}
