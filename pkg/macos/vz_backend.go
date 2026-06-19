//go:build darwin && cgo

package macos

import "fmt"

type VZBackend struct {
	vms    map[string]*vzHandle
}

type vzHandle struct {
	id     string
	config *VMConfig
	state  string
}

func NewVZBackend() *VZBackend {
	return &VZBackend{
		vms: make(map[string]*vzHandle),
	}
}

func (b *VZBackend) Name() string         { return "vz" }
func (b *VZBackend) Available() bool       { return true }
func (b *VZBackend) MinVersion() string    { return "12.0" }

func (b *VZBackend) CreateVM(cfg *VMConfig) error {
	if _, exists := b.vms[cfg.ID]; exists {
		return fmt.Errorf("VM %s already exists", cfg.ID)
	}
	b.vms[cfg.ID] = &vzHandle{
		id:     cfg.ID,
		config: cfg,
		state:  "stopped",
	}
	return nil
}

func (b *VZBackend) StartVM(id string) error {
	h, ok := b.vms[id]
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}
	h.state = "running"
	return nil
}

func (b *VZBackend) StopVM(id string, timeoutSec int) error {
	h, ok := b.vms[id]
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}
	h.state = "stopped"
	return nil
}

func (b *VZBackend) DeleteVM(id string) error {
	if _, ok := b.vms[id]; !ok {
		return fmt.Errorf("VM %s not found", id)
	}
	delete(b.vms, id)
	return nil
}

func (b *VZBackend) VMStatus(id string) (string, error) {
	h, ok := b.vms[id]
	if !ok {
		return "", fmt.Errorf("VM %s not found", id)
	}
	return h.state, nil
}

func (b *VZBackend) ShareHostDir(hostPath, guestPath, tag string, readOnly bool) error {
	return nil
}

func (b *VZBackend) UnshareHostDir(tag string) error {
	return nil
}

func (b *VZBackend) ForwardPort(hostPort, guestPort int, proto string) error {
	return nil
}

func (b *VZBackend) RemoveForwardPort(hostPort int, proto string) error {
	return nil
}

func (b *VZBackend) Stats(id string) (*VMStats, error) {
	h, ok := b.vms[id]
	if !ok {
		return nil, fmt.Errorf("VM %s not found", id)
	}
	return &VMStats{
		State:       h.state,
		MemoryTotal: h.config.MemoryMB * 1024 * 1024,
	}, nil
}
