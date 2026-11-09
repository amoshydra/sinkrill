//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These are latency-critical. parec's --latency-msec / --process-time-msec and
// ffmpeg's -fragment_size / -flush_packets / -avioflags direct are what keep
// the pipeline at ~0.1 s instead of several seconds. Any change here has to be
// a deliberate decision, not an accident of refactoring.
func TestParecArgs(t *testing.T) {
	want := []string{
		"--client-name=sinkrill",
		"--stream-name=sinkrill",
		"--device=@DEFAULT_MONITOR@",
		"--rate=48000",
		"--format=s16le",
		"--channels=2",
		"--latency-msec=20",
		"--process-time-msec=10",
		"--raw",
	}
	if got := parecArgs("@DEFAULT_MONITOR@"); !reflect.DeepEqual(got, want) {
		t.Fatalf("parec args changed:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFFmpegPCMArgs(t *testing.T) {
	want := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-nostdin",
		"-f", "pulse",
		"-fragment_size", "2400",
		"-i", "my.monitor",
		"-ac", "2",
		"-ar", "48000",
		"-c:a", "pcm_s16le",
		"-flush_packets", "1",
		"-avioflags", "direct",
		"-f", "s16le",
		"pipe:1",
	}
	if got := ffmpegPCMArgs("my.monitor"); !reflect.DeepEqual(got, want) {
		t.Fatalf("ffmpeg PCM args changed:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFFmpegMP3Args(t *testing.T) {
	want := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-nostdin",
		"-f", "pulse",
		"-fragment_size", "2400",
		"-i", "my.monitor",
		"-ac", "2",
		"-c:a", "libmp3lame",
		"-b:a", "192k",
		"-compression_level", "0",
		"-write_xing", "0",
		"-id3v2_version", "0",
		"-flush_packets", "1",
		"-avioflags", "direct",
		"-f", "mp3",
		"pipe:1",
	}
	if got := ffmpegMP3Args("my.monitor", 192); !reflect.DeepEqual(got, want) {
		t.Fatalf("ffmpeg MP3 args changed:\n got: %#v\nwant: %#v", got, want)
	}
}

// The read loop must keep handing data on in small chunks; a bigger buffer here
// adds coalescing delay to every listener.
func TestReadBufferStaysSmall(t *testing.T) {
	if readBufferSize != 32*1024 {
		t.Fatalf("read buffer changed to %d bytes; keep it at 32 KiB unless latency is re-measured", readBufferSize)
	}
}

func fakeBin(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBackendPrefersParec(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "parec")
	fakeBin(t, dir, "ffmpeg")
	t.Setenv("PATH", dir)

	path, args, err := selectBackend(Options{Format: FormatPCM, Source: "s", ParecPath: "parec", FFmpegPath: "ffmpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "parec" {
		t.Fatalf("wanted parec, got %s", path)
	}
	if !reflect.DeepEqual(args, parecArgs("s")) {
		t.Fatalf("parec args differ: %#v", args)
	}
}

func TestBackendFallsBackToFFmpeg(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "ffmpeg")
	t.Setenv("PATH", dir)

	path, args, err := selectBackend(Options{Format: FormatPCM, Source: "s", ParecPath: "parec", FFmpegPath: "ffmpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "ffmpeg" {
		t.Fatalf("wanted ffmpeg, got %s", path)
	}
	if !reflect.DeepEqual(args, ffmpegPCMArgs("s")) {
		t.Fatalf("ffmpeg args differ: %#v", args)
	}
}

func TestBackendMP3RequiresFFmpeg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, _, err := selectBackend(Options{Format: FormatMP3, Source: "s", FFmpegPath: "ffmpeg"}); err == nil {
		t.Fatal("expected an error when ffmpeg is missing in MP3 mode")
	}
}

func TestBackendMissingEverything(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, _, err := selectBackend(Options{Format: FormatPCM, Source: "s", ParecPath: "parec", FFmpegPath: "ffmpeg"})
	if err == nil || !strings.Contains(err.Error(), "no capture backend found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
