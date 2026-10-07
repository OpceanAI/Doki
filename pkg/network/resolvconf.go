package network

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// ResolvConf holds parsed /etc/resolv.conf content.
// Nameservers are stored WITHOUT port numbers (resolv.conf format).
// Use NameserverList() to get host:port format for dialling.
type ResolvConf struct {
	Nameservers []string
	Search      []string
	Options     []string
	SortList    []string
}

// ParseResolvConf parses a resolv.conf file from the given path.
// Supports: nameserver, search, options, sortlist directives.
// Lines starting with # or ; are comments.
// Nameservers are stored as plain IPs (no port) for safe resolv.conf output.
func ParseResolvConf(path string) (*ResolvConf, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	rc := &ResolvConf{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		keyword := strings.ToLower(fields[0])
		switch keyword {
		case "nameserver":
			ns := fields[1]
			// Strip port if present — resolv.conf doesn't use ports.
			if host, _, err := net.SplitHostPort(ns); err == nil {
				ns = host
			}
			rc.Nameservers = append(rc.Nameservers, ns)
		case "search":
			rc.Search = append(rc.Search, fields[1:]...)
		case "options":
			rc.Options = append(rc.Options, fields[1:]...)
		case "sortlist":
			rc.SortList = append(rc.SortList, fields[1:]...)
		}
	}
	return rc, scanner.Err()
}

// HostResolvConf reads the host's DNS configuration in a platform-aware order:
//  1. Android DNS via getprop (net.dns1..4), when on an Android system.
//  2. $PREFIX/etc/resolv.conf (Termux layout), when PREFIX is set.
//  3. /etc/resolv.conf.
//  4. Public DNS (8.8.8.8) as a last resort.
func HostResolvConf() *ResolvConf {
	// 1. Android DNS via getprop.
	if servers := AndroidDNSServers(); len(servers) > 0 {
		nameservers := make([]string, 0, len(servers))
		for _, s := range servers {
			if host, _, err := net.SplitHostPort(s); err == nil {
				nameservers = append(nameservers, host)
			} else {
				nameservers = append(nameservers, s)
			}
		}
		return &ResolvConf{Nameservers: nameservers}
	}
	// 2. $PREFIX/etc/resolv.conf (Termux layout).
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		if rc, err := ParseResolvConf(filepath.Join(prefix, "etc", "resolv.conf")); err == nil && len(rc.Nameservers) > 0 {
			return rc
		}
	}
	// 3. /etc/resolv.conf.
	if rc, err := ParseResolvConf("/etc/resolv.conf"); err == nil && len(rc.Nameservers) > 0 {
		return rc
	}
	// 4. Public DNS fallback.
	return &ResolvConf{
		Nameservers: []string{"8.8.8.8", "8.8.4.4"},
	}
}

// NameserverList returns the nameservers as host:port strings suitable for dialling.
// Appends ":53" to nameservers without a port.
//
// BUG-12 fix: the previous code used strings.Contains(ns, ":") to detect
// whether a port was present. IPv6 addresses contain colons
// (e.g. "2001:4860:4860::8888") and were incorrectly treated as already
// having a port. Use net.SplitHostPort to correctly distinguish bare IPs
// from host:port pairs.
func (rc *ResolvConf) NameserverList() []string {
	if len(rc.Nameservers) == 0 {
		return []string{"8.8.8.8:53", "8.8.4.4:53"}
	}
	list := make([]string, len(rc.Nameservers))
	for i, ns := range rc.Nameservers {
		if _, _, err := net.SplitHostPort(ns); err == nil {
			// Already has a port — use as-is.
			list[i] = ns
		} else {
			// Bare IP (v4 or v6) — append default port.
			list[i] = net.JoinHostPort(ns, "53")
		}
	}
	return list
}
