// Package proot implements the proot container runner.
// Uses ptrace-based isolation via doki-proot binary.
// Supports two modes:
//   - IPC mode: communicates with doki-proot daemon via Unix socket
//   - CLI mode: spawns proot as a child process (fallback)
package proot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/proot"
	"github.com/OpceanAI/Doki/pkg/common"
	rt "github.com/OpceanAI/Doki/pkg/runtime"
)

type Runner struct {
	root string
	ipc  *proot.IPCClient
	log  *slog.Logger
	cmd  *exec.Cmd
}

func New(root string) *Runner {
	socketPath := "/tmp/doki-proot.sock"
	if s := os.Getenv("DOKI_PROOT_SOCKET"); s != "" {
		socketPath = s
	}
	return &Runner{
		root: root,
		ipc:  proot.NewIPCClient(socketPath),
		log:  slog.Default().With("component", "runner.proot"),
	}
}

func (r *Runner) Name() rt.ExecutionMode { return rt.ModeProot }

func (r *Runner) Detect() bool {
	if _, err := exec.LookPath("doki-proot"); err == nil {
		return true
	}
	if _, err := exec.LookPath("proot"); err == nil {
		return true
	}
	return false
}

func (r *Runner) Capabilities() rt.RunnerCapabilities {
	return rt.RunnerCapabilities{
		Arch:         []string{"arm64", "armv7", "amd64", "386"},
		RootRequired: false,
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

	// Try IPC mode first.
	if err := r.ipc.Connect(ctx); err != nil {
		r.log.Warn("IPC connect failed, falling back to CLI mode", "err", err)
		return r.startCLI(ctx, state)
	}

	// Send exec via IPC.
	events, err := r.ipc.Exec(ctx, id, args, state.Config.Env, state.Config.Cwd)
	if err != nil {
		r.log.Warn("IPC exec failed, falling back to CLI mode", "err", err)
		return r.startCLI(ctx, state)
	}

	// Goroutine to log events.
	go func() {
		for event := range events {
			switch event.Type {
			case proot.EventStdout:
				os.Stdout.Write([]byte(event.Data))
			case proot.EventStderr:
				os.Stderr.Write([]byte(event.Data))
			case proot.EventExit:
				r.log.Info("container exited via IPC", "id", common.ShortID(id), "code", event.ExitCode)
			}
		}
	}()

	r.log.Info("container started via IPC", "id", common.ShortID(id))
	return 0, nil
}

// startCLI spawns proot as a child process (fallback when IPC unavailable).
func (r *Runner) startCLI(ctx context.Context, state *rt.ContainerState) (int, error) {
	args := state.Config.Args
	rootfsDir := state.Config.RootfsReady
	if rootfsDir == "" {
		rootfsDir = filepath.Join(state.Bundle, "rootfs")
	}

	prootBin := proot.FindProotBinary()
	if prootBin == "" {
		prootBin = "proot"
	}

	prootArgs := []string{
		"-r", rootfsDir,
		"-b", "/proc", "-b", "/sys", "-b", "/dev",
		"--kill-on-exit", "--link2symlink",
		"--kernel-release=" + proot.DetectKernelRelease(),
		"-i", "0:0",
	}
	if state.Config.Cwd != "" {
		prootArgs = append(prootArgs, "-w", state.Config.Cwd)
	}
	prootArgs = append(prootArgs, args...)

	// Clear LD_PRELOAD family in the parent process so exec.Command does not
	// propagate libtermux-exec.so to the proot child.
	proot.UnsetProotKillers()

	cmd := exec.CommandContext(ctx, prootBin, prootArgs...)
	// Use BuildEnv for the same env composition as startWithProot:
	// StripHostEnv (17-var deny-list) + AndroidEnv defaults + image env +
	// user env.
	var imageEnv []string
	if state.Config.ImageConfig != nil {
		imageEnv = state.Config.ImageConfig.Env
	}
	cmd.Env = proot.BuildEnv(state.Config.Env, imageEnv)
	cmd.Dir = "/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("proot start: %w", err)
	}
	r.cmd = cmd
	r.log.Info("container started via CLI", "id", common.ShortID(state.ID), "pid", cmd.Process.Pid, "bin", prootBin)
	return cmd.Process.Pid, nil
}

func (r *Runner) Stop(_ context.Context, id string, timeout time.Duration) error {
	// Try IPC signal first.
	if r.ipc.IsConnected() {
		if err := r.ipc.Signal(id, "SIGTERM"); err == nil {
			if timeout > 0 {
				time.Sleep(timeout)
			}
			r.ipc.Signal(id, "SIGKILL")
			return nil
		}
	}
	// Fallback to process signal.
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
	state, err := r.loadState(id)
	if err != nil {
		return 0, err
	}
	rootfsDir := ""
	if state.Config != nil {
		rootfsDir = state.Config.RootfsReady
	}
	if rootfsDir == "" {
		return 0, fmt.Errorf("no rootfs for container %s", id)
	}

	uid, gid := parseUser(cfg.User)
	prootBase, err := proot.BuildProotBaseArgs(rootfsDir, uid, gid)
	if err != nil {
		return 0, err
	}
	if cfg.WorkingDir != "" {
		prootBase = append(prootBase, "--cwd", cfg.WorkingDir)
	}
	// Build the full argv: [prootBin, prootBase..., "--", userCmd...]
	// prootBase[0] is "-r", not the proot binary. The previous code passed
	// prootBase as the entire argv to exec.Command, which made the kernel
	// try to exec "-r" as a command and fail with "executable file not
	// found".
	prootBin := proot.FindProotBinary()
	if prootBin == "" {
		prootBin = "proot"
	}
	argv := append([]string{prootBin}, prootBase...)
	argv = append(argv, "--")
	argv = append(argv, cfg.Args...)

	// Clear LD_PRELOAD family in the parent process so exec.Command does not
	// propagate libtermux-exec.so to the proot child.
	proot.UnsetProotKillers()

	cmd := exec.Command(prootBin, argv[1:]...)
	// Use BuildEnv for the same env composition as startWithProot.
	cmd.Env = proot.BuildEnv(cfg.Env, nil)
	cmd.Dir = "/"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// parseUser parses a user string ("uid" or "uid:gid") and returns uid, gid.
// Returns (-1, -1) if the string is empty.
func parseUser(user string) (int, int) {
	if user == "" {
		return -1, -1
	}
	parts := strings.SplitN(user, ":", 2)
	uid, err := strconv.Atoi(parts[0])
	if err != nil {
		return -1, -1
	}
	gid := uid
	if len(parts) >= 2 {
		if g, err := strconv.Atoi(parts[1]); err == nil {
			gid = g
		}
	}
	return uid, gid
}

// normalizeProotCwd converts a container working directory into a form
// that proot can safely use with -w. Relative paths like "." or "" must
// be replaced with "/" to avoid the "<rootfs>/./." chdir warning that
// proot emits when the guest cwd is ambiguous.
func normalizeProotCwd(cwd string) string {
	cwd = filepath.Clean(cwd)
	if cwd == "." || cwd == "" {
		return "/"
	}
	if !strings.HasPrefix(cwd, "/") {
		return "/" + cwd
	}
	return cwd
}

func (r *Runner) Kill(_ context.Context, id string, sig syscall.Signal) error {
	if r.ipc.IsConnected() {
		return r.ipc.Signal(id, sig.String())
	}
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
	if r.cmd != nil {
		err := r.cmd.Wait()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), nil
			}
			return -1, err
		}
		return 0, nil
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
		Config: state.Config, Mode: rt.ModeProot, IsolationType: "user-space",
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
