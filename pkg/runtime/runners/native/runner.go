// Package native implements the native container runner.
// This is the simplest mode: direct host execution without any isolation.
package native

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

// Runner implements ContainerRunner for direct host execution.
type Runner struct {
	root string
	log  *slog.Logger
	cmd  *exec.Cmd
}

// New creates a new native runner.
func New(root string) *Runner {
	return &Runner{
		root: root,
		log:  slog.Default().With("component", "runner.native"),
	}
}

// Name returns the execution mode.
func (r *Runner) Name() rt.ExecutionMode { return rt.ModeNative }

// Detect checks if this runner is available on the current host.
func (r *Runner) Detect() bool { return true }

// Capabilities returns the runner capabilities.
func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch:         []string{"arm64", "armv7", "amd64", "386"},
		ExecSupport:  true,
		StatsSupport: true,
		PauseSupport: true,
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
		return "", fmt.Errorf("create container dir: %w", err)
	}
	r.log.Info("container created", "id", common.ShortID(id))
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
		return 0, fmt.Errorf("no command specified for container %s", id)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if state.Config.Cwd != "" {
		cmd.Dir = state.Config.Cwd
	}
	cmd.Env = state.Config.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start process: %w", err)
	}
	r.cmd = cmd

	r.log.Info("container started", "id", common.ShortID(id), "pid", cmd.Process.Pid)
	return cmd.Process.Pid, nil
}

// Stop terminates the container with a signal.
func (r *Runner) Stop(_ context.Context, id string, timeout time.Duration) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	if state.Pid <= 0 {
		return fmt.Errorf("container %s has no PID", id)
	}
	process, err := os.FindProcess(state.Pid)
	if err != nil {
		return err
	}
	_ = process.Signal(syscall.SIGTERM)
	if timeout > 0 {
		time.Sleep(timeout)
	}
	_ = process.Signal(syscall.SIGKILL)
	r.log.Info("container stopped", "id", common.ShortID(id))
	return nil
}

// Exec runs a process inside a running container.
func (r *Runner) Exec(_ context.Context, _ string, cfg *rt.ExecConfig) (int, error) {
	if len(cfg.Args) == 0 {
		return 0, fmt.Errorf("no command specified for exec")
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
	process, err := os.FindProcess(state.Pid)
	if err != nil {
		return err
	}
	return process.Signal(sig)
}

// Pause suspends the container.
func (r *Runner) Pause(_ context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, err := os.FindProcess(state.Pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGSTOP)
}

// Resume resumes a paused container.
func (r *Runner) Resume(_ context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, err := os.FindProcess(state.Pid)
	if err != nil {
		return err
	}
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
		ID:            state.ID,
		Pid:           state.Pid,
		Status:        string(state.Status),
		Config:        state.Config,
		RootfsPath:    state.Config.RootfsReady,
		LogPath:       state.LogPath,
		Mode:          rt.ModeNative,
		IsolationType: "none",
		CreatedAt:     state.Created,
		StartedAt:     state.Started,
	}, nil
}

// Cleanup removes container state after exit.
func (r *Runner) Cleanup(_ context.Context, id string) error {
	dir := filepath.Join(r.root, "containers", id)
	return os.RemoveAll(dir)
}

func (r *Runner) loadState(id string) (*rt.ContainerState, error) {
	statePath := filepath.Join(r.root, "containers", id, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	var state rt.ContainerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

var _ rt.ContainerRunner = (*Runner)(nil)
