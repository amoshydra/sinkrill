package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/amoshydra/thrum/internal/doctor"
	"github.com/amoshydra/thrum/internal/logx"
	"github.com/amoshydra/thrum/internal/probe"
	"github.com/amoshydra/thrum/internal/sink"
	"github.com/amoshydra/thrum/internal/sonos"
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
		fmt.Printf("thrum %s\n", version)
		return 0
	case "devices":
		return runDevices(args)
	case "stop":
		return runStop(args)
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
	case "start", "devices", "stop", "probe", "doctor", "version", "help":
		return argv[0], argv[1:]
	case "-h", "--help":
		return "help", nil
	case "-V", "--version":
		return "version", nil
	}
	if strings.HasPrefix(argv[0], "-") {
		return "start", argv
	}
	// Shorthand: `thrum "Living Room"`.
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
	fmt.Print(`thrum — stream this computer's audio to Sonos speakers

Usage:
  thrum [options]           Start streaming (same as "start")
  thrum "Living Room"       Pick a speaker by room name
  thrum devices [options]   List Sonos devices on the LAN
  thrum stop [options]      Stop playback on a device
  thrum probe [options]     Measure buffered latency
  thrum doctor [options]    Diagnose silence, capture, and routing
  thrum version             Print the version

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
      --virtual-sink [name] Create an output device (default "thrum")
      --station <name>      Name shown on the speaker (default "Thrum")
  -v, --verbose             Verbose logs

Run "thrum <command> -h" for command-specific options.
`)
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
		Station:  "Thrum",
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
	fs.Var(&virtualSink, "virtual-sink", "create/use an output device (default sonos)")
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

func runDoctor(args []string) int {
	opts := doctor.Options{
		Port:        8567,
		VirtualSink: "thrum",
	}

	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	intVar(fs, &opts.Port, "port", "p", opts.Port, "Thrum server port")
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
		Station: "Thrum Probe",
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

// optionalString supports `--virtual-sink` (uses "sonos") and
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
		o.name = "thrum"
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
