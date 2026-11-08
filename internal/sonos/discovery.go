package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/amoshydra/thrum/internal/logx"
	"github.com/amoshydra/thrum/internal/netutil"
)

const (
	ssdpAddress   = "239.255.255.250:1900"
	ssdpBroadcast = "255.255.255.255:1900"
	ssdpPayload   = "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n" +
		"\r\n"
	sonosUpnpPort = 1400
)

type Device struct {
	IP    string `json:"ip"`
	Name  string `json:"name"`
	Model string `json:"model"`
	UUID  string `json:"uuid"`
}

func (d Device) String() string {
	return fmt.Sprintf("%s (%s)", d.Name, d.IP)
}

// Discover finds Sonos devices via SSDP, falling back to a TCP port scan when
// multicast is blocked (VPNs, AP isolation).
func Discover(ctx context.Context, timeout time.Duration, log *logx.Logger) ([]Device, error) {
	ips := ssdpDiscover(timeout, log)
	if len(ips) == 0 {
		log.Infof("SSDP found nothing; scanning the local network on port %d...", sonosUpnpPort)
		ips = tcpScan(ctx, 400*time.Millisecond, log)
	}

	var (
		mu      sync.Mutex
		devices []Device
		wg      sync.WaitGroup
	)
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			device, err := Describe(ctx, ip)
			if err != nil {
				log.Debugf("could not describe %s: %v", ip, err)
				return
			}
			mu.Lock()
			devices = append(devices, device)
			mu.Unlock()
		}(ip)
	}
	wg.Wait()

	sort.Slice(devices, func(i, j int) bool {
		return strings.ToLower(devices[i].Name) < strings.ToLower(devices[j].Name)
	})
	return devices, nil
}

// Resolve turns a room name, partial name, IP, or empty selector into a device.
func Resolve(ctx context.Context, selector string, timeout time.Duration, log *logx.Logger) (Device, error) {
	if ip := net.ParseIP(selector); ip != nil && ip.To4() != nil {
		if device, err := Describe(ctx, selector); err == nil {
			return device, nil
		}
		return Device{IP: selector, Name: selector}, nil
	}

	log.Infof("Searching for Sonos devices...")
	devices, err := Discover(ctx, timeout, log)
	if err != nil {
		return Device{}, err
	}
	if len(devices) == 0 {
		return Device{}, fmt.Errorf(
			"no Sonos devices found. Check that the speakers are on the same LAN, " +
				"or pass --device <ip>",
		)
	}

	if selector == "" {
		if len(devices) == 1 {
			return devices[0], nil
		}
		return Device{}, fmt.Errorf(
			"more than one Sonos device found:\n%s\npick one with --device <name|ip>",
			formatList(devices),
		)
	}

	query := strings.ToLower(selector)
	for _, device := range devices {
		if strings.ToLower(device.Name) == query {
			return device, nil
		}
	}
	for _, device := range devices {
		if strings.Contains(strings.ToLower(device.Name), query) {
			return device, nil
		}
	}
	return Device{}, fmt.Errorf("no device matched %q. found:\n%s", selector, formatList(devices))
}

func formatList(devices []Device) string {
	lines := make([]string, 0, len(devices))
	for _, device := range devices {
		lines = append(lines, fmt.Sprintf("  - %s (%s)", device.Name, device.IP))
	}
	return strings.Join(lines, "\n")
}

var descriptionClient = &http.Client{Timeout: 4 * time.Second}

type deviceDescription struct {
	Device struct {
		RoomName    string `xml:"roomName"`
		DisplayName string `xml:"displayName"`
		ModelName   string `xml:"modelName"`
		UDN         string `xml:"UDN"`
	} `xml:"device"`
}

// Describe fetches and parses a device's UPnP description.
func Describe(ctx context.Context, ip string) (Device, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s:%d/xml/device_description.xml", ip, sonosUpnpPort), nil)
	if err != nil {
		return Device{}, err
	}
	resp, err := descriptionClient.Do(req)
	if err != nil {
		return Device{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Device{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Device{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var doc deviceDescription
	if err := xml.Unmarshal(data, &doc); err != nil {
		return Device{}, err
	}
	if doc.Device.UDN == "" && doc.Device.RoomName == "" {
		return Device{}, fmt.Errorf("not a Sonos device")
	}

	name := doc.Device.RoomName
	if name == "" {
		name = doc.Device.DisplayName
	}
	if name == "" {
		name = ip
	}
	return Device{
		IP:    ip,
		Name:  name,
		Model: doc.Device.ModelName,
		UUID:  strings.TrimPrefix(doc.Device.UDN, "uuid:"),
	}, nil
}

func ssdpDiscover(timeout time.Duration, log *logx.Logger) []string {
	lanIP := netutil.LANIP()
	var laddr *net.UDPAddr
	if lanIP != "" {
		laddr = &net.UDPAddr{IP: net.ParseIP(lanIP)}
	}

	conn, err := net.ListenUDP("udp4", laddr)
	if err != nil && laddr != nil {
		conn, err = net.ListenUDP("udp4", &net.UDPAddr{})
	}
	if err != nil {
		log.Debugf("SSDP listen failed: %v", err)
		return nil
	}
	defer conn.Close()

	payload := []byte(ssdpPayload)
	for _, dest := range []string{ssdpAddress, ssdpBroadcast} {
		addr, err := net.ResolveUDPAddr("udp4", dest)
		if err != nil {
			continue
		}
		if _, err := conn.WriteToUDP(payload, addr); err != nil {
			log.Debugf("SSDP send to %s failed: %v", dest, err)
		}
	}

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	seen := map[string]bool{}
	buf := make([]byte, 2048)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		text := string(buf[:n])
		if strings.Contains(text, "Sonos") || strings.Contains(strings.ToLower(text), "rincon") {
			seen[remote.IP.String()] = true
		}
	}

	ips := make([]string, 0, len(seen))
	for ip := range seen {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips
}

func tcpScan(ctx context.Context, perHost time.Duration, log *logx.Logger) []string {
	bases := netutil.SubnetBases()
	if len(bases) == 0 {
		return nil
	}

	const workers = 64
	sem := make(chan struct{}, workers)
	var (
		mu    sync.Mutex
		found []string
		wg    sync.WaitGroup
	)

	for _, base := range bases {
		for host := 1; host <= 254; host++ {
			ip := fmt.Sprintf("%s.%d", base, host)
			select {
			case <-ctx.Done():
				wg.Wait()
				return found
			default:
			}

			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, fmt.Sprint(sonosUpnpPort)), perHost)
				if err != nil {
					return
				}
				_ = conn.Close()
				mu.Lock()
				found = append(found, ip)
				mu.Unlock()
			}(ip)
		}
	}
	wg.Wait()
	sort.Strings(found)
	log.Debugf("scan found %d candidate(s): %s", len(found), strings.Join(found, ", "))
	return found
}
