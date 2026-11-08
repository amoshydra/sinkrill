package logx

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger writes timestamped, level-prefixed lines. Debug output is opt-in.
type Logger struct {
	mu      sync.Mutex
	out     io.Writer
	err     io.Writer
	verbose bool
}

func New(verbose bool) *Logger {
	return &Logger{out: os.Stdout, err: os.Stderr, verbose: verbose}
}

// NewSilent discards all output (used in tests).
func NewSilent() *Logger {
	return &Logger{out: io.Discard, err: io.Discard}
}

func (l *Logger) write(w io.Writer, level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(w, "%s %-5s %s\n", time.Now().Format("15:04:05"), level, fmt.Sprintf(format, args...))
}

func (l *Logger) Debugf(format string, args ...any) {
	if l.verbose {
		l.write(l.out, "DEBUG", format, args...)
	}
}

func (l *Logger) Infof(format string, args ...any) {
	l.write(l.out, "INFO", format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.write(l.err, "WARN", format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.write(l.err, "ERROR", format, args...)
}
