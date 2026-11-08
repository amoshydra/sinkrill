package virtualsink

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/amoshydra/thrum/internal/logx"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type Sink struct {
	Name          string
	MonitorSource string
	// Created is true when this process created the sink and should remove it.
	Created bool

	log *logx.Logger
}

// Ensure creates a null sink (if needed) so it appears in the desktop audio
// selector, and returns the monitor source to capture.
func Ensure(name string, log *logx.Logger) (*Sink, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("pactl"); err != nil {
		return nil, fmt.Errorf("pactl not found; install pulseaudio-utils/libpulse to use --virtual-sink")
	}

	exists, err := sinkExists(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		_, err = pactl(
			"load-module", "module-null-sink",
			"sink_name="+name,
			"sink_properties=device.description=Thrum",
		)
		if err != nil {
			return nil, err
		}
		if err := waitForMonitor(name, 2*time.Second); err != nil {
			log.Warnf("output device created but its monitor is not visible yet: %v", err)
		}
	}

	return &Sink{
		Name:          name,
		MonitorSource: name + ".monitor",
		Created:       !exists,
		log:           log,
	}, nil
}

func validateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid sink name %q: use letters, digits, dot, dash or underscore", name)
	}
	return nil
}

func (s *Sink) Dispose() {
	if !s.Created {
		return
	}

	out, err := pactl("list", "short", "modules")
	if err != nil {
		s.log.Debugf("could not list pactl modules: %v", err)
		return
	}

	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 || !strings.Contains(fields[2], "sink_name="+s.Name) {
			continue
		}
		if _, err := pactl("unload-module", fields[0]); err != nil {
			s.log.Debugf("could not unload virtual sink: %v", err)
			return
		}
		s.log.Infof("removed virtual output %q", s.Name)
		return
	}
}

func waitForMonitor(name string, timeout time.Duration) error {
	source := name + ".monitor"
	deadline := time.Now().Add(timeout)
	for {
		out, err := pactl("list", "short", "sources")
		if err == nil {
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Split(line, "\t")
				if len(fields) > 1 && fields[1] == source {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("source %q did not appear", source)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func sinkExists(name string) (bool, error) {
	out, err := pactl("list", "short", "sinks")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) > 1 && fields[1] == name {
			return true, nil
		}
	}
	return false, nil
}

func pactl(args ...string) (string, error) {
	out, err := exec.Command("pactl", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("pactl %s: %s", args[0], strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
