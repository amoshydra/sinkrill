//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"fmt"
	"os/exec"
)

// pulseBackend records from a PulseAudio or PipeWire server. parec is preferred
// because it has the lowest and most predictable latency; ffmpeg is the
// fallback when libpulse tools are not installed.
type pulseBackend struct{}

func (pulseBackend) name() string { return "pulseaudio" }

func (pulseBackend) command(opts Options) (string, []string, error) {
	if opts.Format == FormatPCM {
		if opts.ParecPath != "" {
			if parec, err := exec.LookPath(opts.ParecPath); err == nil {
				return parec, parecArgs(opts.Source), nil
			}
		}
		if ffmpeg, err := exec.LookPath(opts.FFmpegPath); err == nil {
			return ffmpeg, ffmpegPCMArgs(opts.Source), nil
		}
		return "", nil, fmt.Errorf(
			"no capture backend found: install parec (libpulse) or ffmpeg, or pass --ffmpeg/--parec",
		)
	}

	ffmpeg, err := exec.LookPath(opts.FFmpegPath)
	if err != nil {
		return "", nil, fmt.Errorf("ffmpeg not found at %q (needed for --format mp3)", opts.FFmpegPath)
	}
	return ffmpeg, ffmpegMP3Args(opts.Source, opts.Bitrate), nil
}

func platformBackends() []backend { return []backend{pulseBackend{}} }

func parecArgs(source string) []string {
	return []string{
		"--client-name=sinkrill",
		"--stream-name=sinkrill",
		"--device=" + source,
		"--rate=48000",
		"--format=s16le",
		"--channels=2",
		"--latency-msec=20",
		"--process-time-msec=10",
		"--raw",
	}
}

func ffmpegBaseArgs(source string) []string {
	return []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-nostdin",
		"-f", "pulse",
		"-fragment_size", "2400",
		"-i", source,
		"-ac", "2",
	}
}

func ffmpegPCMArgs(source string) []string {
	args := ffmpegBaseArgs(source)
	return append(args,
		"-ar", "48000",
		"-c:a", "pcm_s16le",
		"-flush_packets", "1",
		"-avioflags", "direct",
		"-f", "s16le",
		"pipe:1",
	)
}

func ffmpegMP3Args(source string, bitrate int) []string {
	args := ffmpegBaseArgs(source)
	return append(args,
		"-c:a", "libmp3lame",
		"-b:a", fmt.Sprintf("%dk", bitrate),
		"-compression_level", "0",
		"-write_xing", "0",
		"-id3v2_version", "0",
		"-flush_packets", "1",
		"-avioflags", "direct",
		"-f", "mp3",
		"pipe:1",
	)
}
