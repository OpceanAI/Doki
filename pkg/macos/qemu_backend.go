//go:build darwin && !cgo

package macos

import (
	"fmt"
	"os/exec"
	"strings"
)

type QEMUBackend struct {
	vms     map[string]*qemuHandle
	qemuBin string
}

type qemuHandle struct {
	id     string
	config *VMConfig
	cmd    *exec.Cmd
	state  string
}

func NewQEMUBackend() *QEMUBackend {
	bin := "qemu-system-aarch64"
	if _, err := exec.LookPath(bin); err != nil {
		bin = "qemu-system-x86_64"
	}
	return &QEMUBackend{
		vms:     make(map[string]*qemuHandle),
		qemuBin: bin,
	}
}

func (b *QEMUBackend) Name() string         { return "qemu" }
func (b *QEMUBackend) Available() bool       { return b.qemuBin != "" }
func (b *QEMUBackend) MinVersion() string    { return "10.15" }

func (b *QEMUBackend) CreateVM(cfg *VMConfig) error {
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
	h, ok := b.vms[id]
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}

	args := b.buildArgs(h.config)
	h.cmd = exec.Command(b.qemuBin, args...)
	if err := h.cmd.Start(); err != nil {
		return fmt.Errorf("start QEMU: %w", err)
	}
	h.state = "running"
	return nil
}

func (b *QEMUBackend) StopVM(id string, timeoutSec int) error {
	h, ok := b.vms[id]
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}
	if h.cmd != nil && h.cmd.Process != nil {
		h.cmd.Process.Kill()
		h.cmd.Wait()
	}
	h.state = "stopped"
	return nil
}

func (b *QEMUBackend) DeleteVM(id string) error {
	if _, ok := b.vms[id]; !ok {
		return fmt.Errorf("VM %s not found", id)
	}
	delete(b.vms, id)
	return nil
}

func (b *QEMUBackend) VMStatus(id string) (string, error) {
	h, ok := b.vms[id]
	if !ok {
		return "", fmt.Errorf("VM %s not found", id)
	}
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
	h, ok := b.vms[id]
	if !ok {
		return nil, fmt.Errorf("VM %s not found", id)
	}
	return &VMStats{
		State:       h.state,
		MemoryTotal: h.config.MemoryMB * 1024 * 1024,
	}, nil
}

func (b *QEMUBackend) buildArgs(cfg *VMConfig) []string {
	args := []string{
		"-accel", "hvf",
		"-cpu", "host",
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
		args = append(args, "-append", "console=ttyAMA0 root=/dev/vda rw")
	}

	args = append(args, "-nographic", "-serial", "mon:stdio")

	return args
}

func DetectQEMUBinary() string {
	for _, bin := range []string{"qemu-system-aarch64", "qemu-system-x86_64"} {
		if path, err := exec.LookPath(bin); err == nil {
			return path
		}
	}
	return ""
}

func init() {
	_ = strings.TrimSpace
}
