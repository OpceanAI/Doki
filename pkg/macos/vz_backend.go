//go:build darwin && cgo

package macos

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Virtualization -framework Foundation
#include "vz_bridge.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// VZBackend implements macOS Virtualization.framework (VZ) backend
// using a cgo bridge to the native VZ APIs. This is the real
// implementation that creates and runs macOS/Linux VMs via the
// Virtualization.framework.
//
// Requirements:
//   - macOS 11+ (Big Sur)
//   - CGO_ENABLED=1
//   - com.apple.vm.hypervisor entitlement for HVF
//   - Xcode for Objective-C compilation
type VZBackend struct {
	mu  sync.RWMutex
	vms map[string]*vzHandle
}

type vzHandle struct {
	id     string
	config *VMConfig
	vm     C.vz_vm_t
	cfg    C.vz_config_t
	state  string
	mu     sync.Mutex
}

// newVZBackend creates a VZBackend.
func newVZBackend() *VZBackend {
	return &VZBackend{
		vms: make(map[string]*vzHandle),
	}
}

// newQEMUBackend creates a QEMUBackend on darwin+cgo builds (where
// qemu_backend.go is excluded by its build tag). This ensures
// SelectBackend can always resolve the constructor.
func newQEMUBackend() *QEMUBackend {
	return &QEMUBackend{
		vms: make(map[string]*qemuHandle),
	}
}

func (b *VZBackend) Name() string       { return "vz" }
func (b *VZBackend) Available() bool    { return C.vz_capabilities_available() == 1 }
func (b *VZBackend) MinVersion() string { return "11.0" }

func (b *VZBackend) CreateVM(cfg *VMConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.vms[cfg.ID]; exists {
		return fmt.Errorf("VM %s already exists", cfg.ID)
	}

	// Determine bootloader type.
	bootloaderType := "linux"
	if cfg.Backend == "macos" {
		bootloaderType = "macos"
	}

	// Create VZ config via cgo bridge.
	diskPath := C.CString(cfg.DiskPath)
	defer C.free(unsafe.Pointer(diskPath))
	kernelPath := C.CString(cfg.KernelPath)
	defer C.free(unsafe.Pointer(kernelPath))
	initrdPath := C.CString(cfg.InitrdPath)
	defer C.free(unsafe.Pointer(initrdPath))
	btType := C.CString(bootloaderType)
	defer C.free(unsafe.Pointer(btType))

	memoryBytes := C.int64_t(cfg.MemoryMB * 1024 * 1024)
	diskSizeBytes := C.int64_t(cfg.DiskSizeGB * 1024 * 1024 * 1024)

	var errMsg *C.char
	vzCfg := C.vz_create_config(C.int(cfg.CPUs), memoryBytes, diskPath, diskSizeBytes,
		kernelPath, initrdPath, btType, &errMsg)
	if vzCfg == nil {
		msg := "unknown error"
		if errMsg != nil {
			msg = C.GoString(errMsg)
			C.vz_free_string(errMsg)
		}
		return fmt.Errorf("VZ create config: %s", msg)
	}

	// Add file shares.
	for _, s := range cfg.Shares {
		hostPath := C.CString(s.HostPath)
		guestPath := C.CString(s.GuestPath)
		tag := C.CString(s.Tag)
		ro := 0
		if s.ReadOnly {
			ro = 1
		}
		var shareErr *C.char
		if rc := C.vz_add_file_share(vzCfg, hostPath, guestPath, tag, C.int(ro), &shareErr); rc != 0 {
			msg := "unknown error"
			if shareErr != nil {
				msg = C.GoString(shareErr)
				C.vz_free_string(shareErr)
			}
			C.vz_delete_config(vzCfg)
			C.free(unsafe.Pointer(hostPath))
			C.free(unsafe.Pointer(guestPath))
			C.free(unsafe.Pointer(tag))
			return fmt.Errorf("VZ add share: %s", msg)
		}
		C.free(unsafe.Pointer(hostPath))
		C.free(unsafe.Pointer(guestPath))
		C.free(unsafe.Pointer(tag))
	}

	// Add NAT network.
	var natErr *C.char
	if rc := C.vz_add_nat_network(vzCfg, &natErr); rc != 0 {
		msg := "unknown error"
		if natErr != nil {
			msg = C.GoString(natErr)
			C.vz_free_string(natErr)
		}
		C.vz_delete_config(vzCfg)
		return fmt.Errorf("VZ add NAT: %s", msg)
	}

	// Add port forwards.
	for _, p := range cfg.Ports {
		proto := C.CString(p.Protocol)
		if p.Protocol == "" {
			proto = C.CString("tcp")
		}
		var pfErr *C.char
		if rc := C.vz_add_port_forward(vzCfg, C.int(p.HostPort), C.int(p.GuestPort), proto, &pfErr); rc != 0 {
			msg := "unknown error"
			if pfErr != nil {
				msg = C.GoString(pfErr)
				C.vz_free_string(pfErr)
			}
			C.free(unsafe.Pointer(proto))
			C.vz_delete_config(vzCfg)
			return fmt.Errorf("VZ add port forward: %s", msg)
		}
		C.free(unsafe.Pointer(proto))
	}

	// Enable Rosetta if requested.
	if cfg.Rosetta {
		var rosErr *C.char
		if rc := C.vz_enable_rosetta(vzCfg, &rosErr); rc != 0 {
			// Rosetta is optional; log but don't fail.
			C.vz_free_string(rosErr)
		}
	}

	// Validate the config.
	var valErr *C.char
	if rc := C.vz_validate(vzCfg, &valErr); rc != 0 {
		msg := "unknown error"
		if valErr != nil {
			msg = C.GoString(valErr)
			C.vz_free_string(valErr)
		}
		C.vz_delete_config(vzCfg)
		return fmt.Errorf("VZ validate: %s", msg)
	}

	// Create the VM.
	var vmErr *C.char
	vm := C.vz_create_vm(vzCfg, &vmErr)
	if vm == nil {
		msg := "unknown error"
		if vmErr != nil {
			msg = C.GoString(vmErr)
			C.vz_free_string(vmErr)
		}
		C.vz_delete_config(vzCfg)
		return fmt.Errorf("VZ create VM: %s", msg)
	}

	b.vms[cfg.ID] = &vzHandle{
		id:     cfg.ID,
		config: cfg,
		vm:     vm,
		cfg:    vzCfg,
		state:  "stopped",
	}
	return nil
}

func (b *VZBackend) StartVM(id string) error {
	b.mu.RLock()
	h, ok := b.vms[id]
	b.mu.RUnlock()
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	var errMsg *C.char
	if rc := C.vz_start_vm(h.vm, &errMsg); rc != 0 {
		msg := "unknown error"
		if errMsg != nil {
			msg = C.GoString(errMsg)
			C.vz_free_string(errMsg)
		}
		return fmt.Errorf("VZ start: %s", msg)
	}
	h.state = "running"
	return nil
}

func (b *VZBackend) StopVM(id string, timeoutSec int) error {
	b.mu.RLock()
	h, ok := b.vms[id]
	b.mu.RUnlock()
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	var errMsg *C.char
	if rc := C.vz_stop_vm(h.vm, C.int(timeoutSec), &errMsg); rc != 0 {
		msg := "unknown error"
		if errMsg != nil {
			msg = C.GoString(errMsg)
			C.vz_free_string(errMsg)
		}
		return fmt.Errorf("VZ stop: %s", msg)
	}
	h.state = "stopped"
	return nil
}

func (b *VZBackend) DeleteVM(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	h, ok := b.vms[id]
	if !ok {
		return fmt.Errorf("VM %s not found", id)
	}
	C.vz_delete_vm(h.vm)
	C.vz_delete_config(h.cfg)
	delete(b.vms, id)
	return nil
}

func (b *VZBackend) VMStatus(id string) (string, error) {
	b.mu.RLock()
	h, ok := b.vms[id]
	b.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("VM %s not found", id)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Query native state for accuracy.
	nativeState := C.vz_vm_state(h.vm)
	switch int(nativeState) {
	case 0:
		h.state = "stopped"
	case 1:
		h.state = "running"
	case 2:
		h.state = "paused"
	case 3:
		h.state = "starting"
	}
	return h.state, nil
}

func (b *VZBackend) ShareHostDir(hostPath, guestPath, tag string, readOnly bool) error {
	// VZ requires the VM to be reconfigured to add shares to a
	// running VM. For now, shares are configured at CreateVM time.
	// Dynamic share addition requires stopping and restarting the VM.
	return nil
}

func (b *VZBackend) UnshareHostDir(tag string) error {
	return nil
}

func (b *VZBackend) ForwardPort(hostPort, guestPort int, proto string) error {
	// Port forwarding via VZ NAT is configured at CreateVM time.
	// Dynamic port forwarding requires pf rules on the host.
	return nil
}

func (b *VZBackend) RemoveForwardPort(hostPort int, proto string) error {
	return nil
}

func (b *VZBackend) Stats(id string) (*VMStats, error) {
	b.mu.RLock()
	h, ok := b.vms[id]
	b.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("VM %s not found", id)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	var cStats C.vz_stats_t
	var errMsg *C.char
	if rc := C.vz_vm_stats(h.vm, &cStats, &errMsg); rc != 0 {
		msg := "unknown error"
		if errMsg != nil {
			msg = C.GoString(errMsg)
			C.vz_free_string(errMsg)
		}
		return nil, fmt.Errorf("VZ stats: %s", msg)
	}

	return &VMStats{
		State:       h.state,
		CPUUsage:    float64(cStats.cpu_usage),
		MemoryUsage: int64(cStats.memory_usage),
		MemoryTotal: int64(cStats.memory_total),
		DiskRead:    int64(cStats.disk_read),
		DiskWrite:   int64(cStats.disk_write),
		NetRx:       int64(cStats.net_rx),
		NetTx:       int64(cStats.net_tx),
	}, nil
}
