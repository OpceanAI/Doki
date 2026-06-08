// Package sysbox implements the rootless DinD container runner.
// Uses sysbox-runc for Docker-in-Docker without --privileged.
package sysbox

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

type Runner struct {
	root string
	log  *slog.Logger
}

func New(root string) *Runner {
	return &Runner{root: root, log: slog.Default().With("component", "runner.sysbox")}
}

func (r *Runner) Name() rt.ExecutionMode { return rt.ModeSysbox }

func (r *Runner) Detect() bool {
	if _, err := exec.LookPath("sysbox-runc"); err != nil {
		return false
	}
	data, _ := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone")
	return len(data) > 0 && data[0] == '1'
}

func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch: []string{"arm64", "amd64"}, RootRequired: true,
		DinDCapable: true, ExecSupport: true, StatsSupport: true, PauseSupport: true,
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
	r.log.Info("sysbox container created", "id", common.ShortID(id))
	return id, nil
}

func (r *Runner) Start(ctx context.Context, id string) (int, error) {
	state, err := r.loadState(id)
	if err != nil {
		return 0, err
	}
	bundleDir := filepath.Join(r.root, "containers", id)
	cmd := exec.CommandContext(ctx, "sysbox-runc", "create", "--bundle", bundleDir, id)
	cmd.Env = state.Config.Env
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("sysbox-runc create: %s %w", string(out), err)
	}
	if err := exec.Command("sysbox-runc", "start", id).Run(); err != nil {
		return 0, fmt.Errorf("sysbox-runc start: %w", err)
	}
	r.log.Info("sysbox container started", "id", common.ShortID(id))
	return 0, nil
}

func (r *Runner) Stop(_ context.Context, id string, timeout time.Duration) error {
	if timeout > 0 {
		exec.Command("sysbox-runc", "kill", id, "TERM").Run()
		time.Sleep(timeout)
	}
	return exec.Command("sysbox-runc", "kill", id, "KILL").Run()
}

func (r *Runner) Exec(_ context.Context, id string, cfg *rt.ExecConfig) (int, error) {
	args := append([]string{"exec", id}, cfg.Args...)
	cmd := exec.Command("sysbox-runc", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return 0, cmd.Run()
}

func (r *Runner) Kill(_ context.Context, id string, sig syscall.Signal) error {
	return exec.Command("sysbox-runc", "kill", id, fmt.Sprintf("%d", sig)).Run()
}

func (r *Runner) Pause(_ context.Context, id string) error {
	return exec.Command("sysbox-runc", "pause", id).Run()
}

func (r *Runner) Resume(_ context.Context, id string) error {
	return exec.Command("sysbox-runc", "resume", id).Run()
}

func (r *Runner) Wait(_ context.Context, id string) (int, error) {
	out, err := exec.Command("sysbox-runc", "wait", id).CombinedOutput()
	if err != nil {
		return -1, err
	}
	code := 0
	fmt.Sscanf(string(out), "%d", &code)
	return code, nil
}

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
		Config: state.Config, Mode: rt.ModeSysbox, IsolationType: "user-namespace",
		CreatedAt: state.Created,
	}, nil
}

func (r *Runner) Cleanup(_ context.Context, id string) error {
	exec.Command("sysbox-runc", "delete", id).Run()
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
