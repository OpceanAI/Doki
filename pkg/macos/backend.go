// Package macos provides macOS native virtualization.
package macos

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Backend interface {
	Name() string
	Available() bool
	MinVersion() string
	CreateVM(cfg *VMConfig) error
	StartVM(id string) error
	StopVM(id string, timeoutSec int) error
	DeleteVM(id string) error
	VMStatus(id string) (string, error)
	ShareHostDir(hostPath, guestPath, tag string, readOnly bool) error
	UnshareHostDir(tag string) error
	ForwardPort(hostPort, guestPort int, proto string) error
	RemoveForwardPort(hostPort int, proto string) error
	Stats(id string) (*VMStats, error)
}

type VMConfig struct {
	ID         string
	CPUs       int
	MemoryMB   int64
	DiskSizeGB int64
	DiskPath   string
	KernelPath string
	InitrdPath string
	Shares     []FSShare
	Ports      []PortForward
	Rosetta    bool
	Network    string
	Backend    string
}

type FSShare struct {
	HostPath  string
	GuestPath string
	ReadOnly  bool
	Tag       string
}

type PortForward struct {
	HostPort  int
	GuestPort int
	Protocol  string
	HostIP    string
}

type VMStats struct {
	CPUUsage    float64
	MemoryUsage int64
	MemoryTotal int64
	DiskRead    int64
	DiskWrite   int64
	NetRx       int64
	NetTx       int64
	State       string
}

type Capabilities struct {
	Version        string
	Arch           string
	IsTahoe        bool
	IsAppleSilicon bool
	ASIF           bool
	NFSv41         bool
	VZ             bool
	VirtioFS       bool
	Rosetta        bool
	Hypervisor     bool
	QEMU           bool
	Sandbox        bool
	BestBackend    string
}

func DetectCapabilities() *Capabilities {
	if runtime.GOOS != "darwin" {
		return &Capabilities{Arch: runtime.GOARCH}
	}

	caps := &Capabilities{
		Arch:           runtime.GOARCH,
		IsAppleSilicon: runtime.GOARCH == "arm64",
		Hypervisor:     checkHypervisor(),
	}

	major, minor, _, err := getMacOSVersion()
	if err != nil {
		// Could not detect version; default to safe minimum.
		caps.Version = "unknown"
	} else {
		caps.Version = fmt.Sprintf("%d.%d", major, minor)
	}

	if major >= 26 {
		caps.IsTahoe = true
		caps.ASIF = true
		caps.NFSv41 = true
	}

	// Virtualization.framework was introduced in macOS 11 (Big Sur).
	if major >= 11 {
		caps.VZ = true
		caps.VirtioFS = true
		caps.BestBackend = "vz"
	}

	if major >= 13 && runtime.GOARCH == "arm64" {
		caps.Rosetta = checkRosetta()
	}

	if _, err := exec.LookPath("qemu-system-aarch64"); err == nil {
		caps.QEMU = true
		if caps.BestBackend == "" {
			caps.BestBackend = "qemu"
		}
	} else if _, err := exec.LookPath("qemu-system-x86_64"); err == nil {
		caps.QEMU = true
		if caps.BestBackend == "" {
			caps.BestBackend = "qemu"
		}
	}

	if _, err := exec.LookPath("sandbox-exec"); err == nil {
		caps.Sandbox = true
		if caps.BestBackend == "" {
			caps.BestBackend = "sandbox"
		}
	}

	return caps
}

// getMacOSVersion returns the macOS version as (major, minor, patch).
// Returns an error if the version cannot be determined.
func getMacOSVersion() (int, int, int, error) {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("detect macOS version: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ".")
	if len(parts) == 0 {
		return 0, 0, 0, errors.New("empty macOS version")
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse major version %q: %w", parts[0], err)
	}
	minor := 0
	patch := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		patch, _ = strconv.Atoi(parts[2])
	}
	return major, minor, patch, nil
}

// checkHypervisor probes whether the Hypervisor.framework is available
// via sysctl kern.hv_support (macOS 10.15+). Returns false if the
// sysctl is unavailable or reports no support.
func checkHypervisor() bool {
	out, err := exec.Command("sysctl", "-n", "kern.hv_support").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}

// checkRosetta detects whether Rosetta 2 is available on Apple Silicon.
// Checks sysctl sysctl.proc_translated and the oahd runtime binary.
func checkRosetta() bool {
	// Check if the current process is translated (running under Rosetta).
	out, err := exec.Command("sysctl", "-n", "sysctl.proc_translated").Output()
	if err == nil && strings.TrimSpace(string(out)) == "1" {
		return true
	}
	// Check for the Rosetta runtime daemon.
	_, err = exec.LookPath("/Library/Apple/usr/share/rosetta/rosetta")
	return err == nil
}

// SelectBackend selects the best available backend based on detected
// capabilities. Returns (backend, error) — the error is non-nil if no
// backend is available.
func SelectBackend(caps *Capabilities) (Backend, error) {
	if caps == nil {
		return nil, errors.New("macos: nil capabilities")
	}
	if caps.VZ && caps.Hypervisor {
		return newVZBackend(), nil
	}
	if caps.QEMU {
		return newQEMUBackend(), nil
	}
	if caps.Sandbox {
		return newSandboxBackend(), nil
	}
	return nil, errors.New("macos: no virtualization backend available")
}

// DefaultVMImage returns the default VM image path for the given arch.
func DefaultVMImage(arch string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		homeDir = os.Getenv("HOME")
	}
	if homeDir == "" {
		homeDir = "."
	}
	switch arch {
	case "arm64":
		return homeDir + "/.doki/vm/ubuntu-arm64.img"
	case "amd64":
		return homeDir + "/.doki/vm/ubuntu-amd64.img"
	default:
		return homeDir + "/.doki/vm/ubuntu-" + arch + ".img"
	}
}
