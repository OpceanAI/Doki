// Package namespaces implements the Linux namespaces container runner.
// Requires root. Uses clone(2) with CLONE_NEW* flags for full isolation.
package namespaces

import (
	"context"
	"fmt"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	ns "github.com/OpceanAI/Doki/internal/namespaces"
	rt "github.com/OpceanAI/Doki/pkg/runtime"
)

type Runner struct {
	root  string
	nsMgr *ns.Manager
	log   *slog.Logger
	cmd   *exec.Cmd
}

func New(root string) *Runner {
	return &Runner{
		root:  root,
		nsMgr: ns.NewManager(root),
		log:   slog.Default().With("component", "runner.namespaces"),
	}
}

func (r *Runner) Name() rt.ExecutionMode { return rt.ModeNamespaces }

func (r *Runner) Detect() bool {
	if os.Geteuid() != 0 {
		return false
	}
	if runtime.GOOS != "linux" {
		return false
	}
	return ns.Supported(ns.MountNS)
}

func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch:         []string{"arm64", "amd64"},
		RootRequired: true,
		ExecSupport:  true,
		StatsSupport: true,
		PauseSupport: true,
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
	args := state.Config.Args
	if len(args) == 0 {
		return 0, fmt.Errorf("no command specified")
	}

	rootfsDir := state.Config.RootfsReady
	if rootfsDir == "" {
		rootfsDir = filepath.Join(state.Bundle, "rootfs")
	}

	// pivot_root setup.
	oldRootDir := filepath.Join(rootfsDir, ".pivot_root")
	if err := os.MkdirAll(oldRootDir, 0755); err != nil {
		return 0, fmt.Errorf("pivot_root setup: %w", err)
	}
	pivotScript := fmt.Sprintf(
		`mount --bind "%s" "%s" && pivot_root "%s" "%s/.pivot_root" && cd / && umount -l "/.pivot_root" && exec "$@"`,
		rootfsDir, rootfsDir, rootfsDir, rootfsDir)
	allArgs := append([]string{"/bin/sh", "-c", pivotScript, "doki-init"}, args...)

	cmd := exec.CommandContext(ctx, allArgs[0], allArgs[1:]...)
	cmd.Dir = rootfsDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = state.Config.Env

	cloneFlags := syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS |
		syscall.CLONE_NEWIPC | syscall.CLONE_NEWPID
	if state.Config.NetworkMode != common.NetworkHost && state.Config.NetworkMode != common.NetworkNone {
		cloneFlags |= syscall.CLONE_NEWNET
	}
	if !state.Config.Privileged {
		cloneFlags |= syscall.CLONE_NEWUSER
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: uintptr(cloneFlags)}

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	r.cmd = cmd

	// Set up user namespace mapping.
	if !state.Config.Privileged {
		if err := r.nsMgr.SetupUserNamespace(cmd.Process.Pid, &ns.Config{
			User: true, Rootless: true,
		}); err != nil {
			_ = cmd.Process.Kill()
			return 0, fmt.Errorf("setup user namespace: %w", err)
		}
	}

	// Bring up loopback in new network namespace.
	if state.Config.NetworkMode != common.NetworkHost && state.Config.NetworkMode != common.NetworkNone {
		_ = exec.Command("nsenter", "-t", strconv.Itoa(cmd.Process.Pid), "-n",
			"ip", "link", "set", "lo", "up").Run()
	}

	r.log.Info("container started", "id", common.ShortID(id), "pid", cmd.Process.Pid)
	return cmd.Process.Pid, nil
}

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

func (r *Runner) Exec(_ context.Context, id string, cfg *rt.ExecConfig) (int, error) {
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

func (r *Runner) Kill(_ context.Context, id string, sig syscall.Signal) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, _ := os.FindProcess(state.Pid)
	return process.Signal(sig)
}

func (r *Runner) Pause(_ context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, _ := os.FindProcess(state.Pid)
	return process.Signal(syscall.SIGSTOP)
}

func (r *Runner) Resume(_ context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	process, _ := os.FindProcess(state.Pid)
	return process.Signal(syscall.SIGCONT)
}

func (r *Runner) Wait(_ context.Context, id string) (int, error) {
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
		Config: state.Config, Mode: rt.ModeNamespaces, IsolationType: "user-space",
		CreatedAt: state.Created, StartedAt: state.Started,
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
		return nil, err
	}
	return &s, nil
}

var _ rt.ContainerRunner = (*Runner)(nil)
