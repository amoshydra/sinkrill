package netutil

import (
	"net"
	"strings"
)

// Interface prefixes we never want to advertise or scan.
var ignoredPrefixes = []string{"lo", "wg", "tun", "docker", "br-", "veth", "virbr", "podman", "cni"}

func usableInterface(name string) bool {
	for _, prefix := range ignoredPrefixes {
		if name == prefix || strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

// LANIP returns the first non-internal IPv4 address on a "real" interface.
func LANIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || !usableInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return ""
}

// SubnetBases returns the /24 networks this machine is attached to, as
// "192.168.0" style prefixes. Used for the TCP discovery fallback.
func SubnetBases() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	var bases []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || !usableInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ones, _ := ipnet.Mask.Size()
			if ones < 24 {
				continue
			}
			base := net.IPv4(ip[0], ip[1], ip[2], 0).String()
			base = strings.TrimSuffix(base, ".0")
			if !seen[base] {
				seen[base] = true
				bases = append(bases, base)
			}
		}
	}
	return bases
}
