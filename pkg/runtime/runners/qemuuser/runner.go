// Package qemuuser implements the QEMU user-mode cross-arch container runner.
// Uses qemu-*-static binaries to run containers of different architectures.
package qemuuser

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
	emulators map[string]string
	log       *slog.Logger
	cmd       *exec.Cmd
}

func New(root string) *Runner {
	r := &Runner{
		root:      root,
		emulators: make(map[string]string),
		log:       slog.Default().With("component", "runner.qemu-user"),
	}
	for _, arch := range []string{"x86_64", "i386", "aarch64", "arm"} {
		bin := "qemu-" + arch + "-static"
		if p, err := exec.LookPath(bin); err == nil {
			r.emulators[arch] = p
		}
	}
	return r
}

func (r *Runner) Name() rt.ExecutionMode { return rt.ModeQEMUUser }

func (r *Runner) Detect() bool { return len(r.emulators) > 0 }

func (r *Runner) Capabilities() rt.RunnerCapabilities {
	guestArch := make([]string, 0, len(r.emulators))
	for arch := range r.emulators {
		guestArch = append(guestArch, arch)
	}
	return rt.RunnerCapabilities{
		Arch: []string{"arm64", "armv7", "amd64", "386"},
		CrossArch: true, GuestArch: guestArch, RootRequired: false,
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
	r.log.Info("qemu-user container created", "id", common.ShortID(id))
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
	// Detect target arch from image config.
	targetArch := r.detectArch(state.Config)
	qemuBin, ok := r.emulators[targetArch]
	if !ok {
		return 0, fmt.Errorf("no QEMU emulator for arch %s", targetArch)
	}
	// If target matches host, run directly.
	if targetArch == hostArch() {
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
		return cmd.Process.Pid, nil
	}
	// Use QEMU as wrapper.
	qemuArgs := []string{"-L", rootfsDir}
	qemuArgs = append(qemuArgs, args...)
	cmd := exec.CommandContext(ctx, qemuBin, qemuArgs...)
	cmd.Dir = rootfsDir
	cmd.Env = state.Config.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("qemu start: %w", err)
	}
	r.cmd = cmd
	r.log.Info("qemu-user container started", "id", common.ShortID(id),
		"pid", cmd.Process.Pid, "arch", targetArch, "emulator", qemuBin)
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
		Config: state.Config, Mode: rt.ModeQEMUUser, IsolationType: "emulation",
		GuestArch: r.detectArch(state.Config), CreatedAt: state.Created,
	}, nil
}

func (r *Runner) Cleanup(_ context.Context, id string) error {
	return os.RemoveAll(filepath.Join(r.root, "containers", id))
}

func (r *Runner) detectArch(cfg *rt.Config) string {
	if cfg != nil && cfg.Platform != "" {
		parts := strings.Split(cfg.Platform, "/")
		if len(parts) >= 3 {
			return archMap(parts[2])
		}
		if len(parts) >= 2 {
			return archMap(parts[1])
		}
	}
	return hostArch()
}

func hostArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "arm":
		return "arm"
	case "amd64":
		return "x86_64"
	case "386":
		return "i386"
	default:
		return runtime.GOARCH
	}
}

func archMap(s string) string {
	switch s {
	case "amd64":
		return "x86_64"
	case "386":
		return "i386"
	case "arm64":
		return "aarch64"
	default:
		return s
	}
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
