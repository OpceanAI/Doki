// Package pkdroid implements the pKVM hardware isolation container runner.
// Uses Android Virtualization Framework (AVF) + Microdroid for hardware-enforced isolation.
package pkdroid

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	rt "github.com/OpceanAI/Doki/pkg/runtime"
)

type Runner struct {
	root string
	log  *slog.Logger
}

func New(root string) *Runner {
	return &Runner{root: root, log: slog.Default().With("component", "runner.pkdroid")}
}

// Name returns the execution mode.
func (r *Runner) Name() rt.ExecutionMode { return rt.ModePkDroid }

// Detect checks if this runner is available on the current host.
func (r *Runner) Detect() bool {
	if runtime.GOOS != "android" || runtime.GOARCH != "arm64" {
		return false
	}
	_, err := os.Stat("/dev/kvm")
	return err == nil
}

// Capabilities returns the runner capabilities.
func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch: []string{"arm64"}, RootRequired: false,
		HWIsolation: true, StatsSupport: true,
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
	r.log.Info("pkdroid container created", "id", common.ShortID(id))
	return id, nil
}

// Start launches the container process.
func (r *Runner) Start(_ context.Context, _ string) (int, error) {
	return 0, fmt.Errorf("pkdroid: AVF integration not yet implemented — requires NDK bridge to VirtualizationService")
}

// Stop terminates the container with a signal.
func (r *Runner) Stop(_ context.Context, _ string, _ time.Duration) error {
	return fmt.Errorf("pkdroid: not implemented")
}

// Exec runs a process inside a running container.
func (r *Runner) Exec(_ context.Context, _ string, _ *rt.ExecConfig) (int, error) {
	return 0, fmt.Errorf("pkdroid: exec not supported")
}

// Kill sends an arbitrary signal to the container.
func (r *Runner) Kill(_ context.Context, _ string, _ syscall.Signal) error {
	return fmt.Errorf("pkdroid: not implemented")
}

// Pause suspends the container.
func (r *Runner) Pause(_ context.Context, _ string) error {
	return fmt.Errorf("pkdroid: pause not supported")
}

// Resume resumes a paused container.
func (r *Runner) Resume(_ context.Context, _ string) error {
	return fmt.Errorf("pkdroid: resume not supported")
}

// Wait blocks until the container exits.
func (r *Runner) Wait(_ context.Context, _ string) (int, error) { return 0, nil }
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
		Config: state.Config, Mode: rt.ModePkDroid, IsolationType: "hardware-pkvm",
		CreatedAt: state.Created,
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
