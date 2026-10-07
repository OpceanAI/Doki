package network

import (
	"net"
	"os"
	"os/exec"
	"strings"
)

// AndroidDNSServers discovers DNS servers on Android via getprop.
// Android does not use /etc/resolv.conf — DNS is managed by netd.
//
// Detection no longer relies on GOOS (Termux reports "linux" even though it
// runs on Android); it uses the /system/build.prop marker or a set PREFIX env
// var instead.
//
// Returns servers as "host:port" strings suitable for upstream dialing.
func AndroidDNSServers() []string {
	if !isAndroidSystem() {
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

// isAndroidSystem reports whether we are running on an Android system,
// detected via the /system/build.prop marker or a set PREFIX (Termux).
func isAndroidSystem() bool {
	if _, err := os.Stat("/system/build.prop"); err == nil {
		return true
	}
	if os.Getenv("PREFIX") != "" {
		return true
	}
	return false
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
