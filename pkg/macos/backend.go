package macos

import (
	"fmt"
	"runtime"
	"os/exec"
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
		Hypervisor:     true,
	}

	major, minor, _ := getMacOSVersion()
	caps.Version = fmt.Sprintf("%d.%d", major, minor)

	if major >= 26 {
		caps.IsTahoe = true
		caps.ASIF = true
		caps.NFSv41 = true
	}

	if major >= 12 {
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

func getMacOSVersion() (int, int, int) {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return 0, 0, 0
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ".")
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	patch := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		patch, _ = strconv.Atoi(parts[2])
	}
	return major, minor, patch
}

func checkRosetta() bool {
	out, err := exec.Command("sysctl", "-n", "sysctl.proc_translated", "1").Output()
	if err != nil {
		_, err = exec.LookPath("/usr/libexec/rosetta")
		return err == nil
	}
	return strings.TrimSpace(string(out)) == "1"
}

func SelectBackend(caps *Capabilities) Backend {
	if caps.VZ {
		return NewVZBackend()
	}
	if caps.QEMU {
		return NewQEMUBackend()
	}
	if caps.Sandbox {
		return NewSandboxBackend()
	}
	return nil
}

func DefaultVMImage(arch string) string {
	home, _ := exec.Command("sh", "-c", "echo $HOME").Output()
	homeDir := strings.TrimSpace(string(home))
	if homeDir == "" {
		homeDir = "~"
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
