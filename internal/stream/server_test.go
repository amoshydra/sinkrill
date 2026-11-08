package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/amoshydra/thrum/internal/logx"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	hub := NewHub(0, 192000, logx.NewSilent())
	srv := NewServer(ServerOptions{
		Port:    0,
		Bind:    "127.0.0.1",
		Format:  "wav",
		Bitrate: 192,
		Station: "test",
		Status:  func() any { return map[string]any{"ok": true} },
		Logger:  logx.NewSilent(),
	}, hub)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	return srv
}

func TestServerStreamsWAVHeader(t *testing.T) {
	srv := testServer(t)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/stream.wav", srv.Port()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "audio/wav" {
		t.Errorf("Content-Type = %q", got)
	}

	header := make([]byte, 44)
	if _, err := io.ReadFull(resp.Body, header); err != nil {
		t.Fatal(err)
	}
	if string(header[:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		t.Errorf("not a WAV header: %q", header[:12])
	}
}

func TestServerStatusAndHealth(t *testing.T) {
	srv := testServer(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", srv.Port())
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(base + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Errorf("status body = %v", body)
	}

	resp2, err := client.Get(base + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("health status = %d", resp2.StatusCode)
	}
}
