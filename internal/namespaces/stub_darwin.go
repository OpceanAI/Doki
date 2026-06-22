//go:build darwin
// +build darwin

// Package namespaces on macOS is a no-op stub because Darwin does not
// expose Linux-style unshare/clone() flags, pivot_root, or persistent
// user namespace state. ModeNative is the only execution mode available
// on macOS, so this package's surface area is kept minimal.
package namespaces

import "errors"

// Manager is a no-op stand-in for the Linux Manager on darwin builds.
type Manager struct{}

// Config mirrors the Linux configuration type. Fields are documented but
// unused on macOS.
type Config struct {
	UID           uint32
	GID           uint32
	DenySetgroups bool
}

// NewManager returns a stub manager. No setup work happens because
// macOS does not support Linux user namespaces.
func NewManager(root string) *Manager {
	return &Manager{}
}

// IsRootless always reports true on macOS because users do not normally
// run Doki as root on a desktop.
func IsRootless() bool {
	return true
}

// SetupUserNamespace is not supported on macOS.
func (m *Manager) SetupUserNamespace(pid int, cfg *Config) error {
	return errors.New("user namespaces not supported on darwin")
}

// DeletePersistentNamespace is not supported on macOS.
func (m *Manager) DeletePersistentNamespace(id string) error {
	return nil
}
