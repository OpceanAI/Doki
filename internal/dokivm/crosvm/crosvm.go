package crosvm

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

// VMM implements the VMM interface using crosvm.
type VMM struct {
	mu      sync.RWMutex
	vms     map[string]*dokivm.MicroVM
	configs map[string]*dokivm.VMConfig
	cfg     *dokivm.VMMConfig
	binary  string
}

// New creates a new crosvm VMM backend.
func New(cfg *dokivm.VMMConfig) (*VMM, error) {
	binary, err := exec.LookPath("crosvm")
	if err != nil {
		return nil, fmt.Errorf("crosvm not found in PATH: %w", err)
	}
	return &VMM{
		vms:     make(map[string]*dokivm.MicroVM),
		configs: make(map[string]*dokivm.VMConfig),
		cfg:     cfg,
		binary:  binary,
	}, nil
}

func init() {
	dokivm.RegisterBackend("crosvm", func(cfg *dokivm.VMMConfig) (dokivm.VMM, error) {
		return New(cfg)
	})
}

func (v *VMM) Name() string { return "crosvm" }

func (v *VMM) Create(ctx context.Context, vmCfg *dokivm.VMConfig) (*dokivm.MicroVM, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	vm := &dokivm.MicroVM{
		ID:        vmCfg.ID,
		VMM:       v,
		State:     dokivm.VMStateCreated,
		CreatedAt: time.Now(),
	}
	if vmCfg.Vsock != nil {
		vm.CID = vmCfg.Vsock.CID
	}
	v.vms[vm.ID] = vm
	v.configs[vm.ID] = vmCfg
	return vm, nil
}

func (v *VMM) Start(ctx context.Context, vmID string) error {
	v.mu.Lock()
	vm, ok := v.vms[vmID]
	v.mu.Unlock()
	if !ok {
		return fmt.Errorf("vm %s not found", vmID)
	}

	// Use the VMConfig stored when Create was called.
	v.mu.RLock()
	vmCfg, ok := v.configs[vmID]
	v.mu.RUnlock()
	if !ok {
		return fmt.Errorf("vm %s config not found", vmID)
	}

	args := []string{"run"}

	if vmCfg.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%d", vmCfg.CPUs))
	}
	if vmCfg.Memory > 0 {
		args = append(args, "--mem", fmt.Sprintf("%d", vmCfg.Memory))
	}
	if vmCfg.Kernel != "" {
		args = append(args, vmCfg.Kernel)
	}
	if vmCfg.Rootfs != "" {
		args = append(args, "--rwroot", vmCfg.Rootfs)
	}
	if vmCfg.Initrd != "" {
		args = append(args, "--initrd", vmCfg.Initrd)
	}
	if vmCfg.Console {
		args = append(args, "--serial", "type=stdout,hardware=virtio-console,console=true,stdin=true")
	}
	if vmCfg.KernelArgs != "" {
		args = append(args, "-p", vmCfg.KernelArgs)
	}

	// Network via TAP.
	if vmCfg.Network != nil && vmCfg.Network.TapName != "" {
		args = append(args, "--net", "tap-name="+vmCfg.Network.TapName)
	}

	// Vsock.
	if vmCfg.Vsock != nil && vmCfg.Vsock.CID > 0 {
		args = append(args, "--vsock", fmt.Sprintf("cid=%d", vmCfg.Vsock.CID))
	}

	// Extra drives.
	for _, drive := range vmCfg.ExtraDrives {
		flag := "--rwdisk"
		if drive.ReadOnly {
			flag = "--disk"
		}
		args = append(args, flag, drive.Path)
	}

	if vmCfg.KernelArgs == "" {
		args = append(args, "-p", "console=ttyS0 root=/dev/vda rw quiet doki.init=1")
	}

	cmd := exec.CommandContext(ctx, v.binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		vm.State = dokivm.VMStateFailed
		return fmt.Errorf("crosvm start: %w", err)
	}

	vm.PID = cmd.Process.Pid
	vm.State = dokivm.VMStateRunning
	vm.StartedAt = time.Now()

	// Monitor process.
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
	if !ok {
		return fmt.Errorf("vm %s not found", vmID)
	}
	if vm.PID == 0 {
		return nil
	}
	proc, _ := os.FindProcess(vm.PID)
	proc.Signal(syscall.SIGTERM)
	// Wait for graceful shutdown.
	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			proc.Signal(syscall.SIGKILL)
			vm.State = dokivm.VMStateStopped
			return nil
		case <-ticker.C:
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				vm.State = dokivm.VMStateStopped
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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

func (v *VMM) Exec(ctx context.Context, vmID string, cmd []string, env []string, tty bool) error     {
	return fmt.Errorf("exec via vsock not implemented")
}
func (v *VMM) Attach(ctx context.Context, vmID string) error                                          {
	return fmt.Errorf("attach not implemented")
}
func (v *VMM) Logs(ctx context.Context, vmID string) (io.Reader, error)                              {
	return nil, fmt.Errorf("logs not implemented")
}
func (v *VMM) Stats(ctx context.Context, vmID string) (*dokivm.VMStats, error)                       {
	return &dokivm.VMStats{}, nil
}
func (v *VMM) Cleanup(ctx context.Context, vmID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	vm, ok := v.vms[vmID]
	if !ok {
		return nil
	}
	if vm.TapDevice != "" {
		exec.Command("ip", "link", "del", vm.TapDevice).Run()
	}
	delete(v.vms, vmID)
	delete(v.configs, vmID)
	os.RemoveAll(filepath.Join(v.cfg.WorkDir, vmID))
	return nil
}

// IsAvailable checks if crosvm is available on this system.
func IsAvailable() bool {
	_, err := exec.LookPath("crosvm")
	return err == nil
}

// Ensure var is used.
var _ dokivm.VMM = (*VMM)(nil)
