package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amoshydra/sinkrill/internal/logx"
)

type ServerOptions struct {
	Port    int
	Bind    string
	Format  string // "wav" or "mp3"
	Bitrate int    // MP3 bitrate, kbps
	Station string
	Status  func() any
	Logger  *logx.Logger
}

type Server struct {
	opts ServerOptions
	hub  *Hub
	ln   net.Listener
	srv  *http.Server
	port int
}

func NewServer(opts ServerOptions, hub *Hub) *Server {
	return &Server{opts: opts, hub: hub}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleStream)

	ln, err := net.Listen("tcp", net.JoinHostPort(s.opts.Bind, strconv.Itoa(s.opts.Port)))
	if err != nil {
		return err
	}
	s.ln = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}

	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.opts.Logger.Errorf("HTTP server: %v", err)
		}
	}()

	s.opts.Logger.Infof(
		"stream server listening on http://%s:%d%s",
		s.opts.Bind, s.port, s.streamPath(),
	)
	return nil
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) streamPath() string {
	if s.opts.Format == "wav" {
		return "/stream.wav"
	}
	return "/stream.mp3"
}

func (s *Server) contentType() string {
	if s.opts.Format == "wav" {
		return "audio/wav"
	}
	return "audio/mpeg"
}

func (s *Server) bitrate() int {
	if s.opts.Format == "wav" {
		return WAVSampleRate * WAVChannels * WAVBits / 1000
	}
	return s.opts.Bitrate
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	active := s.streamPath()
	if r.URL.Path != active && r.URL.Path != "/stream" && r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	headers := map[string]string{
		"Content-Type":                s.contentType(),
		"Connection":                  "close",
		"Cache-Control":               "no-cache, no-store, must-revalidate",
		"Pragma":                      "no-cache",
		"icy-name":                    sanitizeHeader(s.opts.Station),
		"icy-br":                      strconv.Itoa(s.bitrate()),
		"icy-pub":                     "0",
		"Access-Control-Allow-Origin": "*",
	}

	if r.Method == http.MethodHead {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	conn, brw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		// Keep the kernel send buffer small (it is doubled by Linux). A large
		// buffer lets seconds of audio pile up while the speaker is buffering.
		_ = tcp.SetWriteBuffer(32 * 1024)
	}

	fmt.Fprint(brw, "HTTP/1.1 200 OK\r\n")
	for k, v := range headers {
		fmt.Fprintf(brw, "%s: %s\r\n", k, v)
	}
	fmt.Fprint(brw, "\r\n")
	if s.opts.Format == "wav" {
		if _, err := brw.Write(WAVHeader()); err != nil {
			_ = conn.Close()
			return
		}
	}
	if err := brw.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	s.hub.Add(conn, fmt.Sprintf("stream client %s", r.RemoteAddr))
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.opts.Status()); err != nil {
		s.opts.Logger.Debugf("status encode: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true}`+"\n")
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.hub.CloseAll()
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if value == "" {
		return "Sinkrill"
	}
	return value
}
