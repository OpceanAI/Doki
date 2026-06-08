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

func (r *Runner) Name() rt.ExecutionMode { return rt.ModePkDroid }

func (r *Runner) Detect() bool {
	if runtime.GOOS != "android" || runtime.GOARCH != "arm64" {
		return false
	}
	_, err := os.Stat("/dev/kvm")
	return err == nil
}

func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch: []string{"arm64"}, RootRequired: false,
		HWIsolation: true, StatsSupport: true,
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
	r.log.Info("pkdroid container created", "id", common.ShortID(id))
	return id, nil
}

func (r *Runner) Start(ctx context.Context, id string) (int, error) {
	return 0, fmt.Errorf("pkdroid: AVF integration not yet implemented — requires NDK bridge to VirtualizationService")
}

func (r *Runner) Stop(_ context.Context, id string, timeout time.Duration) error {
	return fmt.Errorf("pkdroid: not implemented")
}

func (r *Runner) Exec(_ context.Context, id string, cfg *rt.ExecConfig) (int, error) {
	return 0, fmt.Errorf("pkdroid: exec not supported")
}

func (r *Runner) Kill(_ context.Context, id string, sig syscall.Signal) error {
	return fmt.Errorf("pkdroid: not implemented")
}

func (r *Runner) Pause(_ context.Context, id string) error {
	return fmt.Errorf("pkdroid: pause not supported")
}

func (r *Runner) Resume(_ context.Context, id string) error {
	return fmt.Errorf("pkdroid: resume not supported")
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
		Config: state.Config, Mode: rt.ModePkDroid, IsolationType: "hardware-pkvm",
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
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &s, nil
}

var _ rt.ContainerRunner = (*Runner)(nil)
