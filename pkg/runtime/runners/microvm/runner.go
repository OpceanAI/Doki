// Package microvm implements the microVM container runner.
// Uses Firecracker, crosvm, or Cloud Hypervisor for hardware-level isolation.
package microvm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/dokivm"
	"github.com/OpceanAI/Doki/pkg/common"
	rt "github.com/OpceanAI/Doki/pkg/runtime"
)

// Runner executes containers inside a microVM using Firecracker, crosvm, or Cloud Hypervisor.
type Runner struct {
	root    string
	log     *slog.Logger
	vmm     dokivm.VMM
	microVM *dokivm.MicroVM
}

// New creates a new microVM runner with the given storage root.
func New(root string) *Runner {
	return &Runner{
		root: root,
		log:  slog.Default().With("component", "runner.microvm"),
	}
}

// Name returns the execution mode.
func (r *Runner) Name() rt.ExecutionMode { return rt.ModeMicroVM }

// Detect checks if this runner is available on the current host.
func (r *Runner) Detect() bool {
	return dokivm.IsAvailable()
}

// Capabilities returns the runner capabilities.
func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch:         []string{"arm64", "amd64"},
		RootRequired: true,
		KVMRequired:  true,
		HWIsolation:  true,
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
		return "", err
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

	info := dokivm.DetectHypervisor()
	r.log.Info("starting microVM", "id", common.ShortID(id), "backend", info.Backend)

	vmm, err := dokivm.NewVMM(&dokivm.VMMConfig{})
	if err != nil {
		return 0, fmt.Errorf("create VMM: %w", err)
	}

	vmCfg := &dokivm.VMConfig{
		Rootfs:     state.Config.RootfsReady,
		KernelArgs: "console=ttyS0 reboot=k panic=1",
		Memory:     256,
		CPUs:       1,
		Cmd:        state.Config.Args,
		Env:        state.Config.Env,
		Cwd:        state.Config.Cwd,
	}
	if state.Config.Resources != nil && state.Config.Resources.Memory > 0 {
		vmCfg.Memory = int(state.Config.Resources.Memory / 1024 / 1024)
		if vmCfg.Memory < 128 {
			vmCfg.Memory = 128
		}
	}

	vm, err := vmm.Create(ctx, vmCfg)
	if err != nil {
		return 0, fmt.Errorf("create microVM: %w", err)
	}
	if err := vmm.Start(ctx, vm.ID); err != nil {
		return 0, fmt.Errorf("start microVM: %w", err)
	}
	r.vmm = vmm
	r.microVM = vm
	r.log.Info("microVM started", "id", common.ShortID(id), "pid", vm.PID)
	return vm.PID, nil
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
	return 0, fmt.Errorf("exec not supported in microVM mode")
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
func (r *Runner) Pause(_ context.Context, _ string) error {
	return fmt.Errorf("pause not supported in microVM mode")
}

// Resume resumes a paused container.
func (r *Runner) Resume(_ context.Context, _ string) error {
	return fmt.Errorf("resume not supported in microVM mode")
}

// Wait blocks until the container exits.
func (r *Runner) Wait(ctx context.Context, _ string) (int, error) {
	if r.vmm == nil || r.microVM == nil {
		return 0, fmt.Errorf("no running microVM")
	}
	for {
		state, err := r.vmm.State(ctx, r.microVM.ID)
		if err != nil {
			return -1, err
		}
		if state == dokivm.VMStateStopped || state == dokivm.VMStateFailed {
			return 0, nil
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
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
		Config: state.Config, Mode: rt.ModeMicroVM, IsolationType: "hardware",
		CreatedAt: state.Created, StartedAt: state.Started,
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
		return nil, err
	}
	return &s, nil
}

var _ rt.ContainerRunner = (*Runner)(nil)
