package runtime

import (
	"context"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestExecutionModeString(t *testing.T) {
	tests := []struct {
		mode ExecutionMode
		want string
	}{
		{ModeNative, "native"},
		{ModeProot, "proot"},
		{ModeNamespaces, "namespaces"},
		{ModeMicroVM, "microvm"},
		{ModeGVisor, "gvisor"},
		{ModeWASM, "wasm"},
		{ModePkDroid, "pkdroid"},
		{ModeSysbox, "sysbox"},
		{ModeQEMUUser, "qemu-user"},
		{ModeChroot, "chroot"},
		{ModeFEX, "fex"},
		{ModeLegacy32, "legacy32"},
		{ExecutionMode(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("ExecutionMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestParseExecutionMode(t *testing.T) {
	tests := []struct {
		input string
		want  ExecutionMode
		ok    bool
	}{
		{"native", ModeNative, true},
		{"proot", ModeProot, true},
		{"namespaces", ModeNamespaces, true},
		{"microvm", ModeMicroVM, true},
		{"gvisor", ModeGVisor, true},
		{"wasm", ModeWASM, true},
		{"pkdroid", ModePkDroid, true},
		{"sysbox", ModeSysbox, true},
		{"qemu-user", ModeQEMUUser, true},
		{"chroot", ModeChroot, true},
		{"fex", ModeFEX, true},
		{"legacy32", ModeLegacy32, true},
		{"invalid", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseExecutionMode(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseExecutionMode(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("ParseExecutionMode(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestAllExecutionModes(t *testing.T) {
	modes := AllExecutionModes()
	if len(modes) != 12 {
		t.Errorf("AllExecutionModes() returned %d modes, want 12", len(modes))
	}
	seen := make(map[ExecutionMode]bool)
	for _, m := range modes {
		if seen[m] {
			t.Errorf("Duplicate mode: %d", m)
		}
		seen[m] = true
	}
}

func TestRegistryRegister(t *testing.T) {
	reg := NewRegistry()
	// Native always detects.
	nr := &mockRunner{mode: ModeNative, detect: true}
	reg.Register(nr)
	if reg.Get(ModeNative) == nil {
		t.Error("Register: native runner not found")
	}
	// Undetectable runner should not be registered.
	ur := &mockRunner{mode: ModeGVisor, detect: false}
	reg.Register(ur)
	if reg.Get(ModeGVisor) != nil {
		t.Error("Register: undetectable runner should not be registered")
	}
}

func TestRegistryAvailable(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRunner{mode: ModeNative, detect: true})
	reg.Register(&mockRunner{mode: ModeProot, detect: true})
	avail := reg.Available()
	if len(avail) != 2 {
		t.Errorf("Available() returned %d, want 2", len(avail))
	}
}

func TestRegistryBestFor(t *testing.T) {
	reg := NewRegistry()
	nr := &mockRunner{mode: ModeNative, detect: true}
	pr := &mockRunner{mode: ModeProot, detect: true}
	reg.Register(nr)
	reg.Register(pr)

	// Explicit runtime selection.
	cfg := &Config{Runtime: "proot"}
	best := reg.BestFor(cfg)
	if best == nil || best.Name() != ModeProot {
		t.Errorf("BestFor(Runtime=proot) = %v, want proot", best)
	}

	// Fallback: when requested runtime unavailable, falls back to best available.
	// In this test, proot is registered and preferred over native.
	cfg2 := &Config{Runtime: "gvisor"}
	best2 := reg.BestFor(cfg2)
	if best2 == nil {
		t.Error("BestFor(Runtime=gvisor) returned nil")
	}

	// Default (no explicit runtime).
	cfg3 := &Config{}
	best3 := reg.BestFor(cfg3)
	if best3 == nil {
		t.Error("BestFor({}) returned nil")
	}
}

func TestRegistryBestForUsesEnvRuntime(t *testing.T) {
	t.Setenv("DOKI_RUNTIME", "proot")
	reg := NewRegistry()
	reg.Register(&mockRunner{mode: ModeNative, detect: true})
	reg.Register(&mockRunner{mode: ModeProot, detect: true})

	best := reg.BestFor(&Config{})
	if best == nil || best.Name() != ModeProot {
		t.Fatalf("BestFor(DOKI_RUNTIME=proot) = %v, want proot", best)
	}
}

func TestRegistryBestForChoosesHighestUsableLevel(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRunner{mode: ModeNative, detect: true})
	reg.Register(&mockRunner{mode: ModeProot, detect: true})
	reg.Register(&mockRunner{mode: ModeGVisor, detect: true, caps: RunnerCapabilities{Arch: []string{runtime.GOARCH}}})

	best := reg.BestFor(&Config{})
	if best == nil || best.Name() != ModeGVisor {
		t.Fatalf("BestFor highest usable = %v, want gvisor", best)
	}
}

func TestRegistryBestForSkipsUnavailableHostRequirements(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRunner{mode: ModeNative, detect: true})
	reg.Register(&mockRunner{mode: ModeProot, detect: true})
	reg.Register(&mockRunner{mode: ModeGVisor, detect: true, caps: RunnerCapabilities{Arch: []string{"definitely-not-this-arch"}}})
	reg.Register(&mockRunner{mode: ModeMicroVM, detect: true, caps: RunnerCapabilities{KVMRequired: true}})

	best := reg.BestFor(&Config{})
	if best == nil || best.Name() != ModeProot {
		t.Fatalf("BestFor with unusable higher levels = %v, want proot", best)
	}
}

func TestRegistryBestForCrossArchPrefersEmulation(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRunner{mode: ModeNative, detect: true})
	reg.Register(&mockRunner{mode: ModeProot, detect: true})
	reg.Register(&mockRunner{mode: ModeQEMUUser, detect: true})

	best := reg.BestFor(&Config{Platform: "linux/not-host"})
	if best == nil || best.Name() != ModeQEMUUser {
		t.Fatalf("BestFor cross-arch = %v, want qemu-user", best)
	}
}

func TestRegistryBestForUsesQEMUPreference(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DOKI_EMULATION_MODE", "qemu")
	reg := NewRegistry()
	reg.Register(&mockRunner{mode: ModeNative, detect: true})
	reg.Register(&mockRunner{mode: ModeProot, detect: true})
	reg.Register(&mockRunner{mode: ModeQEMUUser, detect: true})
	reg.Register(&mockRunner{mode: ModeLegacy32, detect: true})

	best := reg.BestFor(&Config{Platform: "linux/not-host"})
	if best == nil || best.Name() != ModeQEMUUser {
		t.Fatalf("BestFor with qemu preference = %v, want qemu-user", best)
	}
}

func TestRegistryAllRunners(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRunner{mode: ModeNative, detect: true})
	all := reg.AllRunners()
	if len(all) != len(AllExecutionModes()) {
		t.Errorf("AllRunners() returned %d, want %d", len(all), len(AllExecutionModes()))
	}
	for _, info := range all {
		if info.Mode == ModeNative && !info.Available {
			t.Error("Native should be available")
		}
		if info.Mode == ModeGVisor && info.Available {
			t.Error("GVisor should not be available (not registered)")
		}
	}
}

func TestIsWASMImage(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"nil", nil, false},
		{"empty", &Config{}, false},
		{"wasi/wasm", &Config{Platform: "wasi/wasm"}, true},
		{"wasm32-wasip1", &Config{Platform: "wasm32-wasip1"}, true},
		{"linux/arm64", &Config{Platform: "linux/arm64"}, false},
		{"wasm entrypoint", &Config{ImageConfig: &ImageOCIConfig{Entrypoint: []string{"app.wasm"}}}, true},
		{"wasm cmd", &Config{ImageConfig: &ImageOCIConfig{Cmd: []string{"main.wasm"}}}, true},
		{"no wasm", &Config{ImageConfig: &ImageOCIConfig{Entrypoint: []string{"/bin/sh"}}}, false},
	}
	for _, tt := range tests {
		if got := isWASMImage(tt.cfg); got != tt.want {
			t.Errorf("isWASMImage(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// mockRunner is a minimal ContainerRunner for testing.
type mockRunner struct {
	mu     sync.Mutex
	mode   ExecutionMode
	detect bool
	caps   RunnerCapabilities
}

func (m *mockRunner) Name() ExecutionMode { m.mu.Lock(); defer m.mu.Unlock(); return m.mode }
func (m *mockRunner) Detect() bool        { m.mu.Lock(); defer m.mu.Unlock(); return m.detect }
func (m *mockRunner) Capabilities() RunnerCapabilities {
	m.mu.Lock()
	defer m.mu.Unlock()
	caps := m.caps
	caps.Arch = append([]string(nil), caps.Arch...)
	caps.GuestArch = append([]string(nil), caps.GuestArch...)
	return caps
}
func (m *mockRunner) Create(_ context.Context, _ *Config) (string, error)          { return "", nil }
func (m *mockRunner) Start(_ context.Context, _ string) (int, error)               { return 0, nil }
func (m *mockRunner) Stop(_ context.Context, _ string, _ time.Duration) error      { return nil }
func (m *mockRunner) Exec(_ context.Context, _ string, _ *ExecConfig) (int, error) { return 0, nil }
func (m *mockRunner) Kill(_ context.Context, _ string, _ syscall.Signal) error     { return nil }
func (m *mockRunner) Pause(_ context.Context, _ string) error                      { return nil }
func (m *mockRunner) Resume(_ context.Context, _ string) error                     { return nil }
func (m *mockRunner) Wait(_ context.Context, _ string) (int, error)                { return 0, nil }
func (m *mockRunner) Stats(_ context.Context, _ string) (*ContainerStats, error)   { return nil, nil }
func (m *mockRunner) Inspect(_ context.Context, _ string) (*ContainerJSON, error)  { return nil, nil }
func (m *mockRunner) Cleanup(_ context.Context, _ string) error                    { return nil }
