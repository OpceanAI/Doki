package runtime

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

// RunnerInfo holds information about a detected runner.
type RunnerInfo struct {
	Mode         ExecutionMode
	Name         string
	Capabilities RunnerCapabilities
	Available    bool
}

// Registry manages all available container runners and selects the best
// one for a given configuration.
type Registry struct {
	mu      sync.RWMutex
	runners map[ExecutionMode]ContainerRunner
	order   []ExecutionMode // priority order (highest first)
	log     *slog.Logger
}

// NewRegistry creates a new runner registry.
func NewRegistry() *Registry {
	return &Registry{
		runners: make(map[ExecutionMode]ContainerRunner),
		log:     slog.Default().With("component", "runner-registry"),
	}
}

// Register adds a runner to the registry if it's available on this host.
func (r *Registry) Register(runner ContainerRunner) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := runner.Name()
	if runner.Detect() {
		r.runners[name] = runner
		r.order = append(r.order, name)
		caps := runner.Capabilities()
		r.log.Info("runtime available",
			"mode", name.String(),
			"arch", caps.Arch,
			"root_required", caps.RootRequired,
			"kvm_required", caps.KVMRequired,
			"cross_arch", caps.CrossArch,
			"wasm", caps.WASM,
			"hw_isolation", caps.HWIsolation,
			"dind_capable", caps.DinDCapable,
		)
	} else {
		r.log.Debug("runtime unavailable", "mode", name.String())
	}
}

// Get returns the runner for the given mode, or nil if not available.
func (r *Registry) Get(mode ExecutionMode) ContainerRunner {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.runners[mode]
}

// BestFor selects the optimal runner for the given configuration.
// Priority chain:
//  1. Explicit --runtime flag (cfg.Runtime)
//  2. WASM images → ModeWASM
//  3. Cross-arch platform → ModeQEMUUser or ModeFEX
//  4. Root available → ModeNamespaces or ModeChroot
//  5. KVM available → ModeMicroVM
//  6. gVisor available → ModeGVisor
//  7. pKVM available → ModePkDroid
//  8. proot available → ModeProot
//  9. Fallback → ModeNative
func (r *Registry) BestFor(cfg *Config) ContainerRunner {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Explicit runtime selection.
	if cfg != nil && cfg.Runtime != "" {
		if mode, ok := ParseExecutionMode(cfg.Runtime); ok {
			if runner, exists := r.runners[mode]; exists {
				return runner
			}
			r.log.Warn("requested runtime not available, falling back", "requested", cfg.Runtime)
		}
	}

	// 2. WASM images.
	if cfg != nil && isWASMImage(cfg) {
		if runner, ok := r.runners[ModeWASM]; ok {
			return runner
		}
	}

	// 3. Cross-arch platform.
	if cfg != nil && cfg.Platform != "" && cfg.Platform != hostPlatform() {
		// Try QEMU user-mode first (more universal).
		if runner, ok := r.runners[ModeQEMUUser]; ok {
			return runner
		}
		// FEX is faster for x86 on ARM64.
		if isX86OnARM64(cfg.Platform) {
			if runner, ok := r.runners[ModeFEX]; ok {
				return runner
			}
		}
	}

	// 4. Root available.
	if os.Geteuid() == 0 {
		if runner, ok := r.runners[ModeNamespaces]; ok {
			return runner
		}
		if runner, ok := r.runners[ModeChroot]; ok {
			return runner
		}
	}

	// 5. KVM available.
	if hasKVM() {
		if runner, ok := r.runners[ModeMicroVM]; ok {
			return runner
		}
	}

	// 6. gVisor.
	if runner, ok := r.runners[ModeGVisor]; ok {
		return runner
	}

	// 7. pKVM (Android hardware isolation).
	if runner, ok := r.runners[ModePkDroid]; ok {
		return runner
	}

	// 8. Sysbox.
	if runner, ok := r.runners[ModeSysbox]; ok {
		return runner
	}

	// 9. proot.
	if runner, ok := r.runners[ModeProot]; ok {
		return runner
	}

	// 10. Fallback.
	if runner, ok := r.runners[ModeNative]; ok {
		return runner
	}

	// Should never happen (native always detects).
	return nil
}

// Available returns all detected runners with their info.
func (r *Registry) Available() []RunnerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]RunnerInfo, 0, len(r.runners))
	for _, mode := range r.order {
		runner := r.runners[mode]
		infos = append(infos, RunnerInfo{
			Mode:         mode,
			Name:         mode.String(),
			Capabilities: runner.Capabilities(),
			Available:    true,
		})
	}
	return infos
}

// AllRunners returns info about all known modes (available and unavailable).
func (r *Registry) AllRunners() []RunnerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var infos []RunnerInfo
	for _, mode := range AllExecutionModes() {
		runner, available := r.runners[mode]
		info := RunnerInfo{
			Mode:      mode,
			Name:      mode.String(),
			Available: available,
		}
		if available {
			info.Capabilities = runner.Capabilities()
		}
		infos = append(infos, info)
	}
	return infos
}

// ─── Helpers ───────────────────────────────────────────────────────

func hostPlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func isWASMImage(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	// WASM images have platform "wasi/wasm" or contain .wasm files.
	if cfg.Platform == "wasi/wasm" || cfg.Platform == "wasm32-wasip1" || cfg.Platform == "wasm32-wasip2" {
		return true
	}
	// Check image config for WASM entrypoint.
	if cfg.ImageConfig != nil {
		for _, arg := range cfg.ImageConfig.Entrypoint {
			if len(arg) > 5 && arg[len(arg)-5:] == ".wasm" {
				return true
			}
		}
		for _, arg := range cfg.ImageConfig.Cmd {
			if len(arg) > 5 && arg[len(arg)-5:] == ".wasm" {
				return true
			}
		}
	}
	return false
}

func isX86OnARM64(platform string) bool {
	if runtime.GOARCH != "arm64" {
		return false
	}
	return platform == "linux/amd64" || platform == "linux/386"
}

func hasKVM() bool {
	_, err := os.Stat("/dev/kvm")
	return err == nil
}

// DetectGVisor checks if gVisor (runsc) is available.
func DetectGVisor() bool {
	_, err := exec.LookPath("runsc")
	return err == nil
}

// DetectWasmEdge checks if WasmEdge is available.
func DetectWasmEdge() bool {
	_, err := exec.LookPath("wasmedge")
	return err == nil
}

// DetectWAMR checks if WAMR (iwasm) is available.
func DetectWAMR() bool {
	_, err := exec.LookPath("iwasm")
	return err == nil
}

// DetectSysbox checks if sysbox-runc is available.
func DetectSysbox() bool {
	_, err := exec.LookPath("sysbox-runc")
	return err == nil
}

// DetectFEXEmu checks if FEX-Emu is available.
func DetectFEXEmu() bool {
	_, err := exec.LookPath("FEXInterpreter")
	return err == nil
}

// DetectBox64 checks if Box64 is available.
func DetectBox64() bool {
	_, err := exec.LookPath("box64")
	return err == nil
}
