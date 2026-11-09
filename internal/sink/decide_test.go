package sink

import (
	"testing"
	"time"

	"github.com/amoshydra/sinkrill/internal/capture"
)

func TestEffectiveBitrate(t *testing.T) {
	tests := []struct {
		format  string
		bitrate int
		want    int
	}{
		// WAV is uncompressed: the rate is fixed and --bitrate is ignored.
		{"wav", 192, 1536},
		{"wav", 320, 1536},
		{"wav", 32, 1536},
		// MP3 passes the requested rate through.
		{"mp3", 192, 192},
		{"mp3", 320, 320},
	}
	for _, tc := range tests {
		if got := effectiveBitrate(tc.format, tc.bitrate); got != tc.want {
			t.Errorf("effectiveBitrate(%q, %d) = %d, want %d", tc.format, tc.bitrate, got, tc.want)
		}
	}
}

func TestCaptureStatusAt(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		snap      capture.Snapshot
		wantMS    int64
		wantStall bool
	}{
		{
			name:   "not running reports nothing",
			snap:   capture.Snapshot{Running: false, LastDataAt: now.Add(-time.Hour)},
			wantMS: 0, wantStall: false,
		},
		{
			name:   "running but no data yet is not stalled",
			snap:   capture.Snapshot{Running: true, LastDataAt: time.Time{}, StallTimeout: 5 * time.Second},
			wantMS: 0, wantStall: false,
		},
		{
			name:   "fresh data",
			snap:   capture.Snapshot{Running: true, LastDataAt: now.Add(-time.Second), StallTimeout: 5 * time.Second},
			wantMS: 1000, wantStall: false,
		},
		{
			name:   "data older than the stall timeout",
			snap:   capture.Snapshot{Running: true, LastDataAt: now.Add(-6 * time.Second), StallTimeout: 5 * time.Second},
			wantMS: 6000, wantStall: true,
		},
		{
			name:   "zero timeout falls back to five seconds",
			snap:   capture.Snapshot{Running: true, LastDataAt: now.Add(-6 * time.Second)},
			wantMS: 6000, wantStall: true,
		},
		{
			name:   "just inside the timeout",
			snap:   capture.Snapshot{Running: true, LastDataAt: now.Add(-4999 * time.Millisecond), StallTimeout: 5 * time.Second},
			wantMS: 4999, wantStall: false,
		},
	}

	for _, tc := range tests {
		got := captureStatusAt(tc.snap, now)
		if got.LastDataMS != tc.wantMS {
			t.Errorf("%s: LastDataMS = %d, want %d", tc.name, got.LastDataMS, tc.wantMS)
		}
		if got.Stalled != tc.wantStall {
			t.Errorf("%s: Stalled = %v, want %v", tc.name, got.Stalled, tc.wantStall)
		}
		if got.Running != tc.snap.Running {
			t.Errorf("%s: Running = %v, want %v", tc.name, got.Running, tc.snap.Running)
		}
	}
}

func TestCaptureStatusAtCarriesDiagnostics(t *testing.T) {
	snap := capture.Snapshot{
		Running:    true,
		Restarts:   3,
		Uptime:     90 * time.Second,
		LastError:  "parec exited: signal: killed",
		LastDataAt: time.Now(),
	}
	got := captureStatusAt(snap, time.Now())

	if got.Restarts != 3 {
		t.Errorf("Restarts = %d, want 3", got.Restarts)
	}
	if got.UptimeMS != 90000 {
		t.Errorf("UptimeMS = %d, want 90000", got.UptimeMS)
	}
	if got.LastError != snap.LastError {
		t.Errorf("LastError = %q, want %q", got.LastError, snap.LastError)
	}
}

func TestShouldRetune(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		sinceLast time.Duration
		want      bool
	}{
		{"stopped immediately after a retune", "STOPPED", 0, false},
		{"stopped inside the cooldown", "STOPPED", 19 * time.Second, false},
		{"stopped exactly at the cooldown", "STOPPED", retuneCooldown, true},
		{"stopped long after", "STOPPED", time.Minute, true},
		{"case insensitive", "stopped", 30 * time.Second, true},
		{"playing is left alone", "PLAYING", 30 * time.Second, false},
		{"paused is left alone", "PAUSED_PLAYBACK", 30 * time.Second, false},
		{"empty state is left alone", "", 30 * time.Second, false},
	}
	for _, tc := range tests {
		if got := shouldRetune(tc.state, tc.sinceLast); got != tc.want {
			t.Errorf("%s: shouldRetune(%q, %s) = %v, want %v", tc.name, tc.state, tc.sinceLast, got, tc.want)
		}
	}
}

func TestValidate(t *testing.T) {
	base := func() Options {
		return Options{
			Format:  "wav",
			Port:    8567,
			Bitrate: 192,
			Volume:  -1,
			Station: "Sinkrill",
			Bind:    "0.0.0.0",
		}
	}

	if err := validate(base()); err != nil {
		t.Fatalf("base options should be valid: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Options)
	}{
		{"unknown format", func(o *Options) { o.Format = "flac" }},
		{"empty format", func(o *Options) { o.Format = "" }},
		{"negative port", func(o *Options) { o.Port = -1 }},
		{"port too large", func(o *Options) { o.Port = 65536 }},
		{"bitrate too low", func(o *Options) { o.Bitrate = 31 }},
		{"bitrate too high", func(o *Options) { o.Bitrate = 321 }},
		{"volume below -1", func(o *Options) { o.Volume = -2 }},
		{"volume above 100", func(o *Options) { o.Volume = 101 }},
		{"negative warmup", func(o *Options) { o.WarmupMS = -1 }},
		{"empty station", func(o *Options) { o.Station = "" }},
		{"empty bind", func(o *Options) { o.Bind = "" }},
	}
	for _, tc := range tests {
		opts := base()
		tc.mutate(&opts)
		if err := validate(opts); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}

	// Boundaries that must be accepted.
	edges := []func(*Options){
		func(o *Options) { o.Port = 0 },
		func(o *Options) { o.Port = 65535 },
		func(o *Options) { o.Bitrate = 32 },
		func(o *Options) { o.Bitrate = 320 },
		func(o *Options) { o.Volume = 0 },
		func(o *Options) { o.Volume = 100 },
		func(o *Options) { o.WarmupMS = 0 },
		func(o *Options) { o.Format = "mp3" },
	}
	for i, mutate := range edges {
		opts := base()
		mutate(&opts)
		if err := validate(opts); err != nil {
			t.Errorf("edge %d (%+v): unexpected error: %v", i, opts, err)
		}
	}
}
