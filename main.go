package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/amoshydra/sinkrill/internal/capture"
	"github.com/amoshydra/sinkrill/internal/doctor"
	"github.com/amoshydra/sinkrill/internal/logx"
	"github.com/amoshydra/sinkrill/internal/probe"
	"github.com/amoshydra/sinkrill/internal/sink"
	"github.com/amoshydra/sinkrill/internal/sonos"
	"github.com/amoshydra/sinkrill/internal/virtualsink"
)

// version is set by the release build (-ldflags "-X main.version=...").
var version = "dev"

var releaseVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	command, args := splitCommand(argv)
	switch command {
	case "help":
		printHelp()
		return 0
	case "version":
		resolveVersion()
		fmt.Printf("sinkrill %s\n", version)
		return 0
	case "devices":
		return runDevices(args)
	case "stop":
		return runStop(args)
	case "volume":
		return runVolume(args)
	case "retune":
		return runRetune(args)
	case "cleanup":
		return runCleanup(args)
	case "status":
		return runStatus(args)
	case "probe":
		return runProbe(args)
	case "doctor":
		return runDoctor(args)
	default:
		return runStart(args)
	}
}

func splitCommand(argv []string) (string, []string) {
	if len(argv) == 0 {
		return "start", nil
	}
	switch argv[0] {
	case "start", "devices", "stop", "volume", "retune", "cleanup", "status", "probe", "doctor", "version", "help":
		return argv[0], argv[1:]
	case "-h", "--help":
		return "help", nil
	case "-V", "--version":
		return "version", nil
	}
	if strings.HasPrefix(argv[0], "-") {
		return "start", argv
	}
	// Shorthand: `sinkrill "Living Room"`.
	return "start", append([]string{"--device", argv[0]}, argv[1:]...)
}

func resolveVersion() {
	if version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		// Only accept a clean release version. Local builds report a VCS
		// pseudo-version like v0.0.0-2026...+dirty, which is not useful.
		if releaseVersion.MatchString(info.Main.Version) {
			version = info.Main.Version
		}
	}
}

func printHelp() {
	fmt.Printf(`sinkrill — stream this computer's audio to Sonos speakers

Usage:
  sinkrill --virtual-sink             Add a "Sinkrill" output device (recommended)
  sinkrill --virtual-sink "Living Room"
  sinkrill                            Mirror the current output (no new device)
  sinkrill devices [options]          List Sonos devices on the LAN
  sinkrill stop [options]             Stop playback on a device
  sinkrill volume [args]              Show or change the speaker volume
  sinkrill retune [options]           Point the speaker back at the running stream
  sinkrill cleanup                    Remove a leftover output device after a crash
  sinkrill status                     Show what the running instance is doing
  sinkrill doctor [options]           Diagnose silence, capture, and routing
  sinkrill probe [options]            Measure buffered latency
  sinkrill version                    Print the version

Options for start:
  -d, --device <name|ip>    Sonos room name or IP (auto-detected if only one)
  -s, --source <source>     PulseAudio/PipeWire source (default @DEFAULT_MONITOR@)
  -p, --port <port>         HTTP port (default 8567, 0 picks a free one)
  -f, --format <wav|mp3>    wav = lowest latency, mp3 = lower bandwidth
  -b, --bitrate <kbps>      MP3 bitrate (default 192)
      --warmup <ms>         Replay audio to new listeners (default 0)
      --host <ip>           LAN IP advertised to Sonos (auto-detected)
      --bind <address>      HTTP bind address (default 0.0.0.0)
      --ffmpeg <path>       ffmpeg binary (default ffmpeg)
      --parec <path>        parec binary (default parec)
      --volume <0-100>      Set the speaker volume on start
      --keepalive           Re-tune Sonos if it stops by itself
      --no-tune             Only serve the stream; do not control Sonos
      --virtual-sink [name] Create the Sinkrill output device (recommended)
      --station <name>      Name shown on the speaker (default %q)
  -v, --verbose             Verbose logs

Volume:
  sinkrill volume                     Print the current volume
  sinkrill volume 40                  Set it (0-100)
  sinkrill volume +5 | up 10          Nudge it up
  sinkrill volume -5 | down           Nudge it down
  sinkrill volume mute | unmute

Run "sinkrill <command> -h" for command-specific options.
`, sink.DefaultStation())
}

func runStart(args []string) int {
	args = normalizeVirtualSink(args)

	opts := sink.Options{
		Source:   "@DEFAULT_MONITOR@",
		Port:     8567,
		Bitrate:  192,
		Format:   "wav",
		Bind:     "0.0.0.0",
		FFmpeg:   "ffmpeg",
		Parec:    "parec",
		Volume:   -1,
		Tune:     true,
		Station:  sink.DefaultStation(),
		WarmupMS: 0,
	}
	var virtualSink optionalString
	var noTune bool

	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	strVar(fs, &opts.Device, "device", "d", "", "Sonos room name or IP")
	strVar(fs, &opts.Source, "source", "s", opts.Source, "PulseAudio/PipeWire source")
	intVar(fs, &opts.Port, "port", "p", opts.Port, "HTTP port (0 = random)")
	strVar(fs, &opts.Format, "format", "f", opts.Format, "wav or mp3")
	intVar(fs, &opts.Bitrate, "bitrate", "b", opts.Bitrate, "MP3 bitrate in kbps")
	intVar(fs, &opts.WarmupMS, "warmup", "", opts.WarmupMS, "replay this much audio to new listeners (ms)")
	strVar(fs, &opts.Host, "host", "", "", "LAN IP advertised to Sonos")
	strVar(fs, &opts.Bind, "bind", "", opts.Bind, "HTTP bind address")
	strVar(fs, &opts.FFmpeg, "ffmpeg", "", opts.FFmpeg, "ffmpeg binary")
	strVar(fs, &opts.Parec, "parec", "", opts.Parec, "parec binary")
	intVar(fs, &opts.Volume, "volume", "", -1, "set speaker volume (0-100)")
	boolVar(fs, &opts.Keepalive, "keepalive", "", false, "re-tune if the speaker stops")
	boolVar(fs, &noTune, "no-tune", "", false, "only serve the stream")
	fs.Var(&virtualSink, "virtual-sink", "create/use the Sinkrill output device (recommended)")
	strVar(fs, &opts.Station, "station", "", opts.Station, "station name shown on the speaker")
	verbose := boolVar(fs, nil, "verbose", "v", false, "verbose logging")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if noTune {
		opts.Tune = false
	}
	if virtualSink.set {
		opts.VirtualSink = virtualSink.name
	}

	log := logx.New(*verbose)
	ctx, stop := signalContext()
	defer stop()

	if err := sink.Run(ctx, opts, log); err != nil {
		log.Errorf("%v", err)
		return 1
	}
	return 0
}

func runDevices(args []string) int {
	fs := flag.NewFlagSet("devices", flag.ContinueOnError)
	timeout := fs.Int("timeout", 3000, "discovery timeout in ms")
	asJSON := fs.Bool("json", false, "print JSON")
	verbose := boolVar(fs, nil, "verbose", "v", false, "verbose logging")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	log := logx.New(*verbose)
	ctx, stop := signalContext()
	defer stop()

	devices, err := sonos.Discover(ctx, time.Duration(*timeout)*time.Millisecond, log)
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}
	if len(devices) == 0 {
		log.Errorf("no Sonos devices found. Check that the speakers are powered on and on the same LAN.")
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(devices); err != nil {
			log.Errorf("%v", err)
			return 1
		}
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, device := range devices {
		fmt.Fprintf(w, "%s\t%s\t%s\n", device.Name, device.IP, device.Model)
	}
	_ = w.Flush()
	return 0
}

func runStop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	device := ""
	strVar(fs, &device, "device", "d", "", "Sonos room name or IP")
	verbose := boolVar(fs, nil, "verbose", "v", false, "verbose logging")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	log := logx.New(*verbose)
	ctx, stop := signalContext()
	defer stop()

	resolved, err := sonos.Resolve(ctx, device, 3*time.Second, log)
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}
	if err := sonos.NewClient(resolved.IP).Stop(ctx); err != nil {
		log.Errorf("%v", err)
		return 1
	}
	log.Infof("stopped %s (%s)", resolved.Name, resolved.IP)
	return 0
}

type volumeMode int

const (
	volumeShow volumeMode = iota
	volumeSet
	volumeDelta
	volumeMute
	volumeUnmute
)

// parseVolumeArgs interprets the positional arguments of `sinkrill volume`.
func parseVolumeArgs(args []string) (volumeMode, int, error) {
	if len(args) == 0 {
		return volumeShow, 0, nil
	}
	if len(args) > 2 {
		return 0, 0, fmt.Errorf("expected at most two arguments, got %d", len(args))
	}

	switch args[0] {
	case "mute":
		if len(args) != 1 {
			return 0, 0, fmt.Errorf("mute takes no arguments")
		}
		return volumeMute, 0, nil
	case "unmute":
		if len(args) != 1 {
			return 0, 0, fmt.Errorf("unmute takes no arguments")
		}
		return volumeUnmute, 0, nil
	case "up", "down":
		step := 5
		if len(args) == 2 {
			n, err := strconv.Atoi(args[1])
			if err != nil || n <= 0 {
				return 0, 0, fmt.Errorf("step must be a positive integer")
			}
			step = n
		}
		if args[0] == "down" {
			step = -step
		}
		return volumeDelta, step, nil
	}

	v, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, 0, fmt.Errorf("volume must be 0-100, +N, -N, up, down, mute, or unmute")
	}
	if strings.HasPrefix(args[0], "+") || strings.HasPrefix(args[0], "-") {
		if len(args) != 1 {
			return 0, 0, fmt.Errorf("a relative volume takes no extra arguments")
		}
		return volumeDelta, v, nil
	}
	if v < 0 || v > 100 {
		return 0, 0, fmt.Errorf("volume must be between 0 and 100")
	}
	return volumeSet, v, nil
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

// localStatus reads /status from a running instance. It is used to reuse the
// room that instance is already tuned to, instead of rediscovering it.
func localStatus(port int) (*sink.Status, bool) {
	client := &http.Client{Timeout: 700 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/status", port))
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var st sink.Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, false
	}
	// Another service could return valid JSON on this path; require the fields
	// a real instance always reports.
	if st.Station == "" {
		return nil, false
	}
	return &st, true
}

// resolveTarget picks the room to act on: an explicit --device, else whatever
// the running instance is tuned to, else discovery.
func resolveTarget(ctx context.Context, device string, port int, log *logx.Logger) (sonos.Device, error) {
	if device != "" {
		return sonos.Resolve(ctx, device, 3*time.Second, log)
	}
	if st, ok := localStatus(port); ok && st.Device != nil {
		return sonos.Device{Name: st.Device.Name, IP: st.Device.IP}, nil
	}
	return sonos.Resolve(ctx, "", 3*time.Second, log)
}

func runVolume(args []string) int {
	fs := flag.NewFlagSet("volume", flag.ContinueOnError)
	device := ""
	port := 8567
	strVar(fs, &device, "device", "d", "", "Sonos room name or IP")
	intVar(fs, &port, "port", "p", port, "port of the running instance to read the room from")
	verbose := boolVar(fs, nil, "verbose", "v", false, "verbose logging")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	log := logx.New(*verbose)
	mode, value, err := parseVolumeArgs(fs.Args())
	if err != nil {
		log.Errorf("%v", err)
		return 2
	}

	ctx, stop := signalContext()
	defer stop()

	dev, err := resolveTarget(ctx, device, port, log)
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}
	speaker := sonos.NewClient(dev.IP)

	switch mode {
	case volumeShow:
		level, err := speaker.Volume(ctx)
		if err != nil {
			log.Errorf("%v", err)
			return 1
		}
		line := fmt.Sprintf("%s: %d", dev.Name, level)
		if muted, err := speaker.Muted(ctx); err == nil && muted {
			line += " (muted)"
		}
		fmt.Println(line)

	case volumeMute, volumeUnmute:
		muted := mode == volumeMute
		if err := speaker.SetMute(ctx, muted); err != nil {
			log.Errorf("%v", err)
			return 1
		}
		if muted {
			log.Infof("%s muted", dev.Name)
		} else {
			log.Infof("%s unmuted", dev.Name)
		}

	case volumeSet:
		if err := speaker.SetVolume(ctx, value); err != nil {
			log.Errorf("%v", err)
			return 1
		}
		log.Infof("%s volume: %d", dev.Name, value)

	case volumeDelta:
		current, err := speaker.Volume(ctx)
		if err != nil {
			log.Errorf("%v", err)
			return 1
		}
		target := clampVolume(current + value)
		if err := speaker.SetVolume(ctx, target); err != nil {
			log.Errorf("%v", err)
			return 1
		}
		log.Infof("%s volume: %d -> %d", dev.Name, current, target)
	}
	return 0
}

// streamMetadata rebuilds the DIDL-Lite payload the running instance used, so
// a manual retune looks identical to the original tune-in.
func streamMetadata(format, station, uri string) string {
	if format == "wav" {
		return sonos.WAVMetadata(station, uri, "Sinkrill")
	}
	return sonos.RadioMetadata(station, "Sinkrill")
}

// runRetune points a speaker back at an already running stream. It is the
// manual equivalent of --keepalive.
func runRetune(args []string) int {
	fs := flag.NewFlagSet("retune", flag.ContinueOnError)
	device := ""
	port := 8567
	strVar(fs, &device, "device", "d", "", "Sonos room name or IP")
	intVar(fs, &port, "port", "p", port, "port of the running instance")
	verbose := boolVar(fs, nil, "verbose", "v", false, "verbose logging")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	log := logx.New(*verbose)
	st, ok := localStatus(port)
	if !ok {
		log.Errorf("no sinkrill instance is listening on port %d; start one with `sinkrill --virtual-sink`", port)
		return 1
	}
	if st.URI == "" {
		log.Errorf("the running instance has not advertised a stream yet; check `sinkrill status`")
		return 1
	}

	ctx, stop := signalContext()
	defer stop()

	var target sonos.Device
	switch {
	case device != "":
		resolved, err := sonos.Resolve(ctx, device, 3*time.Second, log)
		if err != nil {
			log.Errorf("%v", err)
			return 1
		}
		target = resolved
	case st.Device != nil:
		target = sonos.Device{Name: st.Device.Name, IP: st.Device.IP}
	default:
		log.Errorf("the running instance is not tuned to a room (started with --no-tune); pass --device")
		return 1
	}

	speaker := sonos.NewClient(target.IP)
	if err := speaker.SetAVTransportURI(ctx, st.URI, streamMetadata(st.Format, st.Station, st.URI)); err != nil {
		log.Errorf("%v", err)
		return 1
	}
	if err := speaker.Play(ctx); err != nil {
		log.Errorf("%v", err)
		return 1
	}
	log.Infof("%s (%s) now playing %s", target.Name, target.IP, st.URI)
	return 0
}

// runCleanup clears what a hard kill leaves behind: the null sink and the
// capture process that was still attached to its monitor.
func runCleanup(args []string) int {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	name := "sinkrill"
	port := 8567
	strVar(fs, &name, "virtual-sink", "", "sinkrill", "sink name to remove")
	intVar(fs, &port, "port", "p", port, "port to check for a running instance")
	verbose := boolVar(fs, nil, "verbose", "v", false, "verbose logging")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	log := logx.New(*verbose)
	// Only refuse if the running instance is using the sink we are about to
	// remove. Another instance on a different sink is none of our business.
	if st, ok := localStatus(port); ok && st.VirtualSink != nil && st.VirtualSink.Name == name {
		log.Errorf("a sinkrill instance on port %d is using %q; stop it before cleaning up", port, name)
		return 1
	}

	// Kill the readers first so the module can be unloaded cleanly.
	stopped := capture.KillStraySinkReaders(name)
	if stopped > 0 {
		log.Infof("stopped %d leftover capture process(es)", stopped)
	}

	removed, err := virtualsink.Remove(name)
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}
	if !removed {
		if stopped == 0 {
			log.Infof("nothing to clean up for %q", name)
		}
		return 0
	}
	log.Infof("removed leftover output device %q", name)
	return 0
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	port := 8567
	intVar(fs, &port, "port", "p", port, "port of the running instance")
	asJSON := fs.Bool("json", false, "print raw JSON")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	st, ok := localStatus(port)
	if !ok {
		fmt.Fprintf(os.Stderr, "no sinkrill instance is listening on http://127.0.0.1:%d\n", port)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(st)
		return 0
	}

	room := "not tuned (--no-tune)"
	if st.Device != nil {
		room = fmt.Sprintf("%s (%s)", st.Device.Name, st.Device.IP)
	}
	capture := "running"
	if !st.Capture.Running {
		capture = "stopped"
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "Room\t%s\n", room)
	fmt.Fprintf(w, "Station\t%s\n", st.Station)
	fmt.Fprintf(w, "Format\t%s (%d kbps)\n", st.Format, st.Bitrate)
	fmt.Fprintf(w, "URI\t%s\n", st.URI)
	fmt.Fprintf(w, "Capture\t%s, %d restarts, %d ms since data\n", capture, st.Capture.Restarts, st.Capture.LastDataMS)
	if st.Capture.LastError != "" {
		fmt.Fprintf(w, "Error\t%s\n", st.Capture.LastError)
	}
	fmt.Fprintf(w, "Listeners\t%d (%d B queued, %d dropped)\n", st.Listeners, st.QueuedBytes, st.DroppedChunks)
	_ = w.Flush()
	return 0
}

func runDoctor(args []string) int {
	opts := doctor.Options{
		Port:        8567,
		VirtualSink: "sinkrill",
	}

	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	intVar(fs, &opts.Port, "port", "p", opts.Port, "Sinkrill server port")
	strVar(fs, &opts.Device, "device", "d", "", "Sonos room name or IP")
	strVar(fs, &opts.VirtualSink, "virtual-sink", "", opts.VirtualSink, "expected output device name")
	verbose := boolVar(fs, nil, "verbose", "v", false, "verbose logging")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}

	log := logx.New(*verbose)
	ctx, stop := signalContext()
	defer stop()

	return doctor.Run(ctx, opts, log)
}

func runProbe(args []string) int {
	opts := probe.Options{
		Source:  "@DEFAULT_MONITOR@",
		Port:    0,
		Bitrate: 192,
		Format:  "wav",
		Bind:    "0.0.0.0",
		FFmpeg:  "ffmpeg",
		Parec:   "parec",
		Seconds: 30,
		Station: sink.DefaultStation() + " (probe)",
	}

	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	strVar(fs, &opts.Device, "device", "d", "", "Sonos room name or IP")
	strVar(fs, &opts.Source, "source", "s", opts.Source, "PulseAudio/PipeWire source")
	intVar(fs, &opts.Port, "port", "p", opts.Port, "HTTP port (0 = random)")
	strVar(fs, &opts.Format, "format", "f", opts.Format, "wav or mp3")
	intVar(fs, &opts.Bitrate, "bitrate", "b", opts.Bitrate, "MP3 bitrate in kbps")
	strVar(fs, &opts.Host, "host", "", "", "LAN IP advertised to Sonos")
	strVar(fs, &opts.Bind, "bind", "", opts.Bind, "HTTP bind address")
	strVar(fs, &opts.FFmpeg, "ffmpeg", "", opts.FFmpeg, "ffmpeg binary")
	strVar(fs, &opts.Parec, "parec", "", opts.Parec, "parec binary")
	intVar(fs, &opts.Seconds, "seconds", "", opts.Seconds, "measurement duration")
	verbose := boolVar(fs, nil, "verbose", "v", false, "verbose logging")

	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if opts.Seconds < 1 || opts.Seconds > 600 {
		fmt.Fprintln(os.Stderr, "seconds must be between 1 and 600")
		return 2
	}

	log := logx.New(*verbose)
	ctx, stop := signalContext()
	defer stop()

	if err := probe.Run(ctx, opts, log); err != nil {
		log.Errorf("%v", err)
		return 1
	}
	return 0
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
}

func parseFlags(fs *flag.FlagSet, args []string) (int, bool) {
	fs.SetOutput(os.Stderr)
	err := fs.Parse(args)
	if err == flag.ErrHelp {
		return 0, false
	}
	if err != nil {
		return 2, false
	}
	return 0, true
}

func strVar(fs *flag.FlagSet, target *string, long, short, value, usage string) {
	fs.StringVar(target, long, value, usage)
	if short != "" {
		fs.StringVar(target, short, value, "alias for --"+long)
	}
}

func intVar(fs *flag.FlagSet, target *int, long, short string, value int, usage string) {
	fs.IntVar(target, long, value, usage)
	if short != "" {
		fs.IntVar(target, short, value, "alias for --"+long)
	}
}

// boolVar registers a bool flag. If target is nil it returns the new pointer.
func boolVar(fs *flag.FlagSet, target *bool, long, short string, value bool, usage string) *bool {
	if target == nil {
		target = new(bool)
	}
	fs.BoolVar(target, long, value, usage)
	if short != "" {
		fs.BoolVar(target, short, value, "alias for --"+long)
	}
	return target
}

// optionalString supports `--virtual-sink` (uses "sinkrill") and
// `--virtual-sink=name`.
type optionalString struct {
	set  bool
	name string
}

// normalizeVirtualSink rewrites `--virtual-sink name` to
// `--virtual-sink=name`. Go's flag package treats a bool-like flag as not
// consuming the next argument, which would also stop parsing at the name.
func normalizeVirtualSink(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if (arg == "--virtual-sink" || arg == "-virtual-sink") &&
			i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			out = append(out, arg+"="+args[i+1])
			i++
			continue
		}
		out = append(out, arg)
	}
	return out
}

func (o *optionalString) String() string {
	if !o.set {
		return ""
	}
	return o.name
}

func (o *optionalString) Set(value string) error {
	switch value {
	case "", "true":
		o.set = true
		o.name = "sinkrill"
	case "false":
		o.set = false
		o.name = ""
	default:
		o.set = true
		o.name = value
	}
	return nil
}

func (o *optionalString) IsBoolFlag() bool { return true }
