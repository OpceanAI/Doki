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
	Name() string
	Detect() bool
	Run(ctx context.Context, wasmPath string, cfg *WASMConfig) (int, error)
	Stop(pid int) error
}

// WASMConfig holds configuration for a WASM container.
type WASMConfig struct {
	Rootfs string
	Args   []string
	Env    []string
	Cwd    string
}

type Runner struct {
	root     string
	backends []Backend
	active   Backend
	log      *slog.Logger
}

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

func (r *Runner) Name() rt.ExecutionMode { return rt.ModeWASM }

func (r *Runner) Detect() bool { return r.active != nil }

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

func (r *Runner) Create(ctx context.Context, cfg *rt.Config) (string, error) {
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

func (r *Runner) Stop(_ context.Context, id string, timeout time.Duration) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	return r.active.Stop(state.Pid)
}

func (r *Runner) Exec(_ context.Context, id string, cfg *rt.ExecConfig) (int, error) {
	return 0, fmt.Errorf("exec not supported in WASM mode")
}

func (r *Runner) Kill(_ context.Context, id string, sig syscall.Signal) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	return r.active.Stop(state.Pid)
}

func (r *Runner) Pause(_ context.Context, id string) error {
	return fmt.Errorf("pause not supported in WASM mode")
}

func (r *Runner) Resume(_ context.Context, id string) error {
	return fmt.Errorf("resume not supported in WASM mode")
}

func (r *Runner) Wait(_ context.Context, id string) (int, error) { return 0, nil }
func (r *Runner) Stats(_ context.Context, id string) (*rt.ContainerStats, error) {
	return &rt.ContainerStats{}, nil
}

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

func (r *Runner) Cleanup(_ context.Context, id string) error {
	return os.RemoveAll(filepath.Join(r.root, "containers", id))
}

func (r *Runner) loadState(id string) (*rt.ContainerState, error) {
	data, err := os.ReadFile(filepath.Join(r.root, "containers", id, "state.json"))
	if err != nil {
		return nil, err
	}
	var s rt.ContainerState
	json.Unmarshal(data, &s)
	return &s, nil
}

// ─── WasmEdge backend ─────────────────────────────────────────────

type WasmEdgeBackend struct{ binPath string }

func (b *WasmEdgeBackend) Name() string { return "wasmedge" }
func (b *WasmEdgeBackend) Detect() bool {
	p, err := exec.LookPath("wasmedge")
	if err == nil {
		b.binPath = p
	}
	return err == nil
}
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
func (b *WasmEdgeBackend) Stop(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// ─── WAMR backend ─────────────────────────────────────────────────

type WAMRBackend struct{ binPath string }

func (b *WAMRBackend) Name() string { return "wamr" }
func (b *WAMRBackend) Detect() bool {
	p, err := exec.LookPath("iwasm")
	if err == nil {
		b.binPath = p
	}
	return err == nil
}
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
func (b *WAMRBackend) Stop(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// ─── Wasmtime backend ─────────────────────────────────────────────

type WasmtimeBackend struct{ binPath string }

func (b *WasmtimeBackend) Name() string { return "wasmtime" }
func (b *WasmtimeBackend) Detect() bool {
	p, err := exec.LookPath("wasmtime")
	if err == nil {
		b.binPath = p
	}
	return err == nil
}
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
func (b *WasmtimeBackend) Stop(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

var _ rt.ContainerRunner = (*Runner)(nil)
