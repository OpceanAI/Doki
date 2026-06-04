// Package gvisor implements the gVisor container runner.
// Uses gVisor's systrap platform for user-space kernel isolation.
package gvisor

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
	return &Runner{root: root, log: slog.Default().With("component", "runner.gvisor")}
}

func (r *Runner) Name() rt.ExecutionMode { return rt.ModeGVisor }

func (r *Runner) Detect() bool {
	if _, err := exec.LookPath("runsc"); err != nil {
		return false
	}
	cmd := exec.Command("runsc", "do", "--platform=systrap", "true")
	return cmd.Run() == nil
}

func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch: []string{"arm64", "amd64"}, RootRequired: false,
		ExecSupport: true, StatsSupport: true, PauseSupport: true,
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
	r.log.Info("container created", "id", common.ShortID(id))
	return id, nil
}

func (r *Runner) Start(ctx context.Context, id string) (int, error) {
	state, err := r.loadState(id)
	if err != nil {
		return 0, err
	}
	bundleDir := filepath.Join(r.root, "containers", id)
	cmd := exec.CommandContext(ctx, "runsc",
		"--platform=systrap", "--network=sandbox", "--rootless",
		"create", "--bundle", bundleDir, id,
	)
	cmd.Env = state.Config.Env
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("runsc create: %s %w", string(out), err)
	}
	if err := exec.Command("runsc", "start", id).Run(); err != nil {
		return 0, fmt.Errorf("runsc start: %w", err)
	}
	pid := 0
	stateFile := filepath.Join(bundleDir, "state.json")
	if data, err := os.ReadFile(stateFile); err == nil {
		var s struct {
			Pid int `json:"pid"`
		}
		json.Unmarshal(data, &s)
		pid = s.Pid
	}
	r.log.Info("container started", "id", common.ShortID(id), "pid", pid)
	return pid, nil
}

func (r *Runner) Stop(_ context.Context, id string, timeout time.Duration) error {
	if timeout > 0 {
		exec.Command("runsc", "kill", "--signal", "TERM", id).Run()
		time.Sleep(timeout)
	}
	return exec.Command("runsc", "kill", "--signal", "KILL", id).Run()
}

func (r *Runner) Exec(_ context.Context, id string, cfg *rt.ExecConfig) (int, error) {
	args := append([]string{"exec", id}, cfg.Args...)
	cmd := exec.Command("runsc", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return 0, cmd.Run()
}

func (r *Runner) Kill(_ context.Context, id string, sig syscall.Signal) error {
	return exec.Command("runsc", "kill", "--signal", fmt.Sprintf("%d", sig), id).Run()
}

func (r *Runner) Pause(_ context.Context, id string) error {
	return exec.Command("runsc", "pause", id).Run()
}

func (r *Runner) Resume(_ context.Context, id string) error {
	return exec.Command("runsc", "resume", id).Run()
}

func (r *Runner) Wait(_ context.Context, id string) (int, error) {
	out, err := exec.Command("runsc", "wait", id).CombinedOutput()
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
		Config: state.Config, Mode: rt.ModeGVisor, IsolationType: "user-space-kernel",
		CreatedAt: state.Created, StartedAt: state.Started,
	}, nil
}

func (r *Runner) Cleanup(_ context.Context, id string) error {
	exec.Command("runsc", "delete", id).Run()
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

var _ rt.ContainerRunner = (*Runner)(nil)
