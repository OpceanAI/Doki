// Package qemu provides QEMU-based VM backend.
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
	vmCfg  *dokivm.VMConfig
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

func (v *VMM) Create(_ context.Context, vmCfg *dokivm.VMConfig) (*dokivm.MicroVM, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	vm := &dokivm.MicroVM{
		ID:        vmCfg.ID,
		VMM:       v,
		State:     dokivm.VMStateCreated,
		CreatedAt: time.Now(),
	}
	v.vmCfg = vmCfg
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

	vcpus := 1
	if v.vmCfg != nil && v.vmCfg.CPUs > 0 {
		vcpus = v.vmCfg.CPUs
	}
	mem := 128
	if v.vmCfg != nil && v.vmCfg.Memory > 0 {
		mem = v.vmCfg.Memory
	}
	kernel := filepath.Join(v.cfg.KernelPath)
	if v.vmCfg != nil && v.vmCfg.Kernel != "" {
		kernel = v.vmCfg.Kernel
	}
	kernelArgs := "console=ttyS0 quiet doki.init=1 root=/dev/vda rw"
	if v.vmCfg != nil && v.vmCfg.KernelArgs != "" {
		kernelArgs = v.vmCfg.KernelArgs
	}
	rootfs := filepath.Join(v.cfg.WorkDir, vmID, "rootfs.ext4")
	if v.vmCfg != nil && v.vmCfg.Rootfs != "" {
		rootfs = v.vmCfg.Rootfs
	}

	args := []string{
		"-M", "microvm,accel=kvm:tcg",
		"-smp", fmt.Sprintf("%d", vcpus),
		"-m", fmt.Sprintf("%dM", mem),
		"-nodefaults", "-no-user-config", "-nographic",
		"-kernel", kernel,
		"-append", kernelArgs,
		"-drive", fmt.Sprintf("file=%s,format=raw,if=none,id=drive0", rootfs),
		"-device", "virtio-blk-device,drive=drive0",
	}

	netType := "user"
	if v.vmCfg != nil && v.vmCfg.Network != nil && v.vmCfg.Network.Type != "" {
		netType = v.vmCfg.Network.Type
	}
	switch netType {
	case "tap":
		tapName := "doki0"
		if v.vmCfg.Network != nil && v.vmCfg.Network.TapName != "" {
			tapName = v.vmCfg.Network.TapName
		}
		args = append(args, "-netdev", fmt.Sprintf("tap,id=net0,ifname=%s,script=no,downscript=no", tapName))
		args = append(args, "-device", "virtio-net-device,netdev=net0")
	case "none":
	default:
		args = append(args, "-netdev", "user,id=net0,hostfwd=tcp::8080-:80")
		args = append(args, "-device", "virtio-net-device,netdev=net0")
	}

	if v.vmCfg != nil && v.vmCfg.Vsock != nil {
		args = append(args, "-device", fmt.Sprintf("vhost-vsock-pci,guest-cid=%d", v.vmCfg.Vsock.CID))
	}

	if v.vmCfg != nil {
		for i, d := range v.vmCfg.ExtraDrives {
			id := fmt.Sprintf("drive%d", i+1)
			args = append(args, "-drive", fmt.Sprintf("file=%s,format=raw,if=none,id=%s", d.Path, id))
			args = append(args, "-device", fmt.Sprintf("virtio-blk-device,drive=%s", id))
		}
	}

	args = append(args, "-serial", "stdio")

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
		_ = cmd.Wait()
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
	_ = proc.Signal(syscall.SIGTERM)
	select {
	case <-time.After(timeout):
		_ = proc.Signal(syscall.SIGKILL)
	case <-ctx.Done():
		return ctx.Err()
	}
	v.mu.Lock()
	vm.State = dokivm.VMStateStopped
	v.mu.Unlock()
	return nil
}

func (v *VMM) Kill(_ context.Context, vmID string) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok || vm.PID == 0 {
		return nil
	}
	proc, _ := os.FindProcess(vm.PID)
	_ = proc.Signal(syscall.SIGKILL)
	v.mu.Lock()
	vm.State = dokivm.VMStateStopped
	v.mu.Unlock()
	return nil
}

func (v *VMM) State(_ context.Context, vmID string) (dokivm.VMState, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	vm, ok := v.vms[vmID]
	if !ok {
		return "", fmt.Errorf("vm %s not found", vmID)
	}
	return vm.State, nil
}

func (v *VMM) Exec(_ context.Context, _ string, _ []string, _ []string, _ bool) error {
	return fmt.Errorf("exec not implemented")
}
func (v *VMM) Attach(_ context.Context, _ string) error            { return nil }
func (v *VMM) Logs(_ context.Context, _ string) (io.Reader, error) { return nil, nil }
func (v *VMM) Stats(_ context.Context, _ string) (*dokivm.VMStats, error) {
	return &dokivm.VMStats{}, nil
}
func (v *VMM) Cleanup(_ context.Context, vmID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.vms, vmID)
	_ = os.RemoveAll(filepath.Join(v.cfg.WorkDir, vmID))
	return nil
}

var _ dokivm.VMM = (*VMM)(nil)
