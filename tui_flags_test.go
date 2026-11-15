package main

import (
	"flag"
	"io"
	"testing"
)

// tuiFlagSet builds the flag surface `sinkrill tui` parses, the same way
// runTUI does.
func tuiFlagSet() (*flag.FlagSet, *startFlags, *bool, *bool) {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	start := &startFlags{}
	addStartFlags(fs, start)
	attach := boolVar(fs, nil, "attach", "a", false, "")
	stream := boolVar(fs, nil, "stream", "", false, "")
	return fs, start, attach, stream
}

func TestTUIWantsStream(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    bool
		wantErr bool
	}{
		// Bare `sinkrill tui` controls an instance that is already running.
		{"bare attaches", nil, false, false},
		{"verbose alone attaches", []string{"-v"}, false, false},
		{"port alone attaches to that instance", []string{"--port", "9000"}, false, false},
		{"explicit attach", []string{"--attach"}, false, false},

		// Anything that names a room or audio starts a stream.
		{"room starts a stream", []string{"--device", "Den"}, true, false},
		{"short room flag", []string{"-d", "Den"}, true, false},
		{"virtual sink starts a stream", []string{"--virtual-sink"}, true, false},
		{"named virtual sink", []string{"--virtual-sink=kitchen"}, true, false},
		{"no-tune starts a stream", []string{"--no-tune"}, true, false},
		{"mirror starts a stream", []string{"--mirror"}, true, false},
		{"format starts a stream", []string{"--format", "mp3"}, true, false},
		{"volume starts a stream", []string{"--volume", "30"}, true, false},
		{"explicit stream", []string{"--stream"}, true, false},

		{"attach plus stream options is a mistake", []string{"--attach", "--mirror"}, false, true},
		{"attach plus explicit stream is a mistake", []string{"--attach", "--stream"}, false, true},

		// With --attach, a room names what to control, not what to start.
		{"attach controls a named room", []string{"--attach", "--device", "Den"}, false, false},
		{"attach controls a short-named room", []string{"--attach", "-d", "Den"}, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, _, attach, stream := tuiFlagSet()
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("parse: %v", err)
			}
			got, err := tuiWantsStream(fs, *attach, *stream)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("tuiWantsStream = %v, want %v", got, tc.want)
			}
		})
	}
}

// The report that started this: `sinkrill tui Den --virtual-sink` silently
// ignored both the room and the flag, because flag parsing stops at the first
// positional argument. The room shorthand has to be rewritten before parsing.
func TestTUIRoomArgumentIsRewritten(t *testing.T) {
	command, args := splitCommand([]string{"tui", "Den", "--virtual-sink"})
	if command != "tui" {
		t.Fatalf("command = %q, want tui", command)
	}

	fs, start, _, _ := tuiFlagSet()
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %q: %v", args, err)
	}
	if start.opts.Device != "Den" {
		t.Errorf("device = %q, want Den", start.opts.Device)
	}
	if !start.virtualSink.set || start.virtualSink.name != "sinkrill" {
		t.Errorf("virtual sink = %+v, want sinkrill", start.virtualSink)
	}

	stream, err := start.stream(fs)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if stream.Device != "Den" || stream.VirtualSink != "sinkrill" || !stream.Tune {
		t.Errorf("stream options = %+v", stream)
	}
}

// The output device is the default because a mirrored stream plays every sound
// twice. Both --mirror and an explicit --source keep the old behaviour.
func TestDefaultAudioSource(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"bare command creates the sink", nil, "sinkrill", false},
		{"room only still creates the sink", []string{"-d", "Den"}, "sinkrill", false},
		{"mirror disables it", []string{"--mirror"}, "", false},
		{"mirror with a room", []string{"--mirror", "-d", "Den"}, "", false},
		{"explicit source keeps mirroring", []string{"--source", "my.monitor"}, "", false},
		{"short source keeps mirroring", []string{"-s", "my.monitor"}, "", false},
		{"named sink", []string{"--virtual-sink=kitchen"}, "kitchen", false},
		{"old false spelling disables it", []string{"--virtual-sink=false"}, "", false},
		{"mirror and a sink conflict", []string{"--mirror", "--virtual-sink"}, "", true},
		{"mirror and a disabled sink agree", []string{"--mirror", "--virtual-sink=false"}, "", false},
		{"sink with a source would ignore it", []string{"--virtual-sink", "--source", "my.monitor"}, "", true},
		{"disabled sink leaves the source alone", []string{"--virtual-sink=false", "--source", "my.monitor"}, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, start, _, _ := tuiFlagSet()
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("parse %q: %v", tc.args, err)
			}
			opts, err := start.stream(fs)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts.VirtualSink != tc.want {
				t.Errorf("VirtualSink = %q, want %q", opts.VirtualSink, tc.want)
			}
		})
	}
}

// Bare `sinkrill` should behave like `sinkrill tui` when a stream is already
// running: attach, rather than start a second stream on a busy port.
func TestStartAttachesOnlyWhenNothingWasAskedFor(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		running bool
		want    bool
	}{
		{"nothing running starts", nil, false, false},
		{"bare command attaches", nil, true, true},
		{"verbose alone attaches", []string{"-v"}, true, true},
		{"port chooses the instance", []string{"--port", "9000"}, true, true},
		{"room asks for a new stream", []string{"-d", "Den"}, true, false},
		{"source asks for a new stream", []string{"--source", "my.monitor"}, true, false},
		{"mirror asks for a new stream", []string{"--mirror"}, true, false},
		{"virtual sink asks for a new stream", []string{"--virtual-sink"}, true, false},
		{"no-tune asks for a new stream", []string{"--no-tune"}, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, _, _, _ := tuiFlagSet()
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("parse %q: %v", tc.args, err)
			}
			if got := startAttaches(fs, tc.running); got != tc.want {
				t.Errorf("startAttaches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStartRoomArgumentIsRewritten(t *testing.T) {
	command, args := splitCommand([]string{"Living Room", "--virtual-sink"})
	if command != "start" {
		t.Fatalf("command = %q, want start", command)
	}

	fs, start, _, _ := tuiFlagSet()
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %q: %v", args, err)
	}
	if start.opts.Device != "Living Room" {
		t.Errorf("device = %q, want Living Room", start.opts.Device)
	}
}

// A bare command with no room keeps its flags: the shorthand must not rewrite
// a leading flag into a room name.
func TestCommandWithoutRoomIsNotRewritten(t *testing.T) {
	for _, argv := range [][]string{
		{"tui", "--virtual-sink"},
		{"--virtual-sink"},
		{"tui"},
	} {
		command, args := splitCommand(argv)
		if len(args) > 0 && args[0] != "--virtual-sink" {
			t.Errorf("splitCommand(%q) = %q, %q; a flag was treated as a room", argv, command, args)
		}
	}
}
