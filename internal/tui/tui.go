package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/amoshydra/sinkrill/internal/logx"
	"github.com/amoshydra/sinkrill/internal/sink"
	"github.com/amoshydra/sinkrill/internal/sonos"
	"github.com/amoshydra/sinkrill/internal/virtualsink"
)

// room is the slice of the speaker the panel uses. *sonos.Client is the real
// implementation; tests supply a recording fake so every key can be checked
// without a speaker on the network.
type room interface {
	Volume(context.Context) (int, error)
	Muted(context.Context) (bool, error)
	SetVolume(context.Context, int) error
	SetMute(context.Context, bool) error
	TransportState(context.Context) (string, error)
	Position(context.Context) (sonos.Position, error)
	Play(context.Context) error
	Pause(context.Context) error
	Stop(context.Context) error
	Next(context.Context) error
	Previous(context.Context) error
	SetAVTransportURI(ctx context.Context, uri, metadata string) error
}

// Options is what the panel needs before it can draw anything.
type Options struct {
	Room    string // display name of the room
	Device  string // IP the room answers on, when known
	Port    int    // port of the local instance
	Local   *sink.Status
	Stream  *sink.Options // non-nil: start this stream in this process
	Verbose bool
}

const (
	enterScreen  = "\x1b[?1049h\x1b[?25l" // alternate screen, hide cursor
	leaveScreen  = "\x1b[?25h\x1b[?1049l" // show cursor, restore screen
	homeAndClear = "\x1b[H\x1b[J"
)

// Run draws the panel until the user quits or ctx ends. It owns the terminal:
// raw mode and the alternate screen are both restored on the way out.
//
// When Options.Stream is set the panel also runs the stream. That inverts the
// usual relationship — the panel is normally free to quit at any moment — so
// shutdown waits for the stream to stop the speaker and remove its virtual
// sink before the terminal is handed back.
func Run(ctx context.Context, opts Options) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("the control panel needs an interactive terminal")
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("could not put the terminal in raw mode: %w", err)
	}

	fmt.Fprint(os.Stdout, enterScreen)
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		fmt.Fprint(os.Stdout, leaveScreen)
		_ = term.Restore(fd, old)
	}
	defer restore()

	model := Model{Room: opts.Room, Volume: -1, Attached: opts.Stream == nil}
	if opts.Local != nil {
		model.Local = opts.Local
		model.Station = opts.Local.Station
		model.Stream = opts.Local.URI
	}

	// The stream logs to a writer the panel owns: anything printed straight to
	// stderr would land on top of the alt screen.
	var (
		logs      *logBuffer
		runCancel context.CancelFunc
		runErr    <-chan error
	)
	if opts.Stream != nil {
		logs = &logBuffer{}
		runCtx, cancel := context.WithCancel(ctx)
		runCancel = cancel
		done := make(chan error, 1)
		go func() {
			done <- sink.Run(runCtx, *opts.Stream, logx.NewTo(logs, opts.Verbose))
		}()
		runErr = done
	}

	redraw := func() {
		if logs != nil {
			model.Log = logs.Last()
		}
		draw(model)
	}

	stopStream := func() {
		if runCancel == nil {
			return
		}
		model.Message, model.Failed = "stopping the stream", false
		redraw()
		runCancel()
		select {
		case <-runErr:
		case <-time.After(8 * time.Second):
		}
		runCancel, runErr = nil, nil
	}

	// finish stops the stream when the panel owns it, hands the terminal back,
	// and prints the facts the alt screen would otherwise take with it.
	finish := func() {
		stopStream()
		restore()
		for _, line := range summaryLines(model, opts.Stream != nil) {
			fmt.Fprintln(os.Stdout, line)
		}
	}

	keys := make(chan key, 8)
	go decodeKeys(os.Stdin, func(k key) {
		select {
		case keys <- k:
		case <-ctx.Done():
		}
	})

	var speaker room
	if opts.Device != "" {
		speaker = sonos.NewClient(opts.Device)
	}

	snapshots := make(chan snapshot, 1)
	refresh := make(chan struct{}, 1)
	go poll(ctx, opts.Device, opts.Port, snapshots, refresh)
	select {
	case refresh <- struct{}{}:
	default:
	}

	// A status fetch can miss once while an instance restarts. Three misses in
	// a row means it is gone, and an attached panel must not keep showing the
	// capture numbers it last saw.
	misses := 0

	redraw()
	for {
		select {
		case <-ctx.Done():
			finish()
			return nil

		case err := <-runErr:
			// The stream ended on its own: show why, keep the panel open so
			// the message can be read, and let the user quit.
			runErr, runCancel = nil, nil
			if err != nil {
				model.Message, model.Failed = "stream stopped: "+shortError(err), true
			} else {
				model.Message, model.Failed = "stream stopped", false
			}
			redraw()

		case snap := <-snapshots:
			model.apply(snap)
			if snap.local == nil {
				misses++
			} else {
				misses = 0
			}
			if misses >= 3 && model.Local != nil {
				model.dropLocal()
			}
			if snap.speaker != nil {
				speaker = snap.speaker
			}
			redraw()

		case k := <-keys:
			act := keyAction(k, model)
			if act.kind == actQuit {
				finish()
				return nil
			}
			if act.kind == actNone {
				continue
			}
			actCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			model.Message, model.Failed = runAction(actCtx, speaker, model, act)
			cancel()
			// Pick up the consequence now instead of a second later.
			select {
			case refresh <- struct{}{}:
			default:
			}
			redraw()
		}
	}
}

// logBuffer keeps the most recent log line so it can be shown inside the panel
// instead of being printed over it.
type logBuffer struct {
	mu   sync.Mutex
	last string
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if line := strings.TrimRight(string(p), "\n"); line != "" {
		b.last = line
	}
	return len(p), nil
}

func (b *logBuffer) Last() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.last
}

// snapshot is one poll of the room and the local instance.
type snapshot struct {
	volume      int
	muted       bool
	state       string
	playing     string
	local       *sink.Status
	speaker     room // the client this poll used, once the room's IP is known
	routed      int  // playback streams pointing at the virtual sink
	routedKnown bool
	err         error // the room did not answer
}

// apply folds a poll into the model. A room that answers clears a stale error;
// a room that does not answer leaves the last known values on screen.
func (m *Model) apply(s snapshot) {
	if s.err != nil {
		m.Message, m.Failed = "room unreachable: "+shortError(s.err), true
		if s.local != nil {
			m.updateLocal(s.local)
		}
		return
	}
	if m.Failed {
		m.Message, m.Failed = "", false
	}
	m.Volume, m.Muted, m.State, m.Playing = s.volume, s.muted, s.state, s.playing
	m.RoutedApps, m.RoutedKnown = s.routed, s.routedKnown
	if s.local != nil {
		m.updateLocal(s.local)
	}
}

// dropLocal forgets an instance that has stopped answering. Only the stream
// fields go: the room can still be controlled, so its volume and transport
// state stay useful.
func (m *Model) dropLocal() {
	m.HadStream = true
	m.Local = nil
	m.Stream = ""
	m.Station = ""
	m.Message, m.Failed = "the stream stopped", true
}

func (m *Model) updateLocal(st *sink.Status) {
	m.HadStream = true
	m.Local = st
	m.Stream = st.URI
	m.Station = st.Station
	if m.Room == "" && st.Device != nil {
		m.Room = fmt.Sprintf("%s (%s)", st.Device.Name, st.Device.IP)
	}
}

// shortError drops the SOAP URL that net/http wraps around the actual cause.
// The URL is the same for every call, so it only pushes the useful part off
// the end of a narrow terminal.
func shortError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err.Error()
	}
	return err.Error()
}

// poll refreshes the model once a second, and immediately when an action asks
// for it. The room's IP is only known once the instance advertises, so the
// client is created on the first poll that learns it.
func poll(ctx context.Context, ip string, port int, out chan<- snapshot, refresh <-chan struct{}) {
	var speaker room
	if ip != "" {
		speaker = sonos.NewClient(ip)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var (
		routed      int
		routedKnown bool
		routedSink  string
		routedAt    time.Time
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-refresh:
		}

		snap := collect(ctx, speaker, port)
		if snap.local != nil && snap.local.Device != nil && snap.local.Device.IP != ip {
			ip = snap.local.Device.IP
			speaker = sonos.NewClient(ip)
			snap = collect(ctx, speaker, port)
		}
		// Routing changes when the user picks a different output device, which
		// is a pactl call per check, so refresh it at half the poll rate.
		if snap.local != nil && snap.local.VirtualSink != nil {
			name := snap.local.VirtualSink.Name
			if name != routedSink || time.Since(routedAt) >= 2*time.Second {
				routed, routedKnown = virtualsink.RoutedInputs(name)
				routedSink, routedAt = name, time.Now()
			}
		} else {
			routed, routedKnown, routedSink, routedAt = 0, false, "", time.Time{}
		}
		snap.routed, snap.routedKnown = routed, routedKnown
		snap.speaker = speaker

		select {
		case out <- snap:
		default: // the loop is busy drawing; the next tick will do
		}
	}
}

func collect(ctx context.Context, speaker room, port int) snapshot {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	snap := snapshot{volume: -1}
	if st, ok := sink.LocalStatus(port); ok {
		snap.local = st
	}
	if speaker == nil {
		// No room yet: the stream has not advertised, or it was started with
		// --no-tune.
		return snap
	}

	volume, err := speaker.Volume(cctx)
	if err != nil {
		snap.err = err
		return snap
	}
	snap.volume = volume
	if muted, err := speaker.Muted(cctx); err == nil {
		snap.muted = muted
	}
	if state, err := speaker.TransportState(cctx); err == nil {
		snap.state = state
	}
	if pos, err := speaker.Position(cctx); err == nil {
		snap.playing = pos.URI
	}
	return snap
}

// runAction performs one keystroke's worth of work and returns what to show in
// the footer. A message with failed set is drawn as an error.
func runAction(ctx context.Context, speaker room, m Model, a action) (string, bool) {
	fail := func(what string, err error) (string, bool) {
		return fmt.Sprintf("%s: %v", what, err), true
	}

	if speaker == nil {
		return "no room to control yet", true
	}

	switch a.kind {
	case actVolume:
		level := m.Volume
		if level < 0 {
			current, err := speaker.Volume(ctx)
			if err != nil {
				return fail("could not read the volume", err)
			}
			level = current
		}
		target := clampVolume(level + a.delta)
		if target == level {
			return fmt.Sprintf("volume %d", target), false
		}
		if err := speaker.SetVolume(ctx, target); err != nil {
			return fail("could not set the volume", err)
		}
		return fmt.Sprintf("volume %d", target), false

	case actMute:
		muted := !m.Muted
		if err := speaker.SetMute(ctx, muted); err != nil {
			return fail("could not mute", err)
		}
		if muted {
			return "muted", false
		}
		return "unmuted", false

	case actPlay:
		if err := speaker.Play(ctx); err != nil {
			return fail("could not play", err)
		}
		return "playing", false

	case actPause:
		if err := speaker.Pause(ctx); err != nil {
			return fail("could not pause", err)
		}
		return "paused", false

	case actStop:
		if err := speaker.Stop(ctx); err != nil {
			return fail("could not stop", err)
		}
		return "stopped", false

	case actNext:
		if err := speaker.Next(ctx); err != nil {
			return fail("could not skip", err)
		}
		return "next track", false

	case actPrevious:
		if err := speaker.Previous(ctx); err != nil {
			return fail("could not go back", err)
		}
		return "previous track", false

	case actRetune:
		if m.Stream == "" {
			return "no stream to tune to; is the instance started?", true
		}
		format, station := "wav", m.Station
		if m.Local != nil {
			format, station = m.Local.Format, m.Local.Station
		}
		meta := sonos.StreamMetadata(format, station, m.Stream)
		if err := speaker.SetAVTransportURI(ctx, m.Stream, meta); err != nil {
			return fail("could not retune", err)
		}
		if err := speaker.Play(ctx); err != nil {
			return fail("retuned, but playback did not start", err)
		}
		return "tuned back to this stream", false
	}
	return "", false
}

func clampVolume(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// draw paints the panel in place, in one write so a slow terminal cannot show
// a half-finished frame.
func draw(m Model) {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 44 {
		width = 80
	}

	var buf bytes.Buffer
	buf.WriteString(homeAndClear)
	render(crlfWriter{&buf}, m, width)
	_, _ = os.Stdout.Write(buf.Bytes())
}

// crlfWriter turns "\n" into "\r\n". Raw mode disables output processing, so
// a bare newline would move down without returning to the first column.
type crlfWriter struct{ w *bytes.Buffer }

func (c crlfWriter) Write(p []byte) (int, error) {
	if _, err := c.w.Write(bytes.ReplaceAll(p, []byte("\n"), []byte("\r\n"))); err != nil {
		return 0, err
	}
	return len(p), nil
}
