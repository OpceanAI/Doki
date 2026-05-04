package qemu

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/dokivm"
)

// VMM implements the VMM interface using QEMU microvm machine type.
type VMM struct {
	mu     sync.RWMutex
	vms    map[string]*dokivm.MicroVM
	cfg    *dokivm.VMMConfig
	binary string
}

func New(cfg *dokivm.VMMConfig) (*VMM, error) {
	binary, err := exec.LookPath("qemu-system-aarch64")
	if err != nil {
		binary, err = exec.LookPath("qemu-system-x86_64")
		if err != nil {
			return nil, fmt.Errorf("qemu not found: %w", err)
		}
	}
	return &VMM{
		vms:    make(map[string]*dokivm.MicroVM),
		cfg:    cfg,
		binary: binary,
	}, nil
}

func init() {
	dokivm.RegisterBackend("qemu", func(cfg *dokivm.VMMConfig) (dokivm.VMM, error) {
		return New(cfg)
	})
}

func (v *VMM) Name() string { return "qemu" }

func (v *VMM) Create(ctx context.Context, vmCfg *dokivm.VMConfig) (*dokivm.MicroVM, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	vm := &dokivm.MicroVM{
		ID:        vmCfg.ID,
		VMM:       v,
		State:     dokivm.VMStateCreated,
		CreatedAt: time.Now(),
	}
	v.vms[vm.ID] = vm
	return vm, nil
}

func (v *VMM) Start(ctx context.Context, vmID string) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok {
		return fmt.Errorf("vm %s not found", vmID)
	}

	args := []string{
		"-M", "microvm,accel=kvm:tcg",
		"-m", "128M",
		"-nodefaults", "-no-user-config", "-nographic",
		"-kernel", filepath.Join(v.cfg.KernelPath),
		"-append", "console=ttyS0 quiet doki.init=1 root=/dev/vda rw",
		"-drive", fmt.Sprintf("file=%s/rootfs.ext4,format=raw,if=none,id=drive0", filepath.Join(v.cfg.WorkDir, vmID)),
		"-device", "virtio-blk-device,drive=drive0",
		"-netdev", "user,id=net0,hostfwd=tcp::8080-:80",
		"-device", "virtio-net-device,netdev=net0",
		"-serial", "stdio",
	}

	cmd := exec.CommandContext(ctx, v.binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		vm.State = dokivm.VMStateFailed
		return fmt.Errorf("qemu start: %w", err)
	}

	vm.PID = cmd.Process.Pid
	vm.State = dokivm.VMStateRunning
	vm.StartedAt = time.Now()

	go func() {
		cmd.Wait()
		v.mu.Lock()
		if vm, ok := v.vms[vmID]; ok {
			vm.State = dokivm.VMStateStopped
		}
		v.mu.Unlock()
	}()

	return nil
}

func (v *VMM) Stop(ctx context.Context, vmID string, timeout time.Duration) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok || vm.PID == 0 {
		return nil
	}
	proc, _ := os.FindProcess(vm.PID)
	proc.Signal(syscall.SIGTERM)
	select {
	case <-time.After(timeout):
		proc.Signal(syscall.SIGKILL)
	case <-ctx.Done():
		return ctx.Err()
	}
	vm.State = dokivm.VMStateStopped
	return nil
}

func (v *VMM) Kill(ctx context.Context, vmID string) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok || vm.PID == 0 {
		return nil
	}
	proc, _ := os.FindProcess(vm.PID)
	proc.Signal(syscall.SIGKILL)
	vm.State = dokivm.VMStateStopped
	return nil
}

func (v *VMM) State(ctx context.Context, vmID string) (dokivm.VMState, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	vm, ok := v.vms[vmID]
	if !ok {
		return "", fmt.Errorf("vm %s not found", vmID)
	}
	return vm.State, nil
}

func (v *VMM) Exec(ctx context.Context, vmID string, cmd []string, env []string, tty bool) error {
	return fmt.Errorf("exec not implemented")
}
func (v *VMM) Attach(ctx context.Context, vmID string) error                    { return nil }
func (v *VMM) Logs(ctx context.Context, vmID string) (io.Reader, error)        { return nil, nil }
func (v *VMM) Stats(ctx context.Context, vmID string) (*dokivm.VMStats, error) { return &dokivm.VMStats{}, nil }
func (v *VMM) Cleanup(ctx context.Context, vmID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.vms, vmID)
	os.RemoveAll(filepath.Join(v.cfg.WorkDir, vmID))
	return nil
}

var _ dokivm.VMM = (*VMM)(nil)
