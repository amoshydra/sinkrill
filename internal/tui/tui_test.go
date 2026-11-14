package tui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/amoshydra/sinkrill/internal/sink"
	"github.com/amoshydra/sinkrill/internal/sonos"
)

// fakeRoom records every call and mutates the state a speaker would.
type fakeRoom struct {
	volume   int
	muted    bool
	state    string
	uri      string
	meta     string
	calls    []string
	failWith error
}

func (f *fakeRoom) record(call string) error {
	f.calls = append(f.calls, call)
	return f.failWith
}

func (f *fakeRoom) Volume(context.Context) (int, error) {
	return f.volume, f.record("Volume")
}

func (f *fakeRoom) Muted(context.Context) (bool, error) {
	return f.muted, f.record("Muted")
}

func (f *fakeRoom) SetVolume(_ context.Context, v int) error {
	f.volume = v
	return f.record("SetVolume")
}

func (f *fakeRoom) SetMute(_ context.Context, muted bool) error {
	f.muted = muted
	return f.record("SetMute")
}

func (f *fakeRoom) TransportState(context.Context) (string, error) {
	return f.state, f.record("TransportState")
}

func (f *fakeRoom) Position(context.Context) (sonos.Position, error) {
	return sonos.Position{URI: f.uri}, f.record("Position")
}

func (f *fakeRoom) Play(context.Context) error {
	f.state = "PLAYING"
	return f.record("Play")
}

func (f *fakeRoom) Pause(context.Context) error {
	f.state = "PAUSED_PLAYBACK"
	return f.record("Pause")
}

func (f *fakeRoom) Stop(context.Context) error {
	f.state = "STOPPED"
	return f.record("Stop")
}

func (f *fakeRoom) Next(context.Context) error {
	return f.record("Next")
}

func (f *fakeRoom) Previous(context.Context) error {
	return f.record("Previous")
}

func (f *fakeRoom) SetAVTransportURI(_ context.Context, uri, metadata string) error {
	f.uri, f.meta = uri, metadata
	return f.record("SetAVTransportURI")
}

func TestRunActionVolume(t *testing.T) {
	tests := []struct {
		name  string
		start int
		want  int
		delta int
		msg   string
	}{
		{"raises", 40, 42, 2, "volume 42"},
		{"lowers", 40, 38, -2, "volume 38"},
		{"clamps at the top", 98, 100, 10, "volume 100"},
		{"clamps at the bottom", 3, 0, -10, "volume 0"},
		{"no change still reports", 100, 100, 2, "volume 100"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			speaker := &fakeRoom{volume: tc.start}
			msg, failed := runAction(context.Background(), speaker, Model{Volume: tc.start},
				action{kind: actVolume, delta: tc.delta})
			if failed {
				t.Fatalf("unexpected failure: %s", msg)
			}
			if msg != tc.msg {
				t.Errorf("message = %q, want %q", msg, tc.msg)
			}
			if speaker.volume != tc.want {
				t.Errorf("volume = %d, want %d", speaker.volume, tc.want)
			}
		})
	}
}

// The panel may still be waiting for its first poll when a key lands, so a
// volume change has to read the current level first.
func TestRunActionVolumeWithoutAKnownLevel(t *testing.T) {
	speaker := &fakeRoom{volume: 31}
	msg, failed := runAction(context.Background(), speaker, Model{Volume: -1},
		action{kind: actVolume, delta: 2})
	if failed {
		t.Fatalf("unexpected failure: %s", msg)
	}
	if speaker.volume != 33 {
		t.Errorf("volume = %d, want 33", speaker.volume)
	}
}

func TestRunActionTransport(t *testing.T) {
	tests := []struct {
		act  action
		want string
		msg  string
	}{
		{action{kind: actMute}, "SetMute", "muted"},
		{action{kind: actPlay}, "Play", "playing"},
		{action{kind: actPause}, "Pause", "paused"},
		{action{kind: actStop}, "Stop", "stopped"},
		{action{kind: actNext}, "Next", "next track"},
		{action{kind: actPrevious}, "Previous", "previous track"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			speaker := &fakeRoom{state: "PLAYING"}
			msg, failed := runAction(context.Background(), speaker, Model{Volume: 20}, tc.act)
			if failed {
				t.Fatalf("unexpected failure: %s", msg)
			}
			if msg != tc.msg {
				t.Errorf("message = %q, want %q", msg, tc.msg)
			}
			if len(speaker.calls) == 0 || speaker.calls[len(speaker.calls)-1] != tc.want {
				t.Errorf("calls = %v, want %s", speaker.calls, tc.want)
			}
		})
	}
}

func TestRunActionMuteTogglesFromModel(t *testing.T) {
	speaker := &fakeRoom{volume: 20, muted: true}
	msg, _ := runAction(context.Background(), speaker, Model{Muted: true}, action{kind: actMute})
	if msg != "unmuted" {
		t.Errorf("message = %q, want unmuted", msg)
	}
	if speaker.muted {
		t.Error("the room is still muted")
	}
}

func TestRunActionRetuneSendsTheStreamMetadata(t *testing.T) {
	speaker := &fakeRoom{}
	stream := "http://192.168.0.29:8567/stream.wav"
	m := Model{
		Station: "homedesktop streaming audio",
		Stream:  stream,
		Local:   &sink.Status{Format: "wav", Station: "homedesktop streaming audio", URI: stream},
	}

	msg, failed := runAction(context.Background(), speaker, m, action{kind: actRetune})
	if failed {
		t.Fatalf("unexpected failure: %s", msg)
	}
	if speaker.uri != stream {
		t.Errorf("URI = %q, want %q", speaker.uri, stream)
	}
	if !strings.Contains(speaker.meta, "audio/wav") {
		t.Errorf("metadata does not look like WAV: %s", speaker.meta)
	}
	if speaker.state != "PLAYING" {
		t.Error("retune did not start playback")
	}
}

func TestRunActionRetuneNeedsAStream(t *testing.T) {
	speaker := &fakeRoom{}
	msg, failed := runAction(context.Background(), speaker, Model{}, action{kind: actRetune})
	if !failed {
		t.Error("retuning without a stream should fail")
	}
	if !strings.Contains(msg, "no stream") {
		t.Errorf("message = %q", msg)
	}
	if len(speaker.calls) != 0 {
		t.Errorf("nothing should have been sent, got %v", speaker.calls)
	}
}

func TestRunActionReportsDeviceFailures(t *testing.T) {
	speaker := &fakeRoom{volume: 20, failWith: errors.New("connection refused")}

	for _, act := range []action{
		{kind: actVolume, delta: 2},
		{kind: actMute},
		{kind: actPlay},
		{kind: actPause},
		{kind: actStop},
		{kind: actNext},
		{kind: actPrevious},
		{kind: actRetune},
	} {
		msg, failed := runAction(context.Background(), speaker,
			Model{Volume: 20, Stream: "http://x/stream.wav"}, act)
		if !failed {
			t.Errorf("action %+v reported success while the room was failing", act)
		}
		if !strings.Contains(msg, "connection refused") {
			t.Errorf("action %+v: message %q does not mention the cause", act, msg)
		}
	}
}

func TestCollectBuildsASnapshot(t *testing.T) {
	stream := "http://192.168.0.29:8567/stream.wav"
	status := sink.Status{
		Station:   "homedesktop streaming audio",
		Format:    "wav",
		Bitrate:   1536,
		URI:       stream,
		Capture:   sink.CaptureStatus{Running: true},
		Listeners: 1,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(status)
	}))
	defer server.Close()

	port := portOf(t, server.URL)
	speaker := &fakeRoom{volume: 45, muted: true, state: "PLAYING", uri: stream}

	snap := collect(context.Background(), speaker, port)
	if snap.err != nil {
		t.Fatalf("collect failed: %v", snap.err)
	}
	if snap.volume != 45 || !snap.muted || snap.state != "PLAYING" || snap.playing != stream {
		t.Errorf("device fields = %+v", snap)
	}
	if snap.local == nil || snap.local.Station != status.Station {
		t.Fatalf("local status was not read: %+v", snap.local)
	}
}

func TestCollectReportsAnUnreachableRoomButStillTracksTheInstance(t *testing.T) {
	status := sink.Status{Station: "homedesktop streaming audio", Format: "wav", Bitrate: 1536}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(status)
	}))
	defer server.Close()

	speaker := &fakeRoom{failWith: errors.New("i/o timeout")}
	snap := collect(context.Background(), speaker, portOf(t, server.URL))

	if snap.err == nil {
		t.Fatal("expected an error from the room")
	}
	if snap.local == nil {
		t.Error("the local instance should still be reported")
	}

	var m Model
	m.apply(snap)
	if !m.Failed || !strings.Contains(m.Message, "room unreachable") {
		t.Errorf("model did not surface the error: %+v", m)
	}
}

func TestApplyKeepsTheLastKnownVolumeOnAnError(t *testing.T) {
	m := Model{Volume: 64, State: "PLAYING"}
	m.apply(snapshot{err: errors.New("timeout"), volume: -1})

	if m.Volume != 64 {
		t.Errorf("volume = %d, want the last known 64", m.Volume)
	}
	if m.State != "PLAYING" {
		t.Errorf("state = %q, want the last known PLAYING", m.State)
	}
}

func TestApplyClearsAStaleError(t *testing.T) {
	m := Model{Volume: 64, Message: "room unreachable: timeout", Failed: true}
	m.apply(snapshot{volume: 65, state: "PLAYING"})

	if m.Failed || m.Message != "" {
		t.Errorf("stale error survived a good poll: %+v", m)
	}
	if m.Volume != 65 {
		t.Errorf("volume = %d, want 65", m.Volume)
	}
}

func TestApplyAdoptsTheLocalInstance(t *testing.T) {
	m := Model{}
	m.apply(snapshot{
		volume: 10,
		local:  &sink.Status{Station: "new station", URI: "http://x/stream.wav", Format: "mp3"},
	})

	if m.Station != "new station" || m.Stream != "http://x/stream.wav" {
		t.Errorf("model did not adopt the instance: %+v", m)
	}
}

func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestShortErrorKeepsTheCause(t *testing.T) {
	wrapped := &url.Error{
		Op:  "Post",
		URL: "http://192.168.0.188:1400/MediaRenderer/RenderingControl/Control",
		Err: errors.New("connection refused"),
	}
	if got := shortError(wrapped); got != "connection refused" {
		t.Errorf("shortError = %q, want the cause", got)
	}
	if got := shortError(errors.New("plain")); got != "plain" {
		t.Errorf("shortError = %q", got)
	}
}

func TestCollectWithoutARoomReadsOnlyTheInstance(t *testing.T) {
	status := sink.Status{Station: "homedesktop streaming audio", Format: "wav", Bitrate: 1536}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(status)
	}))
	defer server.Close()

	// A panel started with --no-tune, or one whose stream has not advertised
	// yet, has no room to ask. That is not an error.
	snap := collect(context.Background(), nil, portOf(t, server.URL))
	if snap.err != nil {
		t.Fatalf("no room should not be an error: %v", snap.err)
	}
	if snap.volume != -1 {
		t.Errorf("volume = %d, want -1 (unknown)", snap.volume)
	}
	if snap.local == nil {
		t.Error("the local instance should still be reported")
	}
}

func TestApplyAdoptsTheRoomOnceItIsKnown(t *testing.T) {
	status := &sink.Status{
		Station: "homedesktop streaming audio",
		URI:     "http://192.168.0.29:8567/stream.wav",
		Device:  &sink.DeviceStatus{Name: "Den", IP: "192.168.0.188"},
	}

	m := Model{}
	m.apply(snapshot{volume: 10, local: status})
	if m.Room != "Den (192.168.0.188)" {
		t.Errorf("room = %q, want Den (192.168.0.188)", m.Room)
	}

	// A room the panel was told about is never replaced by the one in /status.
	named := Model{Room: "Living Room"}
	named.apply(snapshot{volume: 10, local: status})
	if named.Room != "Living Room" {
		t.Errorf("room = %q, want Living Room", named.Room)
	}
}

func TestLogBufferKeepsTheLastLine(t *testing.T) {
	b := &logBuffer{}
	if b.Last() != "" {
		t.Errorf("empty buffer returned %q", b.Last())
	}
	_, _ = b.Write([]byte("01:00:00 INFO  created output device\n"))
	_, _ = b.Write([]byte("01:00:01 INFO  streaming\n"))
	if got := b.Last(); got != "01:00:01 INFO  streaming" {
		t.Errorf("Last() = %q", got)
	}
}

func TestLogBufferIsSafeForConcurrentUse(t *testing.T) {
	b := &logBuffer{}
	done := make(chan struct{})

	go func() {
		for i := 0; i < 100; i++ {
			_, _ = b.Write([]byte("01:00:00 INFO  line\n"))
		}
		close(done)
	}()

	for {
		select {
		case <-done:
			return
		default:
			_ = b.Last()
		}
	}
}

func TestDropLocalKeepsTheRoomUsable(t *testing.T) {
	m := Model{
		Room:    "Den",
		Volume:  22,
		Station: "homedesktop streaming audio",
		Stream:  "http://192.168.0.29:8567/stream.wav",
		Local:   localStatus(),
	}
	m.dropLocal()

	if m.Local != nil || m.Stream != "" || m.Station != "" {
		t.Errorf("stream fields survived: %+v", m)
	}
	if !m.Failed || !strings.Contains(m.Message, "stopped") {
		t.Errorf("message = %q, failed = %v", m.Message, m.Failed)
	}
	if m.Room != "Den" || m.Volume != 22 {
		t.Errorf("room controls were cleared: %+v", m)
	}
	if !m.HadStream {
		t.Error("dropping the stream forgot that one had existed")
	}
}
