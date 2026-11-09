package sink

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestDescribeListenErrorDetectsRunningInstance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(Status{
			Station: "homedesktop streaming audio",
			Device:  &DeviceStatus{Name: "Den", IP: "192.168.0.188"},
		})
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	got := describeListenError(Options{Bind: "127.0.0.1", Port: port}, syscall.EADDRINUSE)
	for _, want := range []string{"already running", "Den", "192.168.0.188", "sinkrill volume"} {
		if !strings.Contains(got.Error(), want) {
			t.Errorf("message missing %q:\n%v", want, got)
		}
	}
}

func TestDescribeListenErrorWithoutInstance(t *testing.T) {
	// Nothing listening: fall back to the generic hint rather than claiming
	// another sinkrill owns the port.
	got := describeListenError(Options{Bind: "127.0.0.1", Port: 1}, syscall.EADDRINUSE)
	if !strings.Contains(got.Error(), "--port 0") {
		t.Errorf("expected the --port 0 hint:\n%v", got)
	}
	if strings.Contains(got.Error(), "already running") {
		t.Errorf("should not claim an instance is running:\n%v", got)
	}
}

func TestDescribeListenErrorPassesThroughOtherErrors(t *testing.T) {
	base := errors.New("boom")
	if got := describeListenError(Options{Port: 1}, base); got != base {
		t.Fatalf("unrelated error should pass through, got %v", got)
	}
}

func TestCheckPortFreeReportsRunningInstance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Status{
			Station: "homedesktop streaming audio",
			Device:  &DeviceStatus{Name: "Den", IP: "192.168.0.188"},
		})
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	err = checkPortFree(Options{Bind: "127.0.0.1", Port: port})
	if err == nil {
		t.Fatal("expected the preflight to fail on a busy port")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected the running-instance message:\n%v", err)
	}
}

func TestCheckPortFreeOnFreePort(t *testing.T) {
	// Grab a free port, release it, then preflight it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	if err := checkPortFree(Options{Bind: "127.0.0.1", Port: port}); err != nil {
		t.Fatalf("expected a free port, got %v", err)
	}
}
