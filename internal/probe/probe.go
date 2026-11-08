package probe

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/amoshydra/sonos-sink/internal/capture"
	"github.com/amoshydra/sonos-sink/internal/logx"
	"github.com/amoshydra/sonos-sink/internal/netutil"
	"github.com/amoshydra/sonos-sink/internal/sonos"
	"github.com/amoshydra/sonos-sink/internal/stream"
)

type Options struct {
	Device  string
	Source  string
	Port    int
	Bitrate int
	Format  string
	Host    string
	Bind    string
	FFmpeg  string
	Parec   string
	Seconds int
	Station string
}

// Run measures how much audio Sonos has buffered but not yet played:
// delivered seconds (from bytes written to the speaker's connection) minus the
// speaker's reported playback position.
func Run(ctx context.Context, opts Options, log *logx.Logger) error {
	lanIP := opts.Host
	if lanIP == "" {
		lanIP = netutil.LANIP()
	}
	if lanIP == "" {
		return errors.New("could not detect a LAN IPv4 address; pass --host <ip>")
	}
	if opts.Format != "wav" && opts.Format != "mp3" {
		return errors.New("format must be \"wav\" or \"mp3\"")
	}

	device, err := sonos.Resolve(ctx, opts.Device, 3*time.Second, log)
	if err != nil {
		return err
	}
	speaker := sonos.NewClient(device.IP)

	hub := stream.NewHub(0, stream.BytesPerSecond(opts.Format, opts.Bitrate), log)
	captureFormat := capture.FormatMP3
	if opts.Format == "wav" {
		captureFormat = capture.FormatPCM
	}
	capt := capture.New(capture.Options{
		Source:     opts.Source,
		Format:     captureFormat,
		Bitrate:    opts.Bitrate,
		FFmpegPath: opts.FFmpeg,
		ParecPath:  opts.Parec,
		Logger:     log,
	})

	server := stream.NewServer(stream.ServerOptions{
		Port:    opts.Port,
		Bind:    opts.Bind,
		Format:  opts.Format,
		Bitrate: opts.Bitrate,
		Station: opts.Station,
		Status: func() any {
			return map[string]any{"probe": true, "listeners": hub.Count()}
		},
		Logger: log,
	}, hub)
	if err := server.Start(); err != nil {
		return err
	}

	firstData := make(chan struct{})
	var firstOnce bool
	fatalCh := make(chan error, 1)
	if err := capt.Start(ctx, capture.Callbacks{
		OnData: func(chunk []byte) {
			if !firstOnce {
				firstOnce = true
				close(firstData)
			}
			hub.Write(chunk)
		},
		OnFatal: func(err error) {
			select {
			case fatalCh <- err:
			default:
			}
		},
	}); err != nil {
		_ = server.Shutdown(context.Background())
		return err
	}

	cleanup := func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = speaker.Stop(stopCtx)
		capt.Stop()
		_ = server.Shutdown(stopCtx)
	}
	defer cleanup()

	select {
	case <-firstData:
	case err := <-fatalCh:
		return fmt.Errorf("capture failed: %w", err)
	case <-time.After(8 * time.Second):
		return errors.New("no audio data arrived from the source")
	case <-ctx.Done():
		return nil
	}

	uri := sonos.StreamURI(lanIP, server.Port(), opts.Format)
	metadata := sonos.RadioMetadata(opts.Station, "sonos-sink")
	if opts.Format == "wav" {
		metadata = sonos.WAVMetadata(opts.Station, uri, "sonos-sink")
	}
	tuneStart := time.Now()
	if err := speaker.SetAVTransportURI(ctx, uri, metadata); err != nil {
		return err
	}
	if err := speaker.Play(ctx); err != nil {
		log.Debugf("play after tune-in failed: %v", err)
	}
	log.Infof("tuned %s -> %s (%s)", device.Name, uri, opts.Format)

	start := time.Now()
	deadline := start.Add(time.Duration(opts.Seconds) * time.Second)
	var (
		samples      []float64
		playingAfter time.Duration
		playingSeen  bool
		bytesPerSec  = stream.BytesPerSecond(opts.Format, opts.Bitrate)
	)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
		}

		state, err := speaker.TransportState(ctx)
		if err != nil {
			continue
		}
		position, err := speaker.Position(ctx)
		if err != nil {
			continue
		}
		if strings.EqualFold(state, "playing") && !playingSeen {
			playingSeen = true
			playingAfter = time.Since(tuneStart)
		}
		if !strings.EqualFold(state, "playing") {
			continue
		}

		delivered := float64(hub.MaxOutstandingBytes()) / float64(bytesPerSec)
		samples = append(samples, delivered-float64(position.Seconds))
	}

	if len(samples) == 0 {
		return errors.New("no samples collected; is the speaker playing?")
	}

	sort.Float64s(samples)
	min := samples[0]
	p50 := samples[len(samples)/2]
	p90 := samples[int(float64(len(samples))*0.9)]

	fmt.Printf("\n=== %s ===\n", strings.ToUpper(opts.Format))
	if playingSeen {
		fmt.Printf("playback started after: %s\n", playingAfter.Round(time.Millisecond))
	} else {
		fmt.Println("playback started after: n/a")
	}
	fmt.Printf("samples: %d\n", len(samples))
	fmt.Printf("lag min: %.2f s\n", min)
	fmt.Printf("lag p50: %.2f s\n", p50)
	fmt.Printf("lag p90: %.2f s\n", p90)
	fmt.Println("estimate: latency is between min and min+1 s (position is whole seconds)")
	return nil
}
