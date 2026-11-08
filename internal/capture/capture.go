package capture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/amoshydra/sinkrill/internal/logx"
)

const (
	SampleRate    = 48000
	Channels      = 2
	BitsPerSample = 16
)

type Format string

const (
	FormatPCM Format = "pcm"
	FormatMP3 Format = "mp3"
)

type Options struct {
	// Source is a PulseAudio/PipeWire source name, e.g. @DEFAULT_MONITOR@.
	Source     string
	Format     Format
	Bitrate    int
	FFmpegPath string
	ParecPath  string
	Logger     *logx.Logger
}

type Callbacks struct {
	// OnData receives an owned chunk that must not be reused by the caller.
	OnData    func([]byte)
	OnExit    func(error)
	OnRestart func()
	OnFatal   func(error)
}

type Snapshot struct {
	Running      bool
	Restarts     int
	Uptime       time.Duration
	LastError    string
	LastDataAt   time.Time
	StallTimeout time.Duration
}

type Capture struct {
	opts Options
	cb   Callbacks
	log  *logx.Logger

	mu           sync.Mutex
	cmd          *exec.Cmd
	stopping     bool
	running      bool
	produced     bool
	restarts     int
	startedAt    time.Time
	lastDataAt   time.Time
	stallTimeout time.Duration
	lastErr      string
}

func New(opts Options) *Capture {
	return &Capture{
		opts:         opts,
		log:          opts.Logger,
		stallTimeout: 5 * time.Second,
	}
}

// Start validates the capture backend and launches the restart loop.
func (c *Capture) Start(ctx context.Context, cb Callbacks) error {
	if _, _, err := c.command(); err != nil {
		return err
	}
	c.cb = cb
	go c.loop(ctx)
	go c.watchdog(ctx)
	return nil
}

func (c *Capture) command() (string, []string, error) {
	if c.opts.Format == FormatPCM {
		if c.opts.ParecPath != "" {
			if parec, err := exec.LookPath(c.opts.ParecPath); err == nil {
				return parec, parecArgs(c.opts.Source), nil
			}
		}
		if ffmpeg, err := exec.LookPath(c.opts.FFmpegPath); err == nil {
			return ffmpeg, ffmpegPCMArgs(c.opts.Source), nil
		}
		return "", nil, fmt.Errorf(
			"no capture backend found: install parec (libpulse) or ffmpeg, or pass --ffmpeg/--parec",
		)
	}

	ffmpeg, err := exec.LookPath(c.opts.FFmpegPath)
	if err != nil {
		return "", nil, fmt.Errorf("ffmpeg not found at %q (needed for --format mp3)", c.opts.FFmpegPath)
	}
	return ffmpeg, ffmpegMP3Args(c.opts.Source, c.opts.Bitrate), nil
}

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

func (c *Capture) loop(ctx context.Context) {
	for {
		started := time.Now()
		err := c.runOnce()

		c.mu.Lock()
		produced := c.produced
		stopping := c.stopping
		c.lastErr = errorString(err)
		c.mu.Unlock()

		if stopping || ctx.Err() != nil {
			return
		}

		// Before any audio arrived the setup is broken; don't loop forever.
		if !produced {
			if c.cb.OnFatal != nil {
				c.cb.OnFatal(err)
			}
			return
		}

		if c.cb.OnExit != nil {
			c.cb.OnExit(err)
		}

		delay := 2 * time.Second
		if time.Since(started) < 1500*time.Millisecond {
			delay = 5 * time.Second
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}

		c.mu.Lock()
		c.restarts++
		c.mu.Unlock()

		if c.cb.OnRestart != nil {
			c.cb.OnRestart()
		}
	}
}

func (c *Capture) runOnce() error {
	path, args, err := c.command()
	if err != nil {
		return err
	}

	cmd := exec.Command(path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &tailBuffer{max: 4096}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	c.mu.Lock()
	c.cmd = cmd
	c.running = true
	c.startedAt = time.Now()
	c.lastDataAt = time.Now()
	c.mu.Unlock()

	c.log.Infof("capturing %s -> %s via %s", c.opts.Source, c.target(), filepath.Base(path))

	defer func() {
		c.mu.Lock()
		c.cmd = nil
		c.running = false
		c.mu.Unlock()
	}()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			c.mu.Lock()
			c.produced = true
			c.lastDataAt = time.Now()
			c.mu.Unlock()

			if c.cb.OnData != nil {
				c.cb.OnData(chunk)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				c.log.Debugf("capture read error: %v", readErr)
			}
			break
		}
	}

	waitErr := cmd.Wait()
	if text := stderr.String(); text != "" {
		c.log.Debugf("capture stderr: %s", text)
	}
	if waitErr != nil {
		return fmt.Errorf("%s exited: %w: %s", filepath.Base(path), waitErr, stderr.String())
	}
	return fmt.Errorf("%s exited", filepath.Base(path))
}

func (c *Capture) target() string {
	if c.opts.Format == FormatPCM {
		return fmt.Sprintf("PCM %d Hz / %d-bit stereo", SampleRate, BitsPerSample)
	}
	return fmt.Sprintf("MP3 %d kbps", c.opts.Bitrate)
}

// watchdog restarts the capture process if it is alive but has stopped
// delivering data. A PipeWire/PulseAudio hiccup can leave the process running
// while the monitor stream goes silent, which otherwise plays as dead air.
func (c *Capture) watchdog(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			running := c.running
			stopping := c.stopping
			produced := c.produced
			cmd := c.cmd
			lastData := c.lastDataAt
			c.mu.Unlock()

			if !running || stopping || !produced || cmd == nil || cmd.Process == nil {
				continue
			}
			if lastData.IsZero() || time.Since(lastData) <= c.stallTimeout {
				continue
			}

			// Silence still arrives as PCM frames; this only fires when the
			// capture process is alive but delivering literally zero bytes.
			c.log.Warnf("capture delivered no bytes for %s; restarting", c.stallTimeout)
			c.mu.Lock()
			c.lastDataAt = time.Now() // avoid repeated signals before exit
			c.mu.Unlock()
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}
}

func (c *Capture) Stop() {
	c.mu.Lock()
	c.stopping = true
	cmd := c.cmd
	c.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	_ = cmd.Process.Signal(syscall.SIGTERM)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		running := c.running
		c.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
}

func (c *Capture) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	var uptime time.Duration
	if c.running {
		uptime = time.Since(c.startedAt)
	}
	return Snapshot{
		Running:      c.running,
		Restarts:     c.restarts,
		Uptime:       uptime,
		LastError:    c.lastErr,
		LastDataAt:   c.lastDataAt,
		StallTimeout: c.stallTimeout,
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// tailBuffer keeps only the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.max:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(bytes.TrimSpace(t.buf))
}
