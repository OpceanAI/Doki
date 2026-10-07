//go:build freebsd

// Package cgroups provides cgroup management for containers.
//
// FreeBSD has no cgroup v2 filesystem; resource limits are enforced with
// rctl(8) and jails instead. This stub keeps the Manager API available so
// cross-platform callers compile unchanged, but every operation is a no-op
// and IsAvailable always reports false.
package cgroups

// Manager is a no-op stand-in for the Linux cgroup manager on FreeBSD.
type Manager struct {
	root    string
	enabled bool
}

// Config holds cgroup configuration.
type Config struct {
	CPUPeriod        uint64
	CPUQuota         int64
	CPUShares        uint64
	CpusetCpus       string
	CpusetMems       string
	Memory           int64
	MemorySwap       int64
	MemorySwappiness *uint64
	PidsLimit        int64
	BlkioWeight      uint16
	NanoCpus         int64
	OomKillDisable   bool
}

// PSIStats holds cgroup v2 pressure-stall information.
type PSIStats struct {
	Some float64 `json:"some"`
	Full float64 `json:"full"`
}

// NewManager returns a disabled manager; cgroups are unavailable on FreeBSD.
func NewManager(root string) *Manager {
	return &Manager{root: root, enabled: false}
}

// IsAvailable reports whether cgroup v2 limits can be enforced. Always false
// on FreeBSD, which uses rctl instead.
func (m *Manager) IsAvailable() bool { return false }

// Create is a no-op on FreeBSD.
func (m *Manager) Create(containerID string, cfg *Config) (string, error) { return "", nil }

// AddProcess is a no-op on FreeBSD.
func (m *Manager) AddProcess(containerID string, pid int) error { return nil }

// Freeze is a no-op on FreeBSD.
func (m *Manager) Freeze(containerID string) error { return nil }

// Thaw is a no-op on FreeBSD.
func (m *Manager) Thaw(containerID string) error { return nil }

// GetStats is a no-op on FreeBSD.
func (m *Manager) GetStats(containerID string) (map[string]interface{}, error) { return nil, nil }

// Update is a no-op on FreeBSD.
func (m *Manager) Update(containerID string, cfg *Config) error { return nil }

// GetStatsFull is a no-op on FreeBSD.
func (m *Manager) GetStatsFull(containerID string) (map[string]interface{}, error) {
	return nil, nil
}

// Destroy is a no-op on FreeBSD.
func (m *Manager) Destroy(containerID string) error { return nil }

// CgroupPath returns the (unused) cgroup path for a container.
func (m *Manager) CgroupPath(containerID string) string {
	return m.root + "/" + containerID
}

// Pressure reads a cgroup v2 pressure file. No-op on FreeBSD.
func (m *Manager) Pressure(containerID, file string) (PSIStats, error) {
	return PSIStats{}, nil
}
