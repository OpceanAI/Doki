package proot

import (
	"os/exec"
	"regexp"
	"strings"
)

// ProotInfo describes a resolved proot binary and the version string it
// reports via "proot --version". The version is parsed from the first line
// of the command output and is "unknown" when the binary does not answer.
type ProotInfo struct {
	Binary  string
	Version string
}

// versionRe matches the common proot version banners we have seen across
// distributions: "proot version 5.1.107", "5.1.107.76", "proot v5.4.0".
var versionRe = regexp.MustCompile(`(\d+\.\d+\.\d+(?:\.\d+)?)`)

// Version invokes "<bin> --version" (or "-V") and returns the first matching
// numeric version token. The returned struct never has nil fields: Version is
// "unknown" on any failure so callers can render diagnostics without an
// extra error check.
func Version(bin string) ProotInfo {
	info := ProotInfo{Binary: bin, Version: "unknown"}
	if bin == "" {
		return info
	}
	for _, arg := range []string{"--version", "-V", "-v"} {
		out, err := exec.Command(bin, arg).Output()
		if err != nil {
			continue
		}
		first := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		if m := versionRe.FindStringSubmatch(first); m != nil {
			info.Version = m[1]
			return info
		}
		// Fall through and try the next flag.
	}
	return info
}
