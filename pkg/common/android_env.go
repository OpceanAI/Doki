package common

import (
	"os"
	"path/filepath"
	"strings"
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
// Detection is purely environment-based so it works for both the Termux app
// and derivative builds (F-Droid, GitHub, NDK variants).
func IsTermux() bool {
	if v := strings.TrimSpace(os.Getenv("TERMUX_VERSION")); v != "" {
		return true
	}
	if p := os.Getenv("PREFIX"); strings.HasPrefix(p, "/data/data/com.termux") {
		return true
	}
	if p := os.Getenv("TERMUX__PREFIX"); p != "" {
		return true
	}
	return false
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
