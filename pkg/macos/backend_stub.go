//go:build !darwin

// Package macos provides macOS native virtualization backends.
package macos

// VZBackend is a stub for the macOS Virtualization framework backend.
type VZBackend struct{}

// QEMUBackend is a stub for the QEMU-based macOS virtualization backend.
type QEMUBackend struct{}

// NewVZBackend returns nil on non-darwin platforms.
func NewVZBackend() *VZBackend { return nil }

// NewQEMUBackend returns nil on non-darwin platforms.
func NewQEMUBackend() *QEMUBackend { return nil }

// Name returns "vz".
func (b *VZBackend) Name() string { return "vz" }

// Available reports that the backend is not available on this platform.
func (b *VZBackend) Available() bool { return false }

// MinVersion returns "N/A" as the backend is unavailable.
func (b *VZBackend) MinVersion() string { return "N/A" }

// CreateVM is a no-op stub.
func (b *VZBackend) CreateVM(_ *VMConfig) error { return nil }

// StartVM is a no-op stub.
func (b *VZBackend) StartVM(_ string) error { return nil }

// StopVM is a no-op stub.
func (b *VZBackend) StopVM(_ string, _ int) error { return nil }

// DeleteVM is a no-op stub.
func (b *VZBackend) DeleteVM(_ string) error { return nil }

// VMStatus returns empty state on unsupported platforms.
func (b *VZBackend) VMStatus(_ string) (string, error) { return "", nil }

// ShareHostDir is a no-op stub.
func (b *VZBackend) ShareHostDir(_, _, _ string, _ bool) error { return nil }

// UnshareHostDir is a no-op stub.
func (b *VZBackend) UnshareHostDir(_ string) error { return nil }

// ForwardPort is a no-op stub.
func (b *VZBackend) ForwardPort(_, _ int, _ string) error { return nil }

// RemoveForwardPort is a no-op stub.
func (b *VZBackend) RemoveForwardPort(_ int, _ string) error { return nil }

// Stats returns nil stats on unsupported platforms.
func (b *VZBackend) Stats(_ string) (*VMStats, error) { return nil, nil }

// Name returns "qemu".
func (b *QEMUBackend) Name() string { return "qemu" }

// Available reports that the backend is not available on this platform.
func (b *QEMUBackend) Available() bool { return false }

// MinVersion returns "N/A" as the backend is unavailable.
func (b *QEMUBackend) MinVersion() string { return "N/A" }

// CreateVM is a no-op stub.
func (b *QEMUBackend) CreateVM(_ *VMConfig) error { return nil }

// StartVM is a no-op stub.
func (b *QEMUBackend) StartVM(_ string) error { return nil }

// StopVM is a no-op stub.
func (b *QEMUBackend) StopVM(_ string, _ int) error { return nil }

// DeleteVM is a no-op stub.
func (b *QEMUBackend) DeleteVM(_ string) error { return nil }

// VMStatus returns empty state on unsupported platforms.
func (b *QEMUBackend) VMStatus(_ string) (string, error) { return "", nil }

// ShareHostDir is a no-op stub.
func (b *QEMUBackend) ShareHostDir(_, _, _ string, _ bool) error { return nil }

// UnshareHostDir is a no-op stub.
func (b *QEMUBackend) UnshareHostDir(_ string) error { return nil }

// ForwardPort is a no-op stub.
func (b *QEMUBackend) ForwardPort(_, _ int, _ string) error { return nil }

// RemoveForwardPort is a no-op stub.
func (b *QEMUBackend) RemoveForwardPort(_ int, _ string) error { return nil }

// Stats returns nil stats on unsupported platforms.
func (b *QEMUBackend) Stats(_ string) (*VMStats, error) { return nil, nil }
