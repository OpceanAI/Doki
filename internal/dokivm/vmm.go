package dokivm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// VMM abstracts any microVM backend (crosvm, firecracker, qemu).
type VMM interface {
	Create(ctx context.Context, cfg *VMConfig) (*MicroVM, error)
	Start(ctx context.Context, vmID string) error
	Stop(ctx context.Context, vmID string, timeout time.Duration) error
	Kill(ctx context.Context, vmID string) error
	State(ctx context.Context, vmID string) (VMState, error)
	Exec(ctx context.Context, vmID string, cmd []string, env []string, tty bool) error
	Attach(ctx context.Context, vmID string) error
	Logs(ctx context.Context, vmID string) (io.Reader, error)
	Stats(ctx context.Context, vmID string) (*VMStats, error)
	Cleanup(ctx context.Context, vmID string) error
	Name() string
}

// VMConfig holds configuration for creating a microVM.
type VMConfig struct {
	ID           string
	Kernel       string
	Rootfs       string
	Initrd       string
	CPUs         int
	Memory       int // MB
	Cmd          []string
	Env          []string
	Cwd          string
	Network      *NetworkConfig
	Vsock        *VsockConfig
	Filesystems  []FilesystemConfig
	KernelArgs   string
	Console      bool
	ReadOnlyRootfs bool
	ExtraDrives  []DriveConfig
}

// NetworkConfig holds network configuration for a VM.
type NetworkConfig struct {
	Type      string // "tap", "bridge", "none", "host"
	TapName   string
	Bridge    string
	IP        string
	Gateway   string
	DNS       []string
	MacAddress string
}

// VsockConfig holds vsock configuration.
type VsockConfig struct {
	CID    uint32
	Port   uint32
	Socket string
}

// FilesystemConfig holds virtio-fs or 9p config.
type FilesystemConfig struct {
	Type   string // "virtiofs", "9p", "pmem"
	Source string
	Target string
	Tag    string
}

// DriveConfig holds extra block device config.
type DriveConfig struct {
	Path       string
	ReadOnly   bool
	RootDevice bool
}

// MicroVM represents a running microVM.
type MicroVM struct {
	ID        string
	VMM       VMM
	PID       int
	State     VMState
	CID       uint32
	TapDevice string
	CreatedAt time.Time
	StartedAt time.Time
}

// VMState represents microVM lifecycle state.
type VMState string

const (
	VMStateCreated VMState = "created"
	VMStateRunning VMState = "running"
	VMStateStopped VMState = "stopped"
	VMStateFailed  VMState = "failed"
)

// VMStats holds resource usage statistics.
type VMStats struct {
	CPUPercent  float64
	MemoryMB    int64
	MemoryMaxMB int64
	NetRxBytes  int64
	NetTxBytes  int64
	DiskRead    int64
	DiskWrite   int64
}

// VMMConfig holds global VMM configuration.
type VMMConfig struct {
	ForceBackend string
	KernelPath   string
	WorkDir      string
	CNIBinDir    string
	CNIConfDir   string
	Debug        bool
}

// ─── Factory ───────────────────────────────────────────────────────

// NewVMM creates the best available VMM for the current platform.
func NewVMM(cfg *VMMConfig) (VMM, error) {
	if cfg == nil {
		cfg = &VMMConfig{}
	}
	// Force specific backend.
	if cfg.ForceBackend != "" {
		return createForcedBackend(cfg.ForceBackend, cfg)
	}
	// Auto-detect the best backend.
	return autoDetectVMM(cfg)
}

func createForcedBackend(name string, cfg *VMMConfig) (VMM, error) {
	switch name {
	case "crosvm":
		return createCrosvm(cfg)
	case "firecracker":
		return createFirecracker(cfg)
	case "qemu":
		return createQEMU(cfg)
	default:
		return nil, fmt.Errorf("unknown VMM backend: %s", name)
	}
}

func autoDetectVMM(cfg *VMMConfig) (VMM, error) {
	// 1. Try crosvm (best for Android + Linux).
	if crosvmInstalled() {
		if vmm, err := createCrosvm(cfg); err == nil {
			return vmm, nil
		}
	}
	// 2. Try Firecracker (best for Linux servers with KVM).
	if firecrackerInstalled() && kvmAvailable() {
		if vmm, err := createFirecracker(cfg); err == nil {
			return vmm, nil
		}
	}
	return nil, fmt.Errorf("no compatible VMM found on this system")
}

// ─── Detection helpers ────────────────────────────────────────────

func crosvmInstalled() bool {
	_, err := exec.LookPath("crosvm")
	return err == nil
}

func firecrackerInstalled() bool {
	_, err := exec.LookPath("firecracker")
	return err == nil
}

func kvmAvailable() bool {
	if _, err := os.Stat("/dev/kvm"); err == nil {
		return true
	}
	// On Android, check for AVF hypervisors.
	for _, dev := range []string{
		"/dev/gunyah",    // Qualcomm
		"/dev/geniezone", // MediaTek
		"/dev/halla",     // Samsung Exynos
	} {
		if _, err := os.Stat(dev); err == nil {
			return true
		}
	}
	return false
}

// HypervisorInfo returns information about the available hypervisor.
type HypervisorInfo struct {
	Type      string // "kvm", "gunyah", "geniezone", "halla", "none"
	Backend   string // "crosvm", "firecracker", "qemu", "none"
	Available bool
	Path      string
}

// DetectHypervisor returns the detected hypervisor info.
func DetectHypervisor() *HypervisorInfo {
	info := &HypervisorInfo{Type: "none", Backend: "none"}

	// Check KVM (Linux + Google Tensor on Android).
	if _, err := os.Stat("/dev/kvm"); err == nil {
		info.Type = "kvm"
		info.Available = true
		info.Path = "/dev/kvm"
	}

	// Check Android AVF hypervisors.
	for _, h := range []struct{ dev, name string }{
		{"/dev/gunyah", "gunyah"},
		{"/dev/geniezone", "geniezone"},
		{"/dev/halla", "halla"},
	} {
		if _, err := os.Stat(h.dev); err == nil {
			info.Type = h.name
			info.Available = true
			info.Path = h.dev
			break
		}
	}

	// Detect best backend.
	if crosvmInstalled() {
		info.Backend = "crosvm"
	} else if firecrackerInstalled() && info.Type == "kvm" {
		info.Backend = "firecracker"
	} else {
		info.Backend = "qemu"
	}

	return info
}

// IsAvailable returns true if any microVM backend is available.
func IsAvailable() bool {
	return crosvmInstalled() || (firecrackerInstalled() && kvmAvailable())
}

// Platform returns the current platform string.
func Platform() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

// ─── Stub backend creators (implemented in respective packages) ────

var registeredBackends = make(map[string]func(*VMMConfig) (VMM, error))

// RegisterBackend registers a VMM backend factory.
func RegisterBackend(name string, factory func(*VMMConfig) (VMM, error)) {
	registeredBackends[name] = factory
	// Update global creators for the factory function.
	switch name {
	case "crosvm":
		createCrosvm = factory
	case "firecracker":
		createFirecracker = factory
	case "qemu":
		createQEMU = factory
	}
}

var (
	createCrosvm      func(*VMMConfig) (VMM, error)
	createFirecracker func(*VMMConfig) (VMM, error)
	createQEMU        func(*VMMConfig) (VMM, error)
)

func init() {
	createCrosvm = func(cfg *VMMConfig) (VMM, error) {
		return nil, fmt.Errorf("crosvm backend not compiled in")
	}
	createFirecracker = func(cfg *VMMConfig) (VMM, error) {
		return nil, fmt.Errorf("firecracker backend not compiled in")
	}
	createQEMU = func(cfg *VMMConfig) (VMM, error) {
		return nil, fmt.Errorf("qemu backend not compiled in")
	}
}

// GenerateID generates a short VM ID.
func GenerateID() string {
	return strings.ToLower(fmt.Sprintf("%x", time.Now().UnixNano()))[:8]
}
