// Package fex implements the FEX-Emu/Box64 container runner for x86 emulation on ARM64.
package fex

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	rt "github.com/OpceanAI/Doki/pkg/runtime"
)

type Runner struct {
	root     string
	backend  string
	fexBin   string
	box64Bin string
	log      *slog.Logger
	cmd      *exec.Cmd
}

func New(root string) *Runner {
	r := &Runner{root: root, log: slog.Default().With("component", "runner.fex")}
	if p, err := exec.LookPath("FEXInterpreter"); err == nil {
		r.fexBin = p
		r.backend = "fex"
	} else if p, err := exec.LookPath("box64"); err == nil {
		r.box64Bin = p
		r.backend = "box64"
	}
	return r
}

// Name returns the execution mode.
func (r *Runner) Name() rt.ExecutionMode { return rt.ModeFEX }

// Detect checks if this runner is available on the current host.
func (r *Runner) Detect() bool {
	return runtime.GOARCH == "arm64" && (r.fexBin != "" || r.box64Bin != "")
}

// Capabilities returns the runner capabilities.
func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch: []string{"arm64"}, CrossArch: true,
		GuestArch: []string{"x86_64", "i386"}, RootRequired: false,
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
	r.log.Info("fex container created", "id", common.ShortID(id), "backend", r.backend)
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
	var cmd *exec.Cmd
	switch r.backend {
	case "fex":
		fexArgs := []string{"--rootfs", rootfsDir, "--"}
		fexArgs = append(fexArgs, args...)
		cmd = exec.CommandContext(ctx, r.fexBin, fexArgs...)
	case "box64":
		entrypoint := filepath.Join(rootfsDir, args[0])
		boxArgs := append([]string{entrypoint}, args[1:]...)
		cmd = exec.CommandContext(ctx, r.box64Bin, boxArgs...)
	default:
		return 0, fmt.Errorf("no FEX backend available")
	}
	cmd.Dir = rootfsDir
	cmd.Env = state.Config.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("fex start: %w", err)
	}
	r.cmd = cmd
	r.log.Info("fex container started", "id", common.ShortID(id), "pid", cmd.Process.Pid, "backend", r.backend)
	return cmd.Process.Pid, nil
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
func (r *Runner) Exec(_ context.Context, _ string, _ *rt.ExecConfig) (int, error) {
	return 0, fmt.Errorf("exec not supported in FEX mode")
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
		Config: state.Config, Mode: rt.ModeFEX, IsolationType: "emulation",
		GuestArch: "x86_64", CreatedAt: state.Created,
	}, nil
}

// Cleanup removes container state after exit.
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

var _ rt.ContainerRunner = (*Runner)(nil)
