package runtime

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/OpceanAI/Doki/pkg/emulation"
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
	mu      sync.Mutex
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
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runners[mode]
}

// BestFor selects the optimal runner for the given configuration.
// Selection order:
//  1. Explicit cfg.Runtime or DOKI_RUNTIME override.
//  2. Workload-specific modes (WASM, foreign architecture).
//  3. Highest available isolation level compatible with the host.
//  4. Native fallback.
func (r *Registry) BestFor(cfg *Config) ContainerRunner {
	r.mu.Lock()
	defer r.mu.Unlock()

	if requested := requestedRuntime(cfg); requested != "" {
		if mode, ok := ParseExecutionMode(requested); ok {
			if runner, exists := r.runners[mode]; exists {
				return runner
			}
			r.log.Warn("requested runtime not available, falling back", "requested", requested)
		} else {
			r.log.Warn("requested runtime is unknown, falling back", "requested", requested)
		}
	}

	if cfg != nil && isWASMImage(cfg) {
		if runner, ok := r.runners[ModeWASM]; ok {
			return runner
		}
	}

	if cfg != nil && cfg.Platform != "" && cfg.Platform != hostPlatform() {
		if runner := r.preferredEmulationRunner(cfg.Platform); runner != nil {
			return runner
		}
		if isX86OnARM64(cfg.Platform) {
			if runner, ok := r.runners[ModeFEX]; ok {
				return runner
			}
		}
		for _, mode := range []ExecutionMode{ModeQEMUUser, ModeLegacy32} {
			if runner, ok := r.runners[mode]; ok {
				return runner
			}
		}
	}

	for _, info := range ExecutionModeInfos() {
		if info.Mode == ModeWASM || info.Mode == ModeQEMUUser || info.Mode == ModeFEX || info.Mode == ModeLegacy32 {
			continue
		}
		runner, ok := r.runners[info.Mode]
		if !ok {
			continue
		}
		if !runnerUsableOnHost(runner.Capabilities()) {
			continue
		}
		if cfg != nil && cfg.Platform != "" && !modeSupportsPlatform(info, cfg.Platform) {
			continue
		}
		return runner
	}

	if cfg != nil && cfg.Platform != "" {
		for _, info := range ExecutionModeInfos() {
			runner, ok := r.runners[info.Mode]
			if !ok || !runnerUsableOnHost(runner.Capabilities()) {
				continue
			}
			if modeSupportsPlatform(info, cfg.Platform) {
				return runner
			}
		}
	}

	if runner, ok := r.runners[ModeNative]; ok {
		return runner
	}
	for _, info := range ExecutionModeInfos() {
		if runner, ok := r.runners[info.Mode]; ok {
			return runner
		}
	}
	return nil
}

func (r *Registry) preferredEmulationRunner(platform string) ContainerRunner {
	switch emulation.PreferredMode() {
	case emulation.ModeFEX, emulation.ModeBox64:
		if isX86OnARM64(platform) {
			if runner, ok := r.runners[ModeFEX]; ok {
				return runner
			}
		}
	case emulation.ModeQEMU:
		if runner, ok := r.runners[ModeQEMUUser]; ok {
			return runner
		}
	}
	return nil
}

func requestedRuntime(cfg *Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Runtime) != "" {
		return strings.TrimSpace(cfg.Runtime)
	}
	return strings.TrimSpace(os.Getenv("DOKI_RUNTIME"))
}

func runnerUsableOnHost(caps RunnerCapabilities) bool {
	if caps.RootRequired && os.Geteuid() != 0 {
		return false
	}
	if caps.KVMRequired && !hasKVM() {
		return false
	}
	if len(caps.Arch) == 0 {
		return true
	}
	for _, arch := range caps.Arch {
		if arch == runtime.GOARCH {
			return true
		}
	}
	return false
}

func modeSupportsPlatform(info ExecutionModeInfo, platform string) bool {
	if platform == "" {
		return true
	}
	for _, supported := range info.Platforms {
		if supported == platform {
			return true
		}
		if strings.HasSuffix(supported, "/*") && strings.HasPrefix(platform, strings.TrimSuffix(supported, "*")) {
			return true
		}
	}
	return false
}

// Available returns all detected runners with their info.
func (r *Registry) Available() []RunnerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

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
	r.mu.Lock()
	defer r.mu.Unlock()

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
