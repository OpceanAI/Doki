//go:build darwin && !cgo

package macos

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

type QEMUBackend struct {
	mu      sync.RWMutex
	vms     map[string]*qemuHandle
	qemuBin string
}

type qemuHandle struct {
	id     string
	config *VMConfig
	cmd    *exec.Cmd
	state  string
	mu     sync.Mutex
}

// newQEMUBackend creates a QEMUBackend. On darwin without cgo, this
// is the primary virtualization backend. It verifies that a QEMU
// binary is installed before reporting Available()=true.
func newQEMUBackend() *QEMUBackend {
	bin := ""
	for _, candidate := range []string{"qemu-system-aarch64", "qemu-system-x86_64"} {
		if path, err := exec.LookPath(candidate); err == nil {
			bin = path
			break
		}
	}
	return &QEMUBackend{
		vms:     make(map[string]*qemuHandle),
		qemuBin: bin,
	}
}

// newVZBackend creates a VZBackend stub on darwin without cgo (where
// vz_backend.go is excluded by its build tag). This ensures
// SelectBackend can always resolve the constructor.
func newVZBackend() *VZBackend {
	return &VZBackend{
		vms: make(map[string]*vzHandle),
	}
}

func (b *QEMUBackend) Name() string       { return "qemu" }
func (b *QEMUBackend) Available() bool    { return b.qemuBin != "" }
func (b *QEMUBackend) MinVersion() string { return "10.15" }

func (b *QEMUBackend) CreateVM(cfg *VMConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.vms[cfg.ID]; exists {
		return fmt.Errorf("VM %s already exists", cfg.ID)
	}
	b.vms[cfg.ID] = &qemuHandle{
		id:     cfg.ID,
		config: cfg,
		state:  "stopped",
	}
	return nil
}

func (b *QEMUBackend) StartVM(id string) error {
	b.mu.Lock()
	h, ok := b.vms[id]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	args := b.buildArgs(h.config)
	h.cmd = exec.Command(b.qemuBin, args...)
	if err := h.cmd.Start(); err != nil {
		return fmt.Errorf("start QEMU: %w", err)
	}
	h.state = "running"

	// Monitor goroutine: flip state to "exited" when the process
	// exits, so the VM status is accurate even after QEMU crashes
	// or shuts down.
	go func(cmd *exec.Cmd, handle *qemuHandle) {
		_ = cmd.Wait()
		handle.mu.Lock()
		handle.state = "exited"
		handle.mu.Unlock()
	}(h.cmd, h)

	return nil
}

func (b *QEMUBackend) StopVM(id string, timeoutSec int) error {
	b.mu.Lock()
	h, ok := b.vms[id]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cmd == nil || h.cmd.Process == nil {
		h.state = "stopped"
		return nil
	}

	// Graceful shutdown: SIGTERM, then wait up to timeoutSec, then SIGKILL.
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	_ = h.cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- h.cmd.Wait() }()

	select {
	case <-done:
		// Process exited gracefully.
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		// Timeout — force kill.
		_ = h.cmd.Process.Kill()
		<-done
	}

	h.state = "stopped"
	return nil
}

func (b *QEMUBackend) DeleteVM(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.vms[id]; !ok {
		return fmt.Errorf("VM %s not found", id)
	}
	delete(b.vms, id)
	return nil
}

func (b *QEMUBackend) VMStatus(id string) (string, error) {
	b.mu.RLock()
	h, ok := b.vms[id]
	b.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("VM %s not found", id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state, nil
}

func (b *QEMUBackend) ShareHostDir(hostPath, guestPath, tag string, readOnly bool) error {
	return nil
}

func (b *QEMUBackend) UnshareHostDir(tag string) error {
	return nil
}

func (b *QEMUBackend) ForwardPort(hostPort, guestPort int, proto string) error {
	return nil
}

func (b *QEMUBackend) RemoveForwardPort(hostPort int, proto string) error {
	return nil
}

func (b *QEMUBackend) Stats(id string) (*VMStats, error) {
	b.mu.RLock()
	h, ok := b.vms[id]
	b.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("VM %s not found", id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return &VMStats{
		State:       h.state,
		MemoryTotal: h.config.MemoryMB * 1024 * 1024,
	}, nil
}

// buildArgs constructs QEMU CLI arguments from the VM config. It is
// arch-aware: on arm64 it uses ttyAMA0 for console, on amd64 ttyS0.
// It includes an hvf:tcg fallback so QEMU works even without HVF
// support (e.g., in a VM).
func (b *QEMUBackend) buildArgs(cfg *VMConfig) []string {
	// Use hvf with tcg fallback so QEMU works with or without HVF.
	accel := "hvf:tcg"
	cpuModel := "host"
	console := "ttyAMA0"
	if runtime.GOARCH == "amd64" {
		console = "ttyS0"
	}

	args := []string{
		"-accel", accel,
		"-cpu", cpuModel,
		"-smp", fmt.Sprintf("%d", cfg.CPUs),
		"-m", fmt.Sprintf("%dM", cfg.MemoryMB),
	}

	if cfg.DiskPath != "" {
		args = append(args, "-drive", fmt.Sprintf("file=%s,format=qcow2", cfg.DiskPath))
	}

	netdev := "user,id=net0"
	for _, p := range cfg.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		netdev += fmt.Sprintf(",hostfwd=%s::%d-:%d", proto, p.HostPort, p.GuestPort)
	}
	args = append(args,
		"-netdev", netdev,
		"-device", "virtio-net-device,netdev=net0",
	)

	if len(cfg.Shares) > 0 {
		for _, s := range cfg.Shares {
			args = append(args, "-virtfs",
				fmt.Sprintf("local,path=%s,mount_tag=%s,security_model=none,multidevs=remap",
					s.HostPath, s.Tag))
		}
	}

	if cfg.KernelPath != "" {
		args = append(args, "-kernel", cfg.KernelPath)
		if cfg.InitrdPath != "" {
			args = append(args, "-initrd", cfg.InitrdPath)
		}
		args = append(args, "-append", fmt.Sprintf("console=%s root=/dev/vda rw", console))
	}

	args = append(args, "-nographic", "-serial", "mon:stdio")

	return args
}

// DetectQEMUBinary returns the path to the first available QEMU binary,
// or empty string if none is found.
func DetectQEMUBinary() string {
	for _, bin := range []string{"qemu-system-aarch64", "qemu-system-x86_64"} {
		if path, err := exec.LookPath(bin); err == nil {
			return path
		}
	}
	return ""
}
