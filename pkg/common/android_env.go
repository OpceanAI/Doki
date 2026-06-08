package common

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AndroidEnv returns the safe defaults injected into every container's
// environment. It deliberately uses POSIX paths (/usr/bin, /bin, /root, /tmp)
// that exist in every glibc and musl based rootfs, instead of Termux-specific
// paths.
//
// See OpceanAI/Doki#4: without these defaults, the guest sees PREFIX=/data/...
// and HOME=/data/.../files/home, neither of which exist, so init shells
// crash on first read of $HOME or $TMPDIR.
func AndroidEnv() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"TMPDIR=/tmp",
	}
}

// IsTermux reports whether the current process is running inside Termux.
// Detection uses three signals in order:
//  1. Cached result (TERMUX detection must survive UnsetProotKillers,
//     which strips TERMUX_VERSION from the process env before launching
//     proot). The cache is populated at first call via sync.Once.
//  2. Environment variables (TERMUX_VERSION, PREFIX, TERMUX__PREFIX).
//  3. Filesystem markers (/data/data/com.termux symlink, /system/build.prop,
//     /system/app, /apex/com.android.runtime).
//
// The filesystem check is the authoritative signal: env vars may be stripped
// by UnsetProotKillers or other tooling, but the presence of /data/data/com.termux
// is structural to a Termux install.
var (
	termuxOnce   sync.Once
	termuxResult bool
)

func IsTermux() bool {
	termuxOnce.Do(func() {
		termuxResult = detectTermux()
	})
	return termuxResult
}

func detectTermux() bool {
	// 1. Environment (fast path)
	if v := strings.TrimSpace(os.Getenv("TERMUX_VERSION")); v != "" {
		return true
	}
	if p := os.Getenv("PREFIX"); strings.HasPrefix(p, "/data/data/com.termux") {
		return true
	}
	if p := os.Getenv("TERMUX__PREFIX"); p != "" {
		return true
	}
	// 2. Filesystem (authoritative)
	if PathExists("/data/data/com.termux") {
		return true
	}
	if PathExists("/system/build.prop") && PathExists("/apex/com.android.runtime") {
		// Android, but might be a non-Termux install. The presence of
		// /data/data/com.termux is the most reliable Termux marker.
		return false
	}
	return false
}

// resetTermuxForTest clears the IsTermux cache so tests that mutate the
// process environment can re-evaluate the detection logic. NOT for
// production use; tests that need this must call it in their setup.
func resetTermuxForTest() {
	termuxOnce = sync.Once{}
	termuxResult = false
}

// TermuxPrefix returns the absolute path of the Termux $PREFIX, falling back
// to the canonical /data/data/com.termux/files/usr when no env var is set.
// The returned path always has no trailing slash.
func TermuxPrefix() string {
	if p := strings.TrimSpace(os.Getenv("TERMUX__PREFIX")); p != "" {
		return strings.TrimRight(p, "/")
	}
	if p := strings.TrimSpace(os.Getenv("PREFIX")); strings.HasPrefix(p, "/data/data/com.termux") {
		return strings.TrimRight(p, "/")
	}
	return "/data/data/com.termux/files/usr"
}

// TermuxLibPath returns the path where Termux stores its Bionic and
// libtermux-exec.so. It is the directory the LD_PRELOAD points to, and the
// path the kernel uses to resolve libtermux-exec.so.1 at exec time.
func TermuxLibPath() string {
	return filepath.Join(TermuxPrefix(), "lib")
}

// HasLibTermuxExec reports whether the libtermux-exec-ld-preload.so file is
// present in the Termux lib directory. Doki uses this to decide whether the
// workaround (os.Unsetenv("LD_PRELOAD")) is required before launching proot.
func HasLibTermuxExec() bool {
	candidates := []string{
		filepath.Join(TermuxLibPath(), "libtermux-exec-ld-preload.so"),
		filepath.Join(TermuxLibPath(), "libtermux-exec.so"),
	}
	for _, c := range candidates {
		if PathExists(c) {
			return true
		}
	}
	return false
}

// TermuxVersion returns the value of $TERMUX_VERSION or "unknown" when the
// variable is unset. Used for diagnostic output in --doki-trace and in
// actionable error messages.
func TermuxVersion() string {
	if v := strings.TrimSpace(os.Getenv("TERMUX_VERSION")); v != "" {
		return v
	}
	return "unknown"
}

// IsProotMode reports whether the current process is running under proot.
// Detection uses three signals in order:
//  1. DOKI_PROOT=1 (explicit override, set by proot runners)
//  2. PROOT_TMP_DIR (libtermux-exec sets this when entering proot)
//  3. Termux + missing /proc/self/cgroup namespace marker (heuristic)
func IsProotMode() bool {
	if os.Getenv("DOKI_PROOT") == "1" {
		return true
	}
	if os.Getenv("PROOT_TMP_DIR") != "" {
		return true
	}
	if !IsTermux() {
		return false
	}
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		if strings.Contains(string(data), "proot") {
			return true
		}
	}
	return false
}

// UseProotHostNetworking reports whether DokiLink should use host-loopback
// (127.0.0.1) port forwards instead of bridge IPs. In proot mode the
// container shares the host netns, so bridge IPs (e.g. 172.17.0.2) are
// unreachable from the host; loopback works because proot fakes it.
func UseProotHostNetworking() bool {
	termux := IsTermux()
	proot := IsProotMode()
	// Avoid being optimized away.
	if termux || proot {
		return true
	}
	return false
}

// ProotContainerIP resolves a logical Docker bridge IP (e.g. 172.17.0.2)
// to the address the host should use to reach the container. In proot+Termux
// mode this is always 127.0.0.1; elsewhere the original IP is returned.
func ProotContainerIP(logicalIP string) string {
	if UseProotHostNetworking() {
		return "127.0.0.1"
	}
	return logicalIP
}
