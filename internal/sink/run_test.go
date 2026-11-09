//go:build linux

package sink

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amoshydra/sinkrill/internal/logx"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// fakeParec writes an executable that ignores its arguments and runs body.
func fakeParec(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "parec")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// pacedParec emits ~25 ms of 48 kHz stereo s16le per iteration. It deliberately
// runs near real-time: an unthrottled /dev/zero source delivers 32 KiB reads,
// which are larger than the hub's 125 ms per-client queue and get dropped,
// so the test would be exercising a burst path that real capture never hits.
func pacedParec(t *testing.T) string {
	t.Helper()
	return fakeParec(t, "while :; do head -c 4800 /dev/zero; sleep 0.02; done")
}

// TestRunServesAudio exercises the real wiring: capture process -> hub ->
// HTTP server -> listener, with --no-tune so no speaker is needed. It is the
// regression test for "the stream stops after a while".
func TestRunServesAudio(t *testing.T) {
	port := freePort(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			Host:    "127.0.0.1",
			Bind:    "127.0.0.1",
			Port:    port,
			Format:  "wav",
			Bitrate: 192,
			Station: "Test Station",
			Source:  "@DEFAULT_MONITOR@",
			Parec:   pacedParec(t),
			FFmpeg:  "ffmpeg",
			Volume:  -1,
			Tune:    false,
		}, logx.NewSilent())
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// Wait for the server to answer /status.
	deadline := time.Now().Add(10 * time.Second)
	var status Status
	ready := false
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("Run exited before serving: %v", err)
		default:
		}
		resp, err := client.Get(base + "/status")
		if err == nil {
			if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&status) == nil {
				ready = true
			}
			resp.Body.Close()
			if ready {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not become ready")
	}

	// The status payload is what `sinkrill status` renders.
	if status.Station != "Test Station" {
		t.Errorf("status station = %q, want %q", status.Station, "Test Station")
	}
	if status.Format != "wav" || status.Bitrate != 1536 {
		t.Errorf("status format = %q at %d kbps, want wav at 1536", status.Format, status.Bitrate)
	}
	if status.URI == "" {
		t.Error("status URI is empty; advertise did not run")
	}
	if status.Source != "@DEFAULT_MONITOR@" {
		t.Errorf("status source = %q", status.Source)
	}
	if !status.Capture.Running {
		t.Error("status says capture is not running")
	}

	// The endpoint must deliver audio promptly.
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/stream.wav", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("stream status = %d", resp.StatusCode)
	}

	buf := make([]byte, 32*1024)
	n, err := io.ReadFull(resp.Body, buf)
	resp.Body.Close()
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatalf("reading stream: %v (got %d bytes)", err, n)
	}
	if n == 0 {
		t.Fatal("stream delivered no bytes")
	}

	// Shutting down must not hang.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned an error on shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not shut down")
	}
}

// TestRunFailsWhenCaptureDies covers the fatal path: a capture process that
// produces nothing must surface an error instead of hanging until the
// first-data timeout.
func TestRunFailsWhenCaptureDies(t *testing.T) {
	port := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			Host:    "127.0.0.1",
			Bind:    "127.0.0.1",
			Port:    port,
			Format:  "wav",
			Bitrate: 192,
			Station: "Test Station",
			Source:  "@DEFAULT_MONITOR@",
			Parec:   fakeParec(t, "exit 1"),
			FFmpeg:  "ffmpeg",
			Volume:  -1,
			Tune:    false,
		}, logx.NewSilent())
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error when capture dies immediately")
		}
	case <-time.After(12 * time.Second):
		t.Fatal("Run hung instead of reporting the dead capture process")
	}
}
