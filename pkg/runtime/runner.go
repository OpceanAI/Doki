package runtime

import (
	"context"
	"log/slog"
	"syscall"
	"time"
)

// ExecutionMode defines how a container process is run.
type ExecutionMode int

const (
	ModeNative     ExecutionMode = iota // 0: Direct host execution
	ModeProot                           // 1: proot-based isolation
	ModeNamespaces                      // 2: Full Linux namespace isolation
	ModeMicroVM                         // 3: Hardware-level isolation via microVM
	ModeGVisor                          // 4: gVisor systrap user-space kernel
	ModeWASM                            // 5: WasmEdge/WAMR/Wasmtime WASI containers
	ModePkDroid                         // 6: pKVM hardware isolation + Microdroid
	ModeSysbox                          // 7: Rootless DinD via sysbox-runc
	ModeQEMUUser                        // 8: QEMU user-mode cross-arch emulation
	ModeChroot                          // 9: Lightweight chroot isolation
	ModeFEX                             // 10: FEX-Emu/Box64 x86 emulation on ARM64
	ModeLegacy32                        // 11: Dual-arch ARMv7/ARM64 containers
)

// ExecutionModeInfo describes a runtime mode in user-facing terms. Level is a
// rough isolation strength score where higher means stronger isolation.
type ExecutionModeInfo struct {
	Mode        ExecutionMode
	Name        string
	Level       int
	Isolation   string
	Platforms   []string
	Description string
}

// Info returns user-facing metadata for an execution mode.
func (m ExecutionMode) Info() ExecutionModeInfo {
	switch m {
	case ModePkDroid:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 12, Isolation: "hardware-pkvm", Platforms: []string{"android/arm64"}, Description: "Android pKVM or Microdroid hardware isolation"}
	case ModeMicroVM:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 11, Isolation: "hardware-vm", Platforms: []string{"linux/amd64", "linux/arm64", "android/arm64"}, Description: "MicroVM isolation through KVM, crosvm, Firecracker, or QEMU"}
	case ModeGVisor:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 10, Isolation: "user-space-kernel", Platforms: []string{"linux/amd64", "linux/arm64"}, Description: "gVisor syscall interception with a user-space kernel"}
	case ModeWASM:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 9, Isolation: "wasm-sandbox", Platforms: []string{"linux/*", "android/*", "darwin/*"}, Description: "WASI/WASM sandbox for WebAssembly workloads"}
	case ModeSysbox:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 8, Isolation: "user-namespace", Platforms: []string{"linux/amd64", "linux/arm64"}, Description: "Sysbox rootless Docker-in-Docker style isolation"}
	case ModeNamespaces:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 7, Isolation: "linux-kernel", Platforms: []string{"linux/*"}, Description: "Linux namespaces and cgroups"}
	case ModeProot:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 6, Isolation: "ptrace-userspace", Platforms: []string{"android/*", "linux/*"}, Description: "proot ptrace-based rootfs and syscall translation"}
	case ModeQEMUUser:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 5, Isolation: "user-mode-emulation", Platforms: []string{"linux/*", "android/*"}, Description: "QEMU user-mode cross-architecture execution"}
	case ModeFEX:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 4, Isolation: "user-mode-emulation", Platforms: []string{"linux/arm64", "android/arm64"}, Description: "FEX or Box64 x86 emulation on ARM64"}
	case ModeLegacy32:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 3, Isolation: "compat", Platforms: []string{"linux/arm64", "android/arm64"}, Description: "ARMv7 compatibility on ARM64 hosts"}
	case ModeChroot:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 2, Isolation: "chroot", Platforms: []string{"linux/*", "android/*"}, Description: "chroot-style filesystem isolation"}
	case ModeNative:
		return ExecutionModeInfo{Mode: m, Name: m.String(), Level: 1, Isolation: "none", Platforms: []string{"linux/*", "android/*", "darwin/*"}, Description: "direct host execution without container isolation"}
	default:
		return ExecutionModeInfo{Mode: m, Name: "unknown", Level: 0, Isolation: "unknown"}
	}
}

// ExecutionModeInfos returns metadata for every known execution mode sorted by
// descending isolation level.
func ExecutionModeInfos() []ExecutionModeInfo {
	modes := []ExecutionMode{
		ModePkDroid, ModeMicroVM, ModeGVisor, ModeWASM, ModeSysbox, ModeNamespaces,
		ModeProot, ModeQEMUUser, ModeFEX, ModeLegacy32, ModeChroot, ModeNative,
	}
	infos := make([]ExecutionModeInfo, 0, len(modes))
	for _, mode := range modes {
		infos = append(infos, mode.Info())
	}
	return infos
}

// String returns the human-readable name of the execution mode.
func (m ExecutionMode) String() string {
	switch m {
	case ModeNative:
		return "native"
	case ModeProot:
		return "proot"
	case ModeNamespaces:
		return "namespaces"
	case ModeMicroVM:
		return "microvm"
	case ModeGVisor:
		return "gvisor"
	case ModeWASM:
		return "wasm"
	case ModePkDroid:
		return "pkdroid"
	case ModeSysbox:
		return "sysbox"
	case ModeQEMUUser:
		return "qemu-user"
	case ModeChroot:
		return "chroot"
	case ModeFEX:
		return "fex"
	case ModeLegacy32:
		return "legacy32"
	default:
		return "unknown"
	}
}

// ParseExecutionMode parses a mode name string into an ExecutionMode.
func ParseExecutionMode(s string) (ExecutionMode, bool) {
	switch s {
	case "native":
		return ModeNative, true
	case "proot":
		return ModeProot, true
	case "namespaces":
		return ModeNamespaces, true
	case "microvm":
		return ModeMicroVM, true
	case "gvisor":
		return ModeGVisor, true
	case "wasm":
		return ModeWASM, true
	case "pkdroid":
		return ModePkDroid, true
	case "sysbox":
		return ModeSysbox, true
	case "qemu-user":
		return ModeQEMUUser, true
	case "chroot":
		return ModeChroot, true
	case "fex":
		return ModeFEX, true
	case "legacy32":
		return ModeLegacy32, true
	default:
		return 0, false
	}
}

// AllExecutionModes returns all supported execution modes.
func AllExecutionModes() []ExecutionMode {
	return []ExecutionMode{
		ModeNative, ModeProot, ModeNamespaces, ModeMicroVM,
		ModeGVisor, ModeWASM, ModePkDroid, ModeSysbox,
		ModeQEMUUser, ModeChroot, ModeFEX, ModeLegacy32,
	}
}

// ContainerRunner is the contract for any execution mode.
// Implementations must be goroutine-safe and support context cancellation.
type ContainerRunner interface {
	// Name returns the execution mode this runner implements.
	Name() ExecutionMode

	// Detect returns true if this runner can work on the current host.
	// Called once at startup to build the registry.
	Detect() bool

	// Capabilities returns what this runner supports.
	Capabilities() RunnerCapabilities

	// Create prepares the container filesystem, config, OCI spec.
	// Returns the container ID. Does NOT start the process.
	Create(ctx context.Context, cfg *Config) (string, error)

	// Start launches the container process. Returns the PID (or 0 for WASM).
	Start(ctx context.Context, id string) (int, error)

	// Stop sends a signal to the container. timeout=0 → SIGKILL.
	Stop(ctx context.Context, id string, timeout time.Duration) error

	// Exec runs a new process inside a running container.
	Exec(ctx context.Context, id string, cfg *ExecConfig) (int, error)

	// Kill sends an arbitrary signal to the container.
	Kill(ctx context.Context, id string, sig syscall.Signal) error

	// Pause suspends the container (cgroup freezer, SIGSTOP, or VM suspend).
	Pause(ctx context.Context, id string) error

	// Resume resumes a paused container.
	Resume(ctx context.Context, id string) error

	// Wait blocks until the container exits and returns the exit code.
	Wait(ctx context.Context, id string) (int, error)

	// Stats returns resource usage (CPU, memory, I/O).
	Stats(ctx context.Context, id string) (*ContainerStats, error)

	// Inspect returns full container metadata.
	Inspect(ctx context.Context, id string) (*ContainerJSON, error)

	// Cleanup removes all resources (rootfs, state, tmp files).
	Cleanup(ctx context.Context, id string) error
}

// RunnerCapabilities describes what a runner supports.
type RunnerCapabilities struct {
	Arch          []string // Architectures this runner can execute on
	RootRequired  bool     // Needs root/sudo to function
	KVMRequired   bool     // Needs /dev/kvm
	CrossArch     bool     // Can run foreign-arch binaries
	WASM          bool     // Is a WASM runtime
	HWIsolation   bool     // Hardware-enforced isolation
	PauseSupport  bool     // Supports pause/resume
	ExecSupport   bool     // Supports exec into running container
	StatsSupport  bool     // Supports resource stats
	DinDCapable   bool     // Can run Docker inside the container
	MaxContainers int      // 0 = unlimited
	GuestArch     []string // For cross-arch: which guest archs are supported
}

// ExecConfig holds configuration for exec into a running container.
type ExecConfig struct {
	ID           string
	ContainerID  string
	Args         []string
	Env          []string
	WorkingDir   string
	User         string
	Tty          bool
	AttachStdin  bool
	AttachStdout bool
	AttachStderr bool
	Privileged   bool
}

// ContainerStats holds resource usage for a container.
type ContainerStats struct {
	CPU     CPUStats
	Memory  MemoryStats
	Network NetworkStats
	PIDs    int
}

// CPUStats holds CPU usage.
type CPUStats struct {
	Usage    uint64
	Throttle uint64
	System   uint64
}

// MemoryStats holds memory usage.
type MemoryStats struct {
	Usage    uint64
	MaxUsage uint64
	Limit    uint64
	Cache    uint64
}

// NetworkStats holds network I/O.
type NetworkStats struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}

// ContainerJSON holds full container information for inspect.
type ContainerJSON struct {
	ID            string
	Pid           int
	Status        string
	Config        *Config
	RootfsPath    string
	LogPath       string
	Mode          ExecutionMode
	RestartCount  int
	CreatedAt     time.Time
	StartedAt     time.Time
	FinishedAt    time.Time
	ExitCode      int
	GuestArch     string // For cross-arch modes
	IsolationType string // "hardware", "user-space", "none"
}

// Logger returns a default logger for runner packages.
func Logger(component string) *slog.Logger {
	return slog.Default().With("component", component)
}
