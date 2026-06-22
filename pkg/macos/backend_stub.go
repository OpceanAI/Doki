//go:build !darwin

// Package macos provides macOS native virtualization backends.
// On non-darwin platforms, all backends are stubs that report
// unavailable and return errors when invoked.
package macos

import "errors"

// VZBackend is a stub for the macOS Virtualization framework backend.
type VZBackend struct{}

// QEMUBackend is a stub for the QEMU-based macOS virtualization backend.
type QEMUBackend struct{}

// newVZBackend returns a stub VZBackend on non-darwin platforms.
func newVZBackend() *VZBackend { return &VZBackend{} }

// newQEMUBackend returns a stub QEMUBackend on non-darwin platforms.
func newQEMUBackend() *QEMUBackend { return &QEMUBackend{} }

// errUnavailable is returned by all stub methods on non-darwin.
var errUnavailable = errors.New("macos: backend not available on this platform")

// VZBackend methods (stub)

func (b *VZBackend) Name() string                              { return "vz" }
func (b *VZBackend) Available() bool                           { return false }
func (b *VZBackend) MinVersion() string                        { return "N/A" }
func (b *VZBackend) CreateVM(_ *VMConfig) error                { return errUnavailable }
func (b *VZBackend) StartVM(_ string) error                    { return errUnavailable }
func (b *VZBackend) StopVM(_ string, _ int) error              { return errUnavailable }
func (b *VZBackend) DeleteVM(_ string) error                   { return errUnavailable }
func (b *VZBackend) VMStatus(_ string) (string, error)         { return "", errUnavailable }
func (b *VZBackend) ShareHostDir(_, _, _ string, _ bool) error { return errUnavailable }
func (b *VZBackend) UnshareHostDir(_ string) error             { return errUnavailable }
func (b *VZBackend) ForwardPort(_, _ int, _ string) error      { return errUnavailable }
func (b *VZBackend) RemoveForwardPort(_ int, _ string) error   { return errUnavailable }
func (b *VZBackend) Stats(_ string) (*VMStats, error)          { return nil, errUnavailable }

// QEMUBackend methods (stub)

func (b *QEMUBackend) Name() string                              { return "qemu" }
func (b *QEMUBackend) Available() bool                           { return false }
func (b *QEMUBackend) MinVersion() string                        { return "N/A" }
func (b *QEMUBackend) CreateVM(_ *VMConfig) error                { return errUnavailable }
func (b *QEMUBackend) StartVM(_ string) error                    { return errUnavailable }
func (b *QEMUBackend) StopVM(_ string, _ int) error              { return errUnavailable }
func (b *QEMUBackend) DeleteVM(_ string) error                   { return errUnavailable }
func (b *QEMUBackend) VMStatus(_ string) (string, error)         { return "", errUnavailable }
func (b *QEMUBackend) ShareHostDir(_, _, _ string, _ bool) error { return errUnavailable }
func (b *QEMUBackend) UnshareHostDir(_ string) error             { return errUnavailable }
func (b *QEMUBackend) ForwardPort(_, _ int, _ string) error      { return errUnavailable }
func (b *QEMUBackend) RemoveForwardPort(_ int, _ string) error   { return errUnavailable }
func (b *QEMUBackend) Stats(_ string) (*VMStats, error)          { return nil, errUnavailable }
