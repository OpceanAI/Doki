// Package legacy32 implements the dual-arch ARMv7/ARM64 container runner.
// Supports running 32-bit containers on 64-bit hosts via kernel compat or QEMU fallback.
package legacy32

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	rt "github.com/OpceanAI/Doki/pkg/runtime"
)

type Runner struct {
	root      string
	canCompat bool
	log       *slog.Logger
	cmd       *exec.Cmd
}

func New(root string) *Runner {
	r := &Runner{
		root:      root,
		canCompat: canRun32Bit(),
		log:       slog.Default().With("component", "runner.legacy32"),
	}
	return r
}

// Name returns the execution mode.
func (r *Runner) Name() rt.ExecutionMode { return rt.ModeLegacy32 }

// Detect checks if this runner is available on the current host.
func (r *Runner) Detect() bool { return true }

// Capabilities returns the runner capabilities.
func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch: []string{"arm64", "armv7", "amd64", "386"},
		GuestArch: []string{"armv7", "386"}, RootRequired: false,
	}
}

// Create prepares the container filesystem and config.
func (r *Runner) Create(_ context.Context, cfg *rt.Config) (string, error) {
	id := cfg.ID
	if id == "" {
		id = common.GenerateID(64)
	}
	dir := filepath.Join(r.root, "containers", id)
	if err := common.EnsureDir(dir); err != nil {
		return "", err
	}
	r.log.Info("legacy32 container created", "id", common.ShortID(id), "can_compat", r.canCompat)
	return id, nil
}

// Start launches the container process.
func (r *Runner) Start(ctx context.Context, id string) (int, error) {
	state, err := r.loadState(id)
	if err != nil {
		return 0, err
	}
	args := state.Config.Args
	if len(args) == 0 {
		return 0, fmt.Errorf("no command specified")
	}
	rootfsDir := state.Config.RootfsReady
	if rootfsDir == "" {
		rootfsDir = filepath.Join(state.Bundle, "rootfs")
	}
	targetArch := r.detectArch(state.Config)

	// If host can run 32-bit natively (kernel compat).
	if r.canCompat && is32Bit(targetArch) {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = rootfsDir
		cmd.Env = state.Config.Env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		r.cmd = cmd
		r.log.Info("legacy32 native", "id", common.ShortID(id), "pid", cmd.Process.Pid, "arch", targetArch)
		return cmd.Process.Pid, nil
	}

	// Fallback: use QEMU user-mode.
	qemuBin := "qemu-" + targetArch + "-static"
	if p, err := exec.LookPath(qemuBin); err == nil {
		qemuArgs := []string{"-L", rootfsDir}
		qemuArgs = append(qemuArgs, args...)
		cmd := exec.CommandContext(ctx, p, qemuArgs...)
		cmd.Dir = rootfsDir
		cmd.Env = state.Config.Env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		r.cmd = cmd
		r.log.Info("legacy32 emulated", "id", common.ShortID(id), "pid", cmd.Process.Pid, "arch", targetArch)
		return cmd.Process.Pid, nil
	}

	return 0, fmt.Errorf("cannot run %s binary: no compat mode and no QEMU", targetArch)
}

// Stop terminates the container with a signal.
func (r *Runner) Stop(_ context.Context, id string, timeout time.Duration) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, _ := os.FindProcess(state.Pid)
	_ = process.Signal(syscall.SIGTERM)
	if timeout > 0 {
		time.Sleep(timeout)
	}
	_ = process.Signal(syscall.SIGKILL)
	return nil
}

// Exec runs a process inside a running container.
func (r *Runner) Exec(_ context.Context, _ string, cfg *rt.ExecConfig) (int, error) {
	if len(cfg.Args) == 0 {
		return 0, fmt.Errorf("no command specified")
	}
	cmd := exec.Command(cfg.Args[0], cfg.Args[1:]...)
	cmd.Env = cfg.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// Kill sends an arbitrary signal to the container.
func (r *Runner) Kill(_ context.Context, id string, sig syscall.Signal) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, _ := os.FindProcess(state.Pid)
	return process.Signal(sig)
}

// Pause suspends the container.
func (r *Runner) Pause(_ context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, _ := os.FindProcess(state.Pid)
	return process.Signal(syscall.SIGSTOP)
}

// Resume resumes a paused container.
func (r *Runner) Resume(_ context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, _ := os.FindProcess(state.Pid)
	return process.Signal(syscall.SIGCONT)
}

// Wait blocks until the container exits.
func (r *Runner) Wait(_ context.Context, _ string) (int, error) {
	if r.cmd == nil {
		return 0, fmt.Errorf("no running process")
	}
	err := r.cmd.Wait()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}
// Stats returns resource usage metrics.
func (r *Runner) Stats(_ context.Context, _ string) (*rt.ContainerStats, error) {
	return &rt.ContainerStats{}, nil
}

// Inspect returns detailed container info.
func (r *Runner) Inspect(_ context.Context, id string) (*rt.ContainerJSON, error) {
	state, err := r.loadState(id)
	if err != nil {
		return nil, err
	}
	return &rt.ContainerJSON{
		ID: state.ID, Pid: state.Pid, Status: string(state.Status),
		Config: state.Config, Mode: rt.ModeLegacy32, IsolationType: "compat",
		GuestArch: r.detectArch(state.Config), CreatedAt: state.Created,
	}, nil
}

// Cleanup removes container state after exit.
func (r *Runner) Cleanup(_ context.Context, id string) error {
	return os.RemoveAll(filepath.Join(r.root, "containers", id))
}

func (r *Runner) detectArch(cfg *rt.Config) string {
	if cfg != nil && cfg.Platform != "" {
		parts := strings.Split(cfg.Platform, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	if runtime.GOARCH == "arm64" {
		return "armv7"
	}
	return "386"
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

func canRun32Bit() bool {
	if runtime.GOARCH == "amd64" {
		return true
	}
	if runtime.GOARCH == "arm64" {
		_, err := os.Stat("/proc/sys/abi")
		return err == nil
	}
	return false
}

func is32Bit(arch string) bool {
	return arch == "armv7" || arch == "arm" || arch == "386" || arch == "i386"
}

var _ rt.ContainerRunner = (*Runner)(nil)
