package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/amoshydra/sinkrill/internal/logx"
	"github.com/amoshydra/sinkrill/internal/sonos"
)

type Options struct {
	Port        int
	Device      string
	VirtualSink string
}

type deviceStatus struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type virtualSinkStatus struct {
	Name        string `json:"name"`
	CreatedByUs bool   `json:"createdByUs"`
}

type captureStatus struct {
	Running    bool   `json:"running"`
	Restarts   int    `json:"restarts"`
	UptimeMS   int64  `json:"uptimeMs"`
	LastDataMS int64  `json:"lastDataMs"`
	Stalled    bool   `json:"stalled"`
	LastError  string `json:"lastError"`
}

type serverStatus struct {
	Station       string             `json:"station"`
	Source        string             `json:"source"`
	Format        string             `json:"format"`
	Bitrate       int                `json:"bitrate"`
	URI           string             `json:"uri"`
	Device        *deviceStatus      `json:"device"`
	VirtualSink   *virtualSinkStatus `json:"virtualSink"`
	Capture       captureStatus      `json:"capture"`
	Listeners     int                `json:"listeners"`
	QueuedBytes   int64              `json:"queuedBytes"`
	DroppedChunks int64              `json:"droppedChunks"`
}

var volumeRe = regexp.MustCompile(`(\d+)%`)

// Run prints a diagnostic checklist and returns a process exit code.
func Run(ctx context.Context, opts Options, log *logx.Logger) int {
	if opts.VirtualSink == "" {
		opts.VirtualSink = "sinkrill"
	}

	fmt.Println("Sinkrill doctor")
	fmt.Println()

	fails, warns := 0, 0
	ok := func(format string, args ...any) {
		fmt.Printf("  [ok]   "+format+"\n", args...)
	}
	warn := func(format string, args ...any) {
		warns++
		fmt.Printf("  [warn] "+format+"\n", args...)
	}
	fail := func(format string, args ...any) {
		fails++
		fmt.Printf("  [fail] "+format+"\n", args...)
	}
	hint := func(format string, args ...any) {
		fmt.Printf("         "+format+"\n", args...)
	}

	status, err := fetchStatus(opts.Port)
	sinkName := opts.VirtualSink
	sonosIP := ""
	sonosName := ""

	if err != nil {
		fail("server on port %d is not responding", opts.Port)
		hint("start it: systemctl --user start sinkrill")
		hint("or:       sinkrill --device \"Living Room\" --virtual-sink")
	} else {
		ok("server: %s, %s, %d listener(s)", status.Station, status.Format, status.Listeners)
		if status.VirtualSink != nil && status.VirtualSink.Name != "" {
			sinkName = status.VirtualSink.Name
		}
		if status.Device != nil {
			sonosIP = status.Device.IP
			sonosName = status.Device.Name
		}

		if status.Format != "wav" {
			warn("format is %q; WAV has much lower latency", status.Format)
			hint("run sinkrill with: --format wav")
		}

		switch {
		case !status.Capture.Running:
			fail("capture is not running")
			hint("restart: systemctl --user restart sinkrill")
		case status.Capture.Stalled:
			fail("capture is stalled (no bytes for %.1f s)", float64(status.Capture.LastDataMS)/1000)
			hint("restart: systemctl --user restart sinkrill")
			hint("the 5 s watchdog should recover this automatically")
		default:
			ok("capture: %s, last data %d ms ago", status.Source, status.Capture.LastDataMS)
		}
		if status.Capture.LastError != "" {
			warn("capture reported: %s", status.Capture.LastError)
		}
		if status.Capture.Restarts > 0 {
			warn("capture restarted %d time(s) this run", status.Capture.Restarts)
		}

		if status.Listeners == 0 {
			warn("stream: no speaker is connected")
			hint("retune: sinkrill stop \"%s\" && sinkrill \"%s\" --virtual-sink", sonosName, sonosName)
		} else {
			ok("stream: %d listener(s), queue %d bytes", status.Listeners, status.QueuedBytes)
		}
		if status.QueuedBytes > 0 {
			warn("stream: %d bytes queued (%d chunks dropped)", status.QueuedBytes, status.DroppedChunks)
		}
	}

	for _, bin := range []string{"parec", "pactl"} {
		if _, err := exec.LookPath(bin); err != nil {
			fail("%s not found on PATH", bin)
			hint("install libpulse (Arch) or pulseaudio-utils (Debian)")
		} else {
			ok("%s found", bin)
		}
	}
	if status != nil && status.Format == "mp3" {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			fail("ffmpeg not found (required for MP3 mode)")
		} else {
			ok("ffmpeg found")
		}
	}

	if _, err := exec.LookPath("pactl"); err == nil {
		if !sinkExists(sinkName) {
			fail("output: %q sink not found", sinkName)
			hint("start sinkrill with --virtual-sink (or create the sink manually)")
		} else {
			current := defaultSink()
			if current == sinkName {
				ok("output: %q is the default sink", sinkName)
			} else {
				warn("output: %q is not the default sink (current: %s)", sinkName, current)
				hint("select it: pactl set-default-sink %s", sinkName)
			}

			muted, _ := sinkMuted(sinkName)
			volume, volErr := sinkVolume(sinkName)
			switch {
			case muted:
				warn("output: %q is muted", sinkName)
				hint("unmute: pactl set-sink-mute %s no", sinkName)
			case volErr == nil && volume < 50:
				warn("output: volume is %d%%", volume)
				hint("raise it: pactl set-sink-volume %s 100%%", sinkName)
			case volErr == nil:
				ok("output: volume %d%%, unmuted", volume)
			}
		}
	}

	if sonosIP == "" && opts.Device != "" {
		if resolved, err := sonos.Resolve(ctx, opts.Device, 2*time.Second, log); err == nil {
			sonosIP = resolved.IP
			sonosName = resolved.Name
		}
	}

	if sonosIP == "" {
		warn("sonos: no device in status (was Sinkrill started with --no-tune?)")
	} else {
		client := sonos.NewClient(sonosIP)
		state, err := client.TransportState(ctx)
		if err != nil {
			warn("sonos: could not query %s: %v", sonosIP, err)
		} else {
			volume, _ := client.Volume(ctx)
			muted, _ := client.Muted(ctx)
			if muted || volume == 0 {
				warn("sonos: %s is muted or at volume 0", sonosName)
				hint("raise the volume in the Sonos app")
			}
			if strings.EqualFold(state, "STOPPED") {
				warn("sonos: %s is stopped", sonosName)
				hint("retune: sinkrill \"%s\" --virtual-sink", sonosName)
			} else {
				ok("sonos: %s is %s, volume %d", sonosName, strings.ToLower(state), volume)
			}
		}
	}

	fmt.Println()
	if fails > 0 {
		fmt.Printf("%d issue(s) found.\n", fails)
		return 1
	}
	if warns > 0 {
		fmt.Printf("No failures, %d warning(s).\n", warns)
		return 0
	}
	fmt.Println("Everything looks good.")
	fmt.Printf("Test the output: ffmpeg -f lavfi -i \"sine=frequency=440:duration=2\" -ar 48000 -ac 2 /tmp/sinkrill-tone.wav && paplay --device=%s /tmp/sinkrill-tone.wav\n", sinkName)
	return 0
}

func fetchStatus(port int) (*serverStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/status", port), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var status serverStatus
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

func pactl(args ...string) (string, error) {
	out, err := exec.Command("pactl", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func sinkExists(name string) bool {
	out, err := pactl("list", "short", "sinks")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) > 1 && fields[1] == name {
			return true
		}
	}
	return false
}

func defaultSink() string {
	out, err := pactl("info")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Default Sink:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Default Sink:"))
		}
	}
	return ""
}

func sinkVolume(name string) (int, error) {
	out, err := pactl("get-sink-volume", name)
	if err != nil {
		return 0, err
	}
	match := volumeRe.FindStringSubmatch(out)
	if match == nil {
		return 0, fmt.Errorf("could not parse volume")
	}
	return strconv.Atoi(match[1])
}

func sinkMuted(name string) (bool, error) {
	out, err := pactl("get-sink-mute", name)
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(out), "yes"), nil
}
