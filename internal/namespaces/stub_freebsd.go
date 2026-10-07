//go:build freebsd

// Package namespaces on FreeBSD is a no-op stub because FreeBSD does not
// expose Linux-style unshare/clone() flags, pivot_root, or persistent user
// namespace state. Containers run in ModeNative or under a jail, so this
// package's surface area is kept minimal.
package namespaces

import "errors"

// Manager is a no-op stand-in for the Linux Manager on FreeBSD builds.
type Manager struct{}

// Config mirrors the Linux configuration type. Fields are documented but
// unused on FreeBSD.
type Config struct {
	UID           uint32
	GID           uint32
	DenySetgroups bool
}

// NewManager returns a stub manager. No setup work happens because FreeBSD
// does not support Linux user namespaces.
func NewManager(root string) *Manager {
	return &Manager{}
}

// IsRootless always reports true on FreeBSD because users do not normally
// run Doki as root on a workstation or jail.
func IsRootless() bool {
	return true
}

// SetupUserNamespace is not supported on FreeBSD.
func (m *Manager) SetupUserNamespace(pid int, cfg *Config) error {
	return errors.New("user namespaces not supported on freebsd")
}

// DeletePersistentNamespace is not supported on FreeBSD.
func (m *Manager) DeletePersistentNamespace(id string) error {
	return nil
}
