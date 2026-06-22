// Package deps - dependency checker for Doki system and Go dependencies.
package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SystemDep describes a system-level dependency that Doki can use.
type SystemDep struct {
	Name        string
	Required    bool
	Optional    bool
	Check       func() (installed bool, version string)
	InstallHint string
}

// SystemDepResult is the outcome of checking a single SystemDep.
type SystemDepResult struct {
	Name        string
	Installed   bool
	Version     string
	Required    bool
	Optional    bool
	InstallHint string
	Path        string
}

// GoDepResult is a single direct Go module dependency.
type GoDepResult struct {
	Path    string
	Version string
}

// isAndroid reports whether we are running on Android/Termux, where proot is
// the preferred execution mode because user namespaces are unavailable.
func isAndroid() bool {
	if runtime.GOOS == "android" {
		return true
	}
	if _, err := os.Stat("/data/data/com.termux"); err == nil {
		return true
	}
	if _, err := os.Stat("/system/bin/adb"); err == nil {
		return true
	}
	return false
}

// lookPathVersion resolves the binary on $PATH and, best-effort, captures the
// first non-empty line of its version output. Multiple common version flags are
// tried (--version, -V, -v, version). Returns ("", "") when the binary is not
// on $PATH; returns (path, "") when it is found but emits no recognisable
// version string.
func lookPathVersion(bin string) (path, version string) {
	p, err := exec.LookPath(bin)
	if err != nil {
		return "", ""
	}
	for _, args := range [][]string{{"--version"}, {"-V"}, {"-v"}, {"version"}} {
		out, err := exec.Command(bin, args...).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return p, line
			}
		}
	}
	return p, ""
}

// makeCheck returns a Check closure for the given binary name.
func makeCheck(bin string) func() (bool, string) {
	return func() (bool, string) {
		p, ver := lookPathVersion(bin)
		return p != "", ver
	}
}

// systemDeps builds the canonical list of system dependencies Doki cares about.
func systemDeps() []SystemDep {
	android := isAndroid()
	return []SystemDep{
		{
			Name:        "proot",
			Required:    android,
			Optional:    !android,
			Check:       makeCheck("proot"),
			InstallHint: "pkg install proot  (Termux)  |  apt install proot",
		},
		{Name: "qemu-system-aarch64", Optional: true, Check: makeCheck("qemu-system-aarch64"), InstallHint: "pkg install qemu-system-aarch64  |  apt install qemu-system-arm"},
		{Name: "qemu-system-x86_64", Optional: true, Check: makeCheck("qemu-system-x86_64"), InstallHint: "pkg install qemu-system-x86_64  |  apt install qemu-system-x86"},
		{Name: "iptables", Optional: true, Check: makeCheck("iptables"), InstallHint: "pkg install iptables  |  apt install iptables"},
		{Name: "nft", Optional: true, Check: makeCheck("nft"), InstallHint: "pkg install nftables  |  apt install nftables"},
		{Name: "socat", Optional: true, Check: makeCheck("socat"), InstallHint: "pkg install socat  |  apt install socat"},
		{Name: "fuse-overlayfs", Optional: true, Check: makeCheck("fuse-overlayfs"), InstallHint: "pkg install fuse-overlayfs  |  apt install fuse-overlayfs"},
		{Name: "sqlite3", Optional: true, Check: makeCheck("sqlite3"), InstallHint: "pkg install sqlite  |  apt install sqlite3"},
		{Name: "slirp4netns", Optional: true, Check: makeCheck("slirp4netns"), InstallHint: "pkg install slirp4netns  |  apt install slirp4netns"},
		{Name: "pasta", Optional: true, Check: makeCheck("pasta"), InstallHint: "pkg install passt  |  apt install passt"},
	}
}

// CheckSystemDeps inspects the host for tools that Doki can use and returns a
// result per dependency, in the canonical order.
func CheckSystemDeps() []SystemDepResult {
	specs := systemDeps()
	results := make([]SystemDepResult, 0, len(specs))
	for _, d := range specs {
		installed, version := false, ""
		if d.Check != nil {
			installed, version = d.Check()
		}
		results = append(results, SystemDepResult{
			Name:        d.Name,
			Installed:   installed,
			Version:     version,
			Required:    d.Required,
			Optional:    d.Optional,
			InstallHint: d.InstallHint,
		})
	}
	return results
}

// CheckGoDeps reads the nearest go.mod (walking up from the current working
// directory) and returns the direct (non-indirect) module requirements.
func CheckGoDeps() ([]GoDepResult, error) {
	modPath, err := findGoMod()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(modPath)
	if err != nil {
		return nil, err
	}
	return parseGoModDirect(data), nil
}

// findGoMod walks up from the current directory looking for a go.mod file.
func findGoMod() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found (walked up from %s)", dir)
		}
		dir = parent
	}
}

// parseGoModDirect extracts the direct (non-indirect) require statements from a
// go.mod file. It is a minimal hand-rolled parser that avoids pulling in
// golang.org/x/mod/modfile as a direct dependency.
func parseGoModDirect(data []byte) []GoDepResult {
	lines := strings.Split(string(data), "\n")
	var results []GoDepResult
	inRequireBlock := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "require ") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "require "))
			if strings.HasPrefix(rest, "(") {
				inRequireBlock = true
				continue
			}
			if dep, ok := parseRequireLine(rest); ok {
				results = append(results, dep)
			}
			continue
		}
		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}
			if dep, ok := parseRequireLine(line); ok {
				results = append(results, dep)
			}
		}
	}
	return results
}

// parseRequireLine parses a single require entry like
// "github.com/foo/bar v1.2.3 // indirect" and returns false for indirect deps.
func parseRequireLine(line string) (GoDepResult, bool) {
	if i := strings.Index(line, "//"); i >= 0 {
		comment := strings.TrimSpace(line[i+2:])
		line = strings.TrimSpace(line[:i])
		if strings.Contains(comment, "indirect") {
			return GoDepResult{}, false
		}
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return GoDepResult{}, false
	}
	return GoDepResult{Path: fields[0], Version: fields[1]}, true
}

// DetectPackageManager returns the name of the first supported package manager
// found on $PATH. Supported managers are: pkg (Termux), apt/apt-get (Debian),
// apk (Alpine), pacman (Arch), brew (macOS), dnf/yum (RHEL), zypper (SUSE).
// Returns the empty string when none are found.
func DetectPackageManager() string {
	candidates := []struct {
		bin  string
		name string
	}{
		{"pkg", "pkg"},
		{"apt-get", "apt"},
		{"apt", "apt"},
		{"apk", "apk"},
		{"pacman", "pacman"},
		{"brew", "brew"},
		{"dnf", "dnf"},
		{"yum", "yum"},
		{"zypper", "zypper"},
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c.bin); err == nil {
			return c.name
		}
	}
	return ""
}

// pkgNameMap translates a binary/dependency name into the native package name
// for each package manager, where they differ.
var pkgNameMap = map[string]map[string]string{
	"apt": {
		"nft": "nftables", "sqlite3": "sqlite3", "pasta": "passt",
		"slirp4netns": "slirp4netns", "fuse-overlayfs": "fuse-overlayfs",
		"proot": "proot", "socat": "socat", "iptables": "iptables",
		"qemu-system-aarch64": "qemu-system-arm", "qemu-system-x86_64": "qemu-system-x86",
	},
	"pkg": {
		"nft": "nftables", "sqlite3": "sqlite", "pasta": "passt",
		"slirp4netns": "slirp4netns", "fuse-overlayfs": "fuse-overlayfs",
		"proot": "proot", "socat": "socat", "iptables": "iptables",
		"qemu-system-aarch64": "qemu-system-aarch64", "qemu-system-x86_64": "qemu-system-x86_64",
	},
	"pacman": {
		"nft": "nftables", "sqlite3": "sqlite", "pasta": "passt",
		"slirp4netns": "slirp4netns", "fuse-overlayfs": "fuse-overlayfs",
		"proot": "proot", "socat": "socat", "iptables": "iptables",
		"qemu-system-aarch64": "qemu-system-aarch64", "qemu-system-x86_64": "qemu-system-x86_64",
	},
	"brew": {
		"nft": "nftables", "sqlite3": "sqlite", "pasta": "passt",
		"slirp4netns": "slirp4netns", "fuse-overlayfs": "fuse-overlayfs",
		"proot": "proot", "socat": "socat", "iptables": "iptables",
	},
	"apk": {
		"nft": "nftables", "sqlite3": "sqlite", "pasta": "passt",
		"slirp4netns": "slirp4netns", "fuse-overlayfs": "fuse-overlayfs",
		"proot": "proot", "socat": "socat", "iptables": "iptables",
		"qemu-system-aarch64": "qemu-system-aarch64", "qemu-system-x86_64": "qemu-system-x86_64",
	},
	"dnf": {
		"nft": "nftables", "sqlite3": "sqlite", "pasta": "passt",
		"slirp4netns": "slirp4netns", "fuse-overlayfs": "fuse-overlayfs",
		"proot": "proot", "socat": "socat", "iptables": "iptables",
		"qemu-system-aarch64": "qemu-system-aarch64", "qemu-system-x86_64": "qemu-system-x86_64",
	},
	"yum": {
		"nft": "nftables", "sqlite3": "sqlite", "pasta": "passt",
		"slirp4netns": "slirp4netns", "fuse-overlayfs": "fuse-overlayfs",
		"proot": "proot", "socat": "socat", "iptables": "iptables",
	},
	"zypper": {
		"nft": "nftables", "sqlite3": "sqlite", "pasta": "passt",
		"slirp4netns": "slirp4netns", "fuse-overlayfs": "fuse-overlayfs",
		"proot": "proot", "socat": "socat", "iptables": "iptables",
		"qemu-system-aarch64": "qemu-system-aarch64", "qemu-system-x86_64": "qemu-system-x86_64",
	},
}

// resolvePkgName maps a dependency name to the native package name for the given
// package manager, falling back to the original name when no mapping exists.
func resolvePkgName(pm, name string) string {
	if m, ok := pkgNameMap[pm]; ok {
		if n, ok := m[name]; ok {
			return n
		}
	}
	return name
}

// installCmd builds the exec.Cmd that installs pkgName via the given package
// manager.
func installCmd(pm, pkgName string) *exec.Cmd {
	switch pm {
	case "apt":
		return exec.Command("apt-get", "install", "-y", pkgName)
	case "pkg":
		return exec.Command("pkg", "install", "-y", pkgName)
	case "pacman":
		return exec.Command("pacman", "-S", "--noconfirm", pkgName)
	case "brew":
		return exec.Command("brew", "install", pkgName)
	case "apk":
		return exec.Command("apk", "add", pkgName)
	case "dnf":
		return exec.Command("dnf", "install", "-y", pkgName)
	case "yum":
		return exec.Command("yum", "install", "-y", pkgName)
	case "zypper":
		return exec.Command("zypper", "install", "-y", pkgName)
	default:
		return nil
	}
}

// InstallDep uses the detected package manager to install the named dependency.
// It is best-effort: the native package name is resolved via pkgNameMap when the
// binary name differs from the package name (e.g. nft -> nftables). The command
// inherits the current process stdio so the user sees progress and can respond
// to prompts.
func InstallDep(name string) error {
	pm := DetectPackageManager()
	if pm == "" {
		return fmt.Errorf("no supported package manager detected (looked for pkg, apt, apk, pacman, brew, dnf, yum, zypper)")
	}
	pkgName := resolvePkgName(pm, name)
	cmd := installCmd(pm, pkgName)
	if cmd == nil {
		return fmt.Errorf("unsupported package manager: %s", pm)
	}
	fmt.Printf("Installing %s via %s (package: %s)...\n", name, pm, pkgName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
