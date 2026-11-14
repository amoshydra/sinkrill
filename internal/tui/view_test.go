package tui

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/amoshydra/sinkrill/internal/sink"
)

var ansi = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

// panel renders a model at a width and returns the visible text, line by line.
func panel(t *testing.T, m Model, width int) []string {
	t.Helper()
	var buf bytes.Buffer
	render(&buf, m, width)
	return strings.Split(strings.TrimRight(stripANSI(buf.String()), "\n"), "\n")
}

func joined(lines []string) string { return strings.Join(lines, "\n") }

func localStatus() *sink.Status {
	return &sink.Status{
		Station:     "homedesktop streaming audio",
		Format:      "wav",
		Bitrate:     1536,
		URI:         "http://192.168.0.29:8567/stream.wav",
		Listeners:   1,
		QueuedBytes: 0,
		Capture:     sink.CaptureStatus{Running: true, LastDataMS: 120},
	}
}

func TestRenderShowsTheRoomAndTheStream(t *testing.T) {
	m := Model{
		Room:    "Den",
		Station: "homedesktop streaming audio",
		Volume:  62,
		State:   "PLAYING",
		Stream:  "http://192.168.0.29:8567/stream.wav",
		Playing: "http://192.168.0.29:8567/stream.wav",
		Local:   localStatus(),
	}

	text := joined(panel(t, m, 80))

	for _, want := range []string{
		"sinkrill",
		"homedesktop streaming audio",
		"Den",
		"playing",
		"this stream",
		"wav, 1536 kbps",
		"running, last data 120 ms ago",
		"0 B queued, 0 dropped, 1 listener",
		"62",
		"q quit",
		"ready",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("panel is missing %q:\n%s", want, text)
		}
	}
}

func TestRenderSaysWhenTheRoomMovedOn(t *testing.T) {
	m := Model{
		Room:    "Den",
		Volume:  10,
		State:   "PLAYING",
		Stream:  "http://192.168.0.29:8567/stream.wav",
		Playing: "x-sonosapi-stream:s123?sid=204",
		Local:   localStatus(),
	}
	if text := joined(panel(t, m, 80)); !strings.Contains(text, "another source") {
		t.Errorf("expected the panel to say the room is on another source:\n%s", text)
	}
}

func TestRenderSaysWhenTheStreamIsNotAdvertised(t *testing.T) {
	m := Model{Room: "Den", Volume: 10, Local: &sink.Status{Format: "wav", Bitrate: 1536}}
	if text := joined(panel(t, m, 80)); !strings.Contains(text, "no stream advertised yet") {
		t.Errorf("expected a missing-URI hint:\n%s", text)
	}
}

func TestRenderSurvivesAMissingInstance(t *testing.T) {
	m := Model{Room: "Den", Volume: 10, State: "STOPPED"}
	text := joined(panel(t, m, 80))
	if !strings.Contains(text, "no running instance on this port") {
		t.Errorf("expected a hint about the local instance:\n%s", text)
	}
	if strings.Contains(text, "<nil>") {
		t.Errorf("nil local status leaked into the panel:\n%s", text)
	}
}

func TestRenderShowsMuteAndStaleCapture(t *testing.T) {
	st := localStatus()
	st.Capture = sink.CaptureStatus{Running: true, Stalled: true, LastDataMS: 4200}
	m := Model{Room: "Den", Volume: 0, Muted: true, State: "PAUSED_PLAYBACK", Local: st}

	text := joined(panel(t, m, 80))
	for _, want := range []string{"(muted)", "paused", "stalled, last data 4200 ms ago"} {
		if !strings.Contains(text, want) {
			t.Errorf("panel is missing %q:\n%s", want, text)
		}
	}
}

func TestRenderShowsTheLastActionAndItsError(t *testing.T) {
	ok := Model{Room: "Den", Volume: 40, Message: "volume 42"}
	if text := joined(panel(t, ok, 80)); !strings.Contains(text, "volume 42") {
		t.Errorf("expected the action result:\n%s", text)
	}

	bad := Model{Room: "Den", Volume: 40, Message: "could not mute: timeout", Failed: true}
	if text := joined(panel(t, bad, 80)); !strings.Contains(text, "could not mute") {
		t.Errorf("expected the error:\n%s", text)
	}
}

func TestRenderFitsTheTerminal(t *testing.T) {
	m := Model{
		Room:    strings.Repeat("a very long room name ", 6),
		Station: strings.Repeat("a very long station ", 6),
		Volume:  62,
		State:   "PLAYING",
		Playing: "http://192.168.0.29:8567/stream.wav",
		Stream:  "http://192.168.0.29:8567/stream.wav",
		Local:   localStatus(),
		Message: strings.Repeat("a very long message ", 6),
	}

	for _, width := range []int{44, 60, 80, 120} {
		for _, line := range panel(t, m, width) {
			if got := utf8.RuneCountInString(line); got > width {
				t.Errorf("width %d: line is %d runes: %q", width, got, line)
			}
		}
	}
}

func TestVolumeBar(t *testing.T) {
	tests := []struct {
		volume int
		width  int
		want   string
	}{
		{0, 10, "[          ]"},
		{50, 10, "[=====     ]"},
		{100, 10, "[==========]"},
		{-1, 10, "[          ]"},   // not read yet
		{1000, 10, "[==========]"}, // clamped
		{30, 2, "[=     ]"},        // minimum width
	}
	for _, tc := range tests {
		if got := volumeBar(tc.volume, tc.width); got != tc.want {
			t.Errorf("volumeBar(%d, %d) = %q, want %q", tc.volume, tc.width, got, tc.want)
		}
	}
}

func TestCRLFWriterReturnsTheInputLength(t *testing.T) {
	var buf bytes.Buffer
	n, err := crlfWriter{&buf}.Write([]byte("a\nb"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("n = %d, want 3", n)
	}
	if got := buf.String(); got != "a\r\nb" {
		t.Errorf("wrote %q, want %q", got, "a\r\nb")
	}
}

func TestRenderHintsWhenNothingIsRouted(t *testing.T) {
	st := localStatus()
	st.VirtualSink = &sink.VirtualSinkStatus{Name: "sinkrill", CreatedByUs: true}

	silent := Model{Room: "Den", Volume: 40, Stream: st.URI, Playing: st.URI, Local: st,
		RoutedApps: 0, RoutedKnown: true}
	text := joined(panel(t, silent, 100))
	if !strings.Contains(text, `no app is using "sinkrill" yet`) {
		t.Errorf("expected the routing hint:\n%s", text)
	}

	playing := silent
	playing.RoutedApps = 2
	if text := joined(panel(t, playing, 100)); strings.Contains(text, "no app is using") {
		t.Errorf("hint survived with two apps routed:\n%s", text)
	}

	unknown := silent
	unknown.RoutedKnown = false
	if text := joined(panel(t, unknown, 100)); strings.Contains(text, "no app is using") {
		t.Errorf("hint shown without knowing the routing:\n%s", text)
	}

	// No virtual sink means nothing can be routed to it; the hint would be
	// nonsense in mirror mode.
	plain := silent
	plain.Local = localStatus()
	if text := joined(panel(t, plain, 100)); strings.Contains(text, "no app is using") {
		t.Errorf("hint shown without a virtual sink:\n%s", text)
	}
}

func TestSummaryLines(t *testing.T) {
	st := localStatus()
	st.ListenerBytes = 12_400_000
	st.Listeners = 1
	m := Model{Room: "Den (192.168.0.188)", Station: st.Station, Stream: st.URI, Local: st}

	owned := summaryLines(m, true)
	want := []string{
		"sinkrill stopped on Den (192.168.0.188)",
		"  station  homedesktop streaming audio",
		"  uri      http://192.168.0.29:8567/stream.wav",
		"  captured 12 MB, 1 listener",
	}
	if len(owned) != len(want) {
		t.Fatalf("summary = %q, want %q", owned, want)
	}
	for i := range want {
		if owned[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, owned[i], want[i])
		}
	}

	// Attached panels must not claim to have stopped anything, and the
	// captured total belongs to the instance, not to this panel session.
	attached := summaryLines(m, false)
	if attached[0] != "panel closed; the stream is still running" {
		t.Errorf("attach summary starts with %q", attached[0])
	}
	if strings.Contains(strings.Join(attached, "\n"), "captured") {
		t.Errorf("attach summary reported the instance's lifetime bytes: %q", attached)
	}

	// Attaching to a port with no instance must not claim a stream exists.
	none := summaryLines(Model{Room: "Den"}, false)
	if none[0] != "panel closed; no stream was running" {
		t.Errorf("empty attach summary starts with %q", none[0])
	}

	// A stream that stopped while it was being watched is neither "still
	// running" nor "never started".
	stopped := summaryLines(Model{Room: "Den", HadStream: true}, false)
	if stopped[0] != "panel closed; the stream stopped" {
		t.Errorf("stopped attach summary starts with %q", stopped[0])
	}

	// Nothing captured yet: no misleading "0 B" line.
	empty := summaryLines(Model{Room: "Den", Local: localStatus()}, true)
	if strings.Contains(strings.Join(empty, "\n"), "captured") {
		t.Errorf("summary reported traffic that did not happen: %q", empty)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[int64]string{
		0:             "0 B",
		999:           "999 B",
		1000:          "1.0 kB",
		12_400_000:    "12 MB",
		2_500_000_000: "2.5 GB",
	}
	for in, want := range tests {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderSaysWhenNothingIsRunning(t *testing.T) {
	empty := Model{Room: "Den", Volume: 22, Attached: true}
	if text := joined(panel(t, empty, 100)); !strings.Contains(text, "no stream on this port") {
		t.Errorf("attached panel with no instance did not say so:\n%s", text)
	}

	// Once a stream exists, the footer goes back to normal.
	running := empty
	running.Local = localStatus()
	running.Stream = running.Local.URI
	if text := joined(panel(t, running, 100)); strings.Contains(text, "no stream on this port") {
		t.Errorf("footer still claims nothing is running:\n%s", text)
	}

	// A stream this panel started is briefly without /status at startup; it
	// must not be told that nothing is running.
	owned := Model{Room: "Den", Volume: 22}
	if text := joined(panel(t, owned, 100)); strings.Contains(text, "no stream on this port") {
		t.Errorf("owned stream was treated as an empty attach:\n%s", text)
	}
}
