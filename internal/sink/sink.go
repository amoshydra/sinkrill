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

	lanIP := opts.Host
	if lanIP == "" {
		lanIP = netutil.LANIP()
	}
	if lanIP == "" {
		return errors.New("could not detect a LAN IPv4 address; pass --host <ip>")
	}

	var (
		device  *sonos.Device
		speaker *sonos.Client
	)
	if opts.Tune {
		resolved, err := sonos.Resolve(ctx, opts.Device, 3*time.Second, log)
		if err != nil {
			return err
		}
		device = &resolved
		speaker = sonos.NewClient(resolved.IP)
	}

	source := opts.Source
	var vsink *virtualsink.Sink
	if opts.VirtualSink != "" {
		created, err := virtualsink.Ensure(opts.VirtualSink, log)
		if err != nil {
			return err
		}
		vsink = created
		source = created.MonitorSource
		if created.Created {
			log.Infof("created output device %q (select it in your audio selector)", opts.VirtualSink)
		} else {
			log.Infof("using existing output device %q", opts.VirtualSink)
		}
	}

	if opts.VirtualSink == "" {
		what := source
		if source == "@DEFAULT_MONITOR@" {
			what = "the default output"
		}
		log.Infof("mirroring %s; no new audio device (use --virtual-sink for a selectable output)", what)
	}

	kbps := effectiveBitrate(opts.Format, opts.Bitrate)

	hub := stream.NewHub(opts.WarmupMS, stream.BytesPerSecond(opts.Format, opts.Bitrate), log)

	var stateMu sync.RWMutex
	var uri string
	getURI := func() string {
		stateMu.RLock()
		defer stateMu.RUnlock()
		return uri
	}
	setURI := func(value string) {
		stateMu.Lock()
		uri = value
		stateMu.Unlock()
	}

	captureFormat := capture.FormatMP3
	if opts.Format == "wav" {
		captureFormat = capture.FormatPCM
	}
	capt := capture.New(capture.Options{
		Source:     source,
		Format:     captureFormat,
		Bitrate:    opts.Bitrate,
		FFmpegPath: opts.FFmpeg,
		ParecPath:  opts.Parec,
		Logger:     log,
	})

	statusFn := func() any {
		snap := capt.Snapshot()
		status := Status{
			Station:       opts.Station,
			Source:        source,
			Format:        opts.Format,
			Bitrate:       kbps,
			URI:           getURI(),
			WarmupMS:      opts.WarmupMS,
			Listeners:     hub.Count(),
			ListenerBytes: hub.TotalBytes(),
			QueuedBytes:   hub.MaxQueuedBytes(),
			DroppedChunks: hub.TotalDrops(),
			Capture:       captureStatusAt(snap, time.Now()),
		}
		if device != nil {
			status.Device = &DeviceStatus{Name: device.Name, IP: device.IP}
		}
		if vsink != nil {
			status.VirtualSink = &VirtualSinkStatus{Name: vsink.Name, CreatedByUs: vsink.Created}
		}
		return status
	}

	server := stream.NewServer(stream.ServerOptions{
		Port:    opts.Port,
		Bind:    opts.Bind,
		Format:  opts.Format,
		Bitrate: opts.Bitrate,
		Station: opts.Station,
		Status:  statusFn,
		Logger:  log,
	}, hub)

	if err := server.Start(); err != nil {
		if vsink != nil {
			vsink.Dispose()
		}
		return describeListenError(opts, err)
	}

	var tuned atomic.Bool
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			if speaker != nil && tuned.Load() {
				// Another instance may have retuned the room since. Only stop
				// the speaker if it is still playing this instance's stream.
				current := ""
				if pos, err := speaker.Position(stopCtx); err == nil {
					current = pos.URI
				}
				if !shouldStopSpeaker(current, getURI()) {
					log.Infof("left %s alone; it is playing another stream now", describeDevice(device))
				} else if err := speaker.Stop(stopCtx); err != nil {
					log.Debugf("could not stop speaker: %v", err)
				} else if device != nil {
					log.Infof("stopped playback on %s (%s)", device.Name, device.IP)
				}
			}
			capt.Stop()
			_ = server.Shutdown(stopCtx)
			if vsink != nil {
				vsink.Dispose()
			}
		})
	}
	defer cleanup()

	firstData := make(chan struct{})
	var firstOnce sync.Once
	fatalCh := make(chan error, 1)
	restartCh := make(chan struct{}, 1)

	if err := capt.Start(ctx, capture.Callbacks{
		OnData: func(chunk []byte) {
			firstOnce.Do(func() { close(firstData) })
			hub.Write(chunk)
		},
		OnExit: func(err error) {
			if err != nil {
				log.Warnf("%v", err)
			}
		},
		OnRestart: func() {
			select {
			case restartCh <- struct{}{}:
			default:
			}
		},
		OnFatal: func(err error) {
			select {
			case fatalCh <- err:
			default:
			}
		},
	}); err != nil {
		return err
	}

	select {
	case <-firstData:
	case err := <-fatalCh:
		return fmt.Errorf("capture failed: %w", err)
	case <-time.After(firstDataTimeout):
		return errors.New(
			"no audio data arrived from the source; check that the PulseAudio/PipeWire " +
				"source exists (try `pactl list short sources`) or pass --source",
		)
	case <-ctx.Done():
		return nil
	}

	streamURI := sonos.StreamURI(lanIP, server.Port(), opts.Format)
	setURI(streamURI)
	log.Infof("advertised URI (%s): %s", opts.Format, streamURI)
	file := "stream.mp3"
	if opts.Format == "wav" {
		file = "stream.wav"
	}
	log.Infof("preview locally: http://%s:%d/%s", lanIP, server.Port(), file)

	metadata := sonos.RadioMetadata(opts.Station, "Sinkrill")
	if opts.Format == "wav" {
		metadata = sonos.WAVMetadata(opts.Station, streamURI, "Sinkrill")
	}
	tune := func() error {
		if speaker == nil {
			return nil
		}
		if err := speaker.SetAVTransportURI(ctx, streamURI, metadata); err != nil {
			return err
		}
		if err := speaker.Play(ctx); err != nil {
			log.Debugf("play after tune-in failed: %v", err)
		}
		tuned.Store(true)
		return nil
	}

	if speaker != nil && device != nil {
		if opts.Volume >= 0 {
			if err := speaker.SetVolume(ctx, opts.Volume); err != nil {
				log.Warnf("could not set volume: %v", err)
			} else {
				log.Infof("set volume on %s to %d", device.Name, opts.Volume)
			}
		}
		if err := tune(); err != nil {
			return err
		}
		log.Infof("streaming PC audio to %s (%s)", device.Name, device.IP)
	} else {
		log.Infof("running without tuning Sonos (--no-tune); point a speaker at the URI above")
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	var lastRetune time.Time

	log.Infof("press Ctrl+C to stop")
	for {
		select {
		case <-ctx.Done():
			log.Infof("shutting down")
			return nil

		case <-restartCh:
			if speaker != nil && ctx.Err() == nil {
				log.Infof("capture restarted; retuning speaker")
				if err := tune(); err != nil {
					log.Warnf("retune failed: %v", err)
				}
			}

		case <-ticker.C:
			if !opts.Keepalive || speaker == nil || ctx.Err() != nil {
				continue
			}
			state, err := speaker.TransportState(ctx)
			if err != nil || !shouldRetune(state, time.Since(lastRetune)) {
				continue
			}
			lastRetune = time.Now()
			log.Infof("speaker stopped unexpectedly; retuning (keepalive)")
			if err := tune(); err != nil {
				log.Warnf("keepalive retune failed: %v", err)
			}
		}
	}
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
				"  control it:   sinkrill status | sinkrill volume 40 | sinkrill volume up\n"+
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
