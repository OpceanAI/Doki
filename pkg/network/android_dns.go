package network

import (
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// AndroidDNSServers discovers DNS servers on Android via getprop.
// Android does not use /etc/resolv.conf — DNS is managed by netd.
// Returns servers as "host:port" strings suitable for upstream dialing.
func AndroidDNSServers() []string {
	if runtime.GOOS != "android" {
		return nil
	}
	var servers []string
	for i := 1; i <= 4; i++ {
		prop := getProp("net.dns" + itoa(i))
		if prop == "" {
			continue
		}
		ip := strings.TrimSpace(prop)
		if net.ParseIP(ip) != nil {
			servers = append(servers, net.JoinHostPort(ip, "53"))
		}
	}
	return servers
}

func getProp(name string) string {
	out, err := exec.Command("getprop", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
