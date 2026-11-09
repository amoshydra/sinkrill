package sink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/amoshydra/sinkrill/internal/capture"
	"github.com/amoshydra/sinkrill/internal/logx"
	"github.com/amoshydra/sinkrill/internal/netutil"
	"github.com/amoshydra/sinkrill/internal/sonos"
	"github.com/amoshydra/sinkrill/internal/stream"
	"github.com/amoshydra/sinkrill/internal/virtualsink"
)

type Options struct {
	Device      string
	Source      string
	Port        int
	Bitrate     int
	Format      string // "wav" or "mp3"
	Host        string
	Bind        string
	FFmpeg      string
	Parec       string
	Volume      int // -1 means "leave unchanged"
	Keepalive   bool
	Tune        bool
	Station     string
	VirtualSink string
	WarmupMS    int
}

type Status struct {
	Station       string             `json:"station"`
	Source        string             `json:"source"`
	Format        string             `json:"format"`
	Bitrate       int                `json:"bitrate"`
	URI           string             `json:"uri"`
	Device        *DeviceStatus      `json:"device"`
	VirtualSink   *VirtualSinkStatus `json:"virtualSink"`
	WarmupMS      int                `json:"warmupMs"`
	Capture       CaptureStatus      `json:"capture"`
	Listeners     int                `json:"listeners"`
	ListenerBytes int64              `json:"listenerBytes"`
	QueuedBytes   int64              `json:"queuedBytes"`
	DroppedChunks int64              `json:"droppedChunks"`
}

type DeviceStatus struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type VirtualSinkStatus struct {
	Name        string `json:"name"`
	CreatedByUs bool   `json:"createdByUs"`
}

type CaptureStatus struct {
	Running    bool   `json:"running"`
	Restarts   int    `json:"restarts"`
	UptimeMS   int64  `json:"uptimeMs"`
	LastDataMS int64  `json:"lastDataMs"`
	Stalled    bool   `json:"stalled"`
	LastError  string `json:"lastError,omitempty"`
}

const firstDataTimeout = 8 * time.Second

func Run(ctx context.Context, opts Options, log *logx.Logger) error {
	if err := validate(opts); err != nil {
		return err
	}

	// Check the port before anything expensive. Device discovery takes seconds
	// and --virtual-sink is a side effect, both pointless if another instance
	// already owns the port.
	if err := checkPortFree(opts); err != nil {
		return err
	}

	inst, err := newInstance(ctx, opts, log)
	if err != nil {
		return err
	}
	defer inst.close()

	ready, err := inst.start(ctx)
	if err != nil || !ready {
		return err
	}
	if err := inst.tuneIn(ctx); err != nil {
		return err
	}
	return inst.loop(ctx)
}

// instance is one running stream: the room it tunes, the source it captures,
// the hub that fans audio out, and the HTTP server that serves it.
//
// Run only sequences the phases; each one lives here so it can be read on its
// own and exercised by the end-to-end test.
type instance struct {
	opts   Options
	log    *logx.Logger
	lanIP  string
	kbps   int
	source string

	device  *sonos.Device
	speaker *sonos.Client
	vsink   *virtualsink.Sink

	hub    *stream.Hub
	capt   *capture.Capture
	server *stream.Server

	mu            sync.RWMutex
	uri           string
	tuned         atomic.Bool
	closeOnce     sync.Once
	firstOnce     sync.Once
	serverStarted bool

	firstData chan struct{}
	fatalCh   chan error
	restartCh chan struct{}
}

func newInstance(ctx context.Context, opts Options, log *logx.Logger) (*instance, error) {
	lanIP := opts.Host
	if lanIP == "" {
		lanIP = netutil.LANIP()
	}
	if lanIP == "" {
		return nil, errors.New("could not detect a LAN IPv4 address; pass --host <ip>")
	}

	inst := &instance{
		opts:      opts,
		log:       log,
		lanIP:     lanIP,
		kbps:      effectiveBitrate(opts.Format, opts.Bitrate),
		source:    opts.Source,
		firstData: make(chan struct{}),
		fatalCh:   make(chan error, 1),
		restartCh: make(chan struct{}, 1),
	}

	if opts.Tune {
		resolved, err := sonos.Resolve(ctx, opts.Device, 3*time.Second, log)
		if err != nil {
			return nil, err
		}
		inst.device = &resolved
		inst.speaker = sonos.NewClient(resolved.IP)
	}

	if opts.VirtualSink != "" {
		created, err := virtualsink.Ensure(opts.VirtualSink, log)
		if err != nil {
			return nil, err
		}
		inst.vsink = created
		inst.source = created.MonitorSource
		if created.Created {
			log.Infof("created output device %q (select it in your audio selector)", opts.VirtualSink)
		} else {
			log.Infof("using existing output device %q", opts.VirtualSink)
		}
	} else {
		what := inst.source
		if what == "@DEFAULT_MONITOR@" {
			what = "the default output"
		}
		log.Infof("mirroring %s; no new audio device (use --virtual-sink for a selectable output)", what)
	}

	inst.hub = stream.NewHub(opts.WarmupMS, stream.BytesPerSecond(opts.Format, opts.Bitrate), log)

	captureFormat := capture.FormatMP3
	if opts.Format == "wav" {
		captureFormat = capture.FormatPCM
	}
	inst.capt = capture.New(capture.Options{
		Source:     inst.source,
		Format:     captureFormat,
		Bitrate:    opts.Bitrate,
		FFmpegPath: opts.FFmpeg,
		ParecPath:  opts.Parec,
		Logger:     log,
	})

	inst.server = stream.NewServer(stream.ServerOptions{
		Port:    opts.Port,
		Bind:    opts.Bind,
		Format:  opts.Format,
		Bitrate: opts.Bitrate,
		Station: opts.Station,
		Status:  inst.status,
		Logger:  log,
	}, inst.hub)

	return inst, nil
}

// status is the payload served on /status.
func (i *instance) status() any {
	snap := i.capt.Snapshot()
	st := Status{
		Station:       i.opts.Station,
		Source:        i.source,
		Format:        i.opts.Format,
		Bitrate:       i.kbps,
		URI:           i.getURI(),
		WarmupMS:      i.opts.WarmupMS,
		Listeners:     i.hub.Count(),
		ListenerBytes: i.hub.TotalBytes(),
		QueuedBytes:   i.hub.MaxQueuedBytes(),
		DroppedChunks: i.hub.TotalDrops(),
		Capture:       captureStatusAt(snap, time.Now()),
	}
	if i.device != nil {
		st.Device = &DeviceStatus{Name: i.device.Name, IP: i.device.IP}
	}
	if i.vsink != nil {
		st.VirtualSink = &VirtualSinkStatus{Name: i.vsink.Name, CreatedByUs: i.vsink.Created}
	}
	return st
}

func (i *instance) getURI() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.uri
}

func (i *instance) setURI(value string) {
	i.mu.Lock()
	i.uri = value
	i.mu.Unlock()
}

// start serves HTTP, launches capture, and waits for the first audio byte.
// ready is false when ctx ended first, in which case there is nothing to tune.
func (i *instance) start(ctx context.Context) (ready bool, err error) {
	if err := i.server.Start(); err != nil {
		return false, describeListenError(i.opts, err)
	}
	i.serverStarted = true

	if err := i.capt.Start(ctx, capture.Callbacks{
		OnData: func(chunk []byte) {
			i.firstOnce.Do(func() { close(i.firstData) })
			i.hub.Write(chunk)
		},
		OnExit: func(err error) {
			if err != nil {
				i.log.Warnf("%v", err)
			}
		},
		OnRestart: func() {
			select {
			case i.restartCh <- struct{}{}:
			default:
			}
		},
		OnFatal: func(err error) {
			select {
			case i.fatalCh <- err:
			default:
			}
		},
	}); err != nil {
		return false, err
	}

	select {
	case <-i.firstData:
	case err := <-i.fatalCh:
		return false, fmt.Errorf("capture failed: %w", err)
	case <-time.After(firstDataTimeout):
		return false, errors.New(
			"no audio data arrived from the source; check that the PulseAudio/PipeWire " +
				"source exists (try `pactl list short sources`) or pass --source",
		)
	case <-ctx.Done():
		return false, nil
	}

	i.advertise()
	return true, nil
}

// advertise publishes the URI the speaker should pull, for /status and logs.
func (i *instance) advertise() {
	uri := sonos.StreamURI(i.lanIP, i.server.Port(), i.opts.Format)
	i.setURI(uri)
	i.log.Infof("advertised URI (%s): %s", i.opts.Format, uri)

	file := "stream.mp3"
	if i.opts.Format == "wav" {
		file = "stream.wav"
	}
	i.log.Infof("preview locally: http://%s:%d/%s", i.lanIP, i.server.Port(), file)
}

// metadata is the DIDL-Lite payload describing this stream.
func (i *instance) metadata() string {
	if i.opts.Format == "wav" {
		return sonos.WAVMetadata(i.opts.Station, i.getURI(), "Sinkrill")
	}
	return sonos.RadioMetadata(i.opts.Station, "Sinkrill")
}

// tune points the speaker at this stream. Safe when there is no speaker.
func (i *instance) tune(ctx context.Context) error {
	if i.speaker == nil {
		return nil
	}
	if err := i.speaker.SetAVTransportURI(ctx, i.getURI(), i.metadata()); err != nil {
		return err
	}
	if err := i.speaker.Play(ctx); err != nil {
		i.log.Debugf("play after tune-in failed: %v", err)
	}
	i.tuned.Store(true)
	return nil
}

// tuneIn applies the startup volume and tunes, or explains --no-tune.
func (i *instance) tuneIn(ctx context.Context) error {
	if i.speaker == nil || i.device == nil {
		i.log.Infof("running without tuning Sonos (--no-tune); point a speaker at the URI above")
		return nil
	}
	if i.opts.Volume >= 0 {
		if err := i.speaker.SetVolume(ctx, i.opts.Volume); err != nil {
			i.log.Warnf("could not set volume: %v", err)
		} else {
			i.log.Infof("set volume on %s to %d", i.device.Name, i.opts.Volume)
		}
	}
	if err := i.tune(ctx); err != nil {
		return err
	}
	i.log.Infof("streaming PC audio to %s (%s)", i.device.Name, i.device.IP)
	return nil
}

// loop services capture restarts and the keepalive check until ctx is done.
func (i *instance) loop(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	var lastRetune time.Time

	i.log.Infof("press Ctrl+C to stop")
	for {
		select {
		case <-ctx.Done():
			i.log.Infof("shutting down")
			return nil

		case <-i.restartCh:
			if i.speaker != nil && ctx.Err() == nil {
				i.log.Infof("capture restarted; retuning speaker")
				if err := i.tune(ctx); err != nil {
					i.log.Warnf("retune failed: %v", err)
				}
			}

		case <-ticker.C:
			if !i.opts.Keepalive || i.speaker == nil || ctx.Err() != nil {
				continue
			}
			state, err := i.speaker.TransportState(ctx)
			if err != nil || !shouldRetune(state, time.Since(lastRetune)) {
				continue
			}
			lastRetune = time.Now()
			i.log.Infof("speaker stopped unexpectedly; retuning (keepalive)")
			if err := i.tune(ctx); err != nil {
				i.log.Warnf("keepalive retune failed: %v", err)
			}
		}
	}
}

// close releases everything, in an order that does not leave a room pointing
// at a dead stream or a stale virtual sink behind.
func (i *instance) close() {
	i.closeOnce.Do(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if i.speaker != nil && i.tuned.Load() {
			// Another instance may have retuned the room since. Only stop the
			// speaker if it is still playing this instance's stream.
			current := ""
			if pos, err := i.speaker.Position(stopCtx); err == nil {
				current = pos.URI
			}
			if !shouldStopSpeaker(current, i.getURI()) {
				i.log.Infof("left %s alone; it is playing another stream now", describeDevice(i.device))
			} else if err := i.speaker.Stop(stopCtx); err != nil {
				i.log.Debugf("could not stop speaker: %v", err)
			} else if i.device != nil {
				i.log.Infof("stopped playback on %s (%s)", i.device.Name, i.device.IP)
			}
		}

		i.capt.Stop()
		if i.serverStarted {
			_ = i.server.Shutdown(stopCtx)
		}
		if i.vsink != nil {
			i.vsink.Dispose()
		}
	})
}

func validate(opts Options) error {
	switch opts.Format {
	case "wav", "mp3":
	default:
		return fmt.Errorf("format must be \"wav\" or \"mp3\"")
	}
	if opts.Port < 0 || opts.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}
	if opts.Bitrate < 32 || opts.Bitrate > 320 {
		return fmt.Errorf("bitrate must be between 32 and 320")
	}
	if opts.Volume < -1 || opts.Volume > 100 {
		return fmt.Errorf("volume must be between 0 and 100")
	}
	if opts.WarmupMS < 0 {
		return fmt.Errorf("warmup must be >= 0")
	}
	if opts.Station == "" {
		return fmt.Errorf("station name must not be empty")
	}
	if opts.Bind == "" {
		return fmt.Errorf("bind address must not be empty")
	}
	return nil
}

// checkPortFree binds and immediately releases the port so a clash is reported
// before discovery runs. The real listener still handles the (tiny) race
// between this probe and the actual bind.
func checkPortFree(opts Options) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(opts.Bind, strconv.Itoa(opts.Port)))
	if err != nil {
		return describeListenError(opts, err)
	}
	return ln.Close()
}

// describeListenError turns "address already in use" into something actionable.
// Re-running the command while a stream is live is the common case, so name the
// instance and the commands that talk to it.
func describeListenError(opts Options, err error) error {
	if !errors.Is(err, syscall.EADDRINUSE) {
		return err
	}
	if st, ok := statusOnPort(opts); ok {
		room := "no room (--no-tune)"
		if st.Device != nil {
			room = fmt.Sprintf("%s (%s)", st.Device.Name, st.Device.IP)
		}
		return fmt.Errorf(
			"sinkrill is already running on port %d: station %q, %s\n"+
				"  control it:   sinkrill status | sinkrill volume 40 | sinkrill retune\n"+
				"  or run a second stream on a free port: --port 0",
			opts.Port, st.Station, room,
		)
	}
	return fmt.Errorf("port %d is already in use; pass --port 0 to pick a free one", opts.Port)
}

// looksLikeSinkrill guards against mistaking an unrelated JSON service on the
// port for a running instance. A real one always reports a station and a media
// format; `{}` does not.
func looksLikeSinkrill(st *Status) bool {
	return st != nil && st.Station != "" && (st.Format == "wav" || st.Format == "mp3")
}

// statusOnPort asks a running instance on the same port who it is.
func statusOnPort(opts Options) (*Status, bool) {
	host := opts.Bind
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 700 * time.Millisecond}
	resp, err := client.Get("http://" + net.JoinHostPort(host, strconv.Itoa(opts.Port)) + "/status")
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var st Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, false
	}
	if !looksLikeSinkrill(&st) {
		return nil, false
	}
	return &st, true
}
