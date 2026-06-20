// Package wasm implements the WASM/WASI container runner.
// Supports WasmEdge, WAMR, and Wasmtime backends.
package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	rt "github.com/OpceanAI/Doki/pkg/runtime"
)

// Backend represents a WASM runtime backend.
type Backend interface {
	// Name returns the backend name.
	Name() string
	// Detect checks if the backend binary is available.
	Detect() bool
	// Run starts a WASM module with the given configuration.
	Run(ctx context.Context, wasmPath string, cfg *WASMConfig) (int, error)
	// Stop terminates a process by PID.
	Stop(pid int) error
}

// WASMConfig holds configuration for a WASM container.
type WASMConfig struct {
	Rootfs string
	Args   []string
	Env    []string
	Cwd    string
}

// Runner executes WASM containers using available backends.
type Runner struct {
	root     string
	backends []Backend
	active   Backend
	log      *slog.Logger
}

// New creates a new WASM runner with detected backends.
func New(root string) *Runner {
	r := &Runner{
		root: root,
		log:  slog.Default().With("component", "runner.wasm"),
	}
	r.backends = []Backend{
		&WasmEdgeBackend{},
		&WAMRBackend{},
		&WasmtimeBackend{},
	}
	for _, b := range r.backends {
		if b.Detect() {
			r.active = b
			break
		}
	}
	return r
}

// Name returns the execution mode for this runner.
func (r *Runner) Name() rt.ExecutionMode { return rt.ModeWASM }

// Detect reports whether a WASM backend is available.
func (r *Runner) Detect() bool { return r.active != nil }

// Capabilities returns the runner's supported features and architectures.
func (r *Runner) Capabilities() rt.RunnerCapabilities {
	arch := []string{"arm64", "amd64"}
	if r.active != nil {
		if _, ok := r.active.(*WAMRBackend); ok {
			arch = []string{"arm64", "armv7", "amd64", "386"}
		}
	}
	return rt.RunnerCapabilities{
		Arch: arch, RootRequired: false, WASM: true,
		ExecSupport: false, StatsSupport: false, PauseSupport: false,
	}
}

// Create initializes a new WASM container directory.
func (r *Runner) Create(_ context.Context, cfg *rt.Config) (string, error) {
	id := cfg.ID
	if id == "" {
		id = common.GenerateID(64)
	}
	dir := filepath.Join(r.root, "containers", id)
	if err := common.EnsureDir(dir); err != nil {
		return "", err
	}
	r.log.Info("wasm container created", "id", common.ShortID(id), "backend", r.active.Name())
	return id, nil
}

// Start begins execution of a WASM container.
func (r *Runner) Start(ctx context.Context, id string) (int, error) {
	state, err := r.loadState(id)
	if err != nil {
		return 0, err
	}
	wasmPath := ""
	for _, arg := range state.Config.Args {
		if len(arg) > 5 && arg[len(arg)-5:] == ".wasm" {
			wasmPath = arg
			break
		}
	}
	if wasmPath == "" {
		return 0, fmt.Errorf("no .wasm file found in args")
	}
	wasmCfg := &WASMConfig{
		Rootfs: state.Config.RootfsReady,
		Args:   state.Config.Args,
		Env:    state.Config.Env,
		Cwd:    state.Config.Cwd,
	}
	pid, err := r.active.Run(ctx, wasmPath, wasmCfg)
	if err != nil {
		return 0, err
	}
	r.log.Info("wasm container started", "id", common.ShortID(id), "pid", pid)
	return pid, nil
}

// Stop terminates a WASM container by its process ID.
func (r *Runner) Stop(_ context.Context, id string, _ time.Duration) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	return r.active.Stop(state.Pid)
}

// Exec is not supported in WASM mode.
func (r *Runner) Exec(_ context.Context, _ string, _ *rt.ExecConfig) (int, error) {
	return 0, fmt.Errorf("exec not supported in WASM mode")
}

// Kill sends a signal to stop the WASM container process.
func (r *Runner) Kill(_ context.Context, id string, _ syscall.Signal) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	return r.active.Stop(state.Pid)
}

// Pause is not supported in WASM mode.
func (r *Runner) Pause(_ context.Context, _ string) error {
	return fmt.Errorf("pause not supported in WASM mode")
}

// Resume is not supported in WASM mode.
func (r *Runner) Resume(_ context.Context, _ string) error {
	return fmt.Errorf("resume not supported in WASM mode")
}

// Wait is a no-op in WASM mode.
func (r *Runner) Wait(_ context.Context, _ string) (int, error) { return 0, nil }

// Stats returns empty container statistics.
func (r *Runner) Stats(_ context.Context, _ string) (*rt.ContainerStats, error) {
	return &rt.ContainerStats{}, nil
}

// Inspect returns container metadata and state.
func (r *Runner) Inspect(_ context.Context, id string) (*rt.ContainerJSON, error) {
	state, err := r.loadState(id)
	if err != nil {
		return nil, err
	}
	return &rt.ContainerJSON{
		ID: state.ID, Pid: state.Pid, Status: string(state.Status),
		Config: state.Config, Mode: rt.ModeWASM, IsolationType: "wasm-sandbox",
		CreatedAt: state.Created,
	}, nil
}

// Cleanup removes the container directory.
func (r *Runner) Cleanup(_ context.Context, id string) error {
	return os.RemoveAll(filepath.Join(r.root, "containers", id))
}

func (r *Runner) loadState(id string) (*rt.ContainerState, error) {
	data, err := os.ReadFile(filepath.Join(r.root, "containers", id, "state.json"))
	if err != nil {
		return nil, err
	}
	var s rt.ContainerState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &s, nil
}

// ─── WasmEdge backend ─────────────────────────────────────────────

// WasmEdgeBackend is a WASM backend using the WasmEdge runtime.
type WasmEdgeBackend struct{ binPath string }

// Name returns "wasmedge".
func (b *WasmEdgeBackend) Name() string { return "wasmedge" }

// Detect checks for the wasmedge binary.
func (b *WasmEdgeBackend) Detect() bool {
	p, err := exec.LookPath("wasmedge")
	if err == nil {
		b.binPath = p
	}
	return err == nil
}
// Run executes a WASM module with WasmEdge.
func (b *WasmEdgeBackend) Run(ctx context.Context, wasmPath string, cfg *WASMConfig) (int, error) {
	args := []string{"--dir", "/:" + cfg.Rootfs}
	for _, e := range cfg.Env {
		args = append(args, "--env", e)
	}
	if cfg.Cwd != "" {
		args = append(args, "--dir", cfg.Cwd)
	}
	args = append(args, wasmPath)
	args = append(args, cfg.Args[1:]...)
	cmd := exec.CommandContext(ctx, b.binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}
// Stop kills a process by PID.
func (b *WasmEdgeBackend) Stop(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// ─── WAMR backend ─────────────────────────────────────────────────

// WAMRBackend is a WASM backend using the WAMR (iwasm) runtime.
type WAMRBackend struct{ binPath string }

// Name returns "wamr".
func (b *WAMRBackend) Name() string { return "wamr" }

// Detect checks for the iwasm binary.
func (b *WAMRBackend) Detect() bool {
	p, err := exec.LookPath("iwasm")
	if err == nil {
		b.binPath = p
	}
	return err == nil
}
// Run executes a WASM module with WAMR.
func (b *WAMRBackend) Run(ctx context.Context, wasmPath string, cfg *WASMConfig) (int, error) {
	args := []string{"--dir", cfg.Rootfs}
	args = append(args, wasmPath)
	args = append(args, cfg.Args[1:]...)
	cmd := exec.CommandContext(ctx, b.binPath, args...)
	cmd.Env = cfg.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}
// Stop kills a process by PID.
func (b *WAMRBackend) Stop(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// ─── Wasmtime backend ─────────────────────────────────────────────

// WasmtimeBackend is a WASM backend using the Wasmtime runtime.
type WasmtimeBackend struct{ binPath string }

// Name returns "wasmtime".
func (b *WasmtimeBackend) Name() string { return "wasmtime" }

// Detect checks for the wasmtime binary.
func (b *WasmtimeBackend) Detect() bool {
	p, err := exec.LookPath("wasmtime")
	if err == nil {
		b.binPath = p
	}
	return err == nil
}
// Run executes a WASM module with Wasmtime.
func (b *WasmtimeBackend) Run(ctx context.Context, wasmPath string, cfg *WASMConfig) (int, error) {
	args := []string{"--dir", "/:" + cfg.Rootfs}
	args = append(args, wasmPath)
	args = append(args, cfg.Args[1:]...)
	cmd := exec.CommandContext(ctx, b.binPath, args...)
	cmd.Env = cfg.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}
// Stop kills a process by PID.
func (b *WasmtimeBackend) Stop(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

var _ rt.ContainerRunner = (*Runner)(nil)
