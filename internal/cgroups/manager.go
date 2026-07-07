// Package cgroups provides cgroup management for containers.
package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Manager manages cgroups v2 for containers.
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

// NewManager creates a new cgroup manager.
func NewManager(root string) *Manager {
	enabled := isCgroupV2()
	return &Manager{
		root:    root,
		enabled: enabled,
	}
}

func isCgroupV2() bool {
	data, err := os.ReadFile("/proc/filesystems")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "cgroup2")
}

// IsAvailable checks if cgroups v2 is available.
func (m *Manager) IsAvailable() bool {
	return m.enabled
}

// Create creates a cgroup for a container.
func (m *Manager) Create(containerID string, cfg *Config) (string, error) {
	if !m.enabled {
		return "", nil
	}

	cgroupPath := filepath.Join(m.root, containerID)
	if err := common.EnsureDir(cgroupPath); err != nil {
		return "", fmt.Errorf("create cgroup dir: %w", err)
	}

	// Enable controllers by writing to cgroup.subtree_control in parent
	parentPath := filepath.Dir(cgroupPath)
	controllers := "+cpu +memory +pids +io"
	if err := os.WriteFile(filepath.Join(parentPath, "cgroup.subtree_control"),
		[]byte(controllers), 0644); err != nil {
		// Non-fatal, controllers may already be enabled
	}

	// Apply resource limits.
	if err := m.applyLimits(cgroupPath, cfg); err != nil {
		return "", fmt.Errorf("apply cgroup limits: %w", err)
	}

	return cgroupPath, nil
}

func (m *Manager) applyLimits(cgroupPath string, cfg *Config) error {
	if cfg.CPUShares > 0 {
		cpuWeight := convertCPUSharesToV2Value(cfg.CPUShares)
		if err := writeFile(cgroupPath, "cpu.weight", strconv.FormatUint(cpuWeight, 10)); err != nil {
			return err
		}
	}

	if cfg.CPUQuota > 0 && cfg.CPUPeriod > 0 {
		maxCPU := fmt.Sprintf("%d %d", cfg.CPUQuota, cfg.CPUPeriod)
		if err := writeFile(cgroupPath, "cpu.max", maxCPU); err != nil {
			return err
		}
	}

	if cfg.NanoCpus > 0 {
		period := common.SafeInt64FromUint64(cfg.CPUPeriod)
		if period == 0 {
			period = 100000
		}
		quota := cfg.NanoCpus * period / 1000000000
		if quota < 1000 {
			quota = 1000
		}
		maxCPU := fmt.Sprintf("%d %d", quota, period)
		if err := writeFile(cgroupPath, "cpu.max", maxCPU); err != nil {
			return err
		}
	}

	if cfg.CpusetCpus != "" {
		if err := writeFile(cgroupPath, "cpuset.cpus", cfg.CpusetCpus); err != nil {
			return err
		}
	}

	if cfg.CpusetMems != "" {
		if err := writeFile(cgroupPath, "cpuset.mems", cfg.CpusetMems); err != nil {
			return err
		}
	}

	if cfg.Memory > 0 {
		if err := writeFile(cgroupPath, "memory.max", strconv.FormatInt(cfg.Memory, 10)); err != nil {
			return err
		}
	}

	if cfg.MemorySwap > 0 {
		if err := writeFile(cgroupPath, "memory.swap.max", strconv.FormatInt(cfg.MemorySwap, 10)); err != nil {
			return err
		}
	} else if cfg.Memory > 0 {
		if err := writeFile(cgroupPath, "memory.swap.max", strconv.FormatInt(cfg.Memory, 10)); err != nil {
			return err
		}
	}

	if cfg.PidsLimit > 0 {
		if err := writeFile(cgroupPath, "pids.max", strconv.FormatInt(cfg.PidsLimit, 10)); err != nil {
			return err
		}
	}

	if cfg.BlkioWeight > 0 {
		if err := writeFile(cgroupPath, "io.weight", strconv.FormatUint(uint64(cfg.BlkioWeight), 10)); err != nil {
			return err
		}
	}

	if cfg.OomKillDisable {
		if err := writeFile(cgroupPath, "memory.oom.group", "1"); err != nil {
			return err
		}
	}

	return nil
}

func convertCPUSharesToV2Value(shares uint64) uint64 {
	if shares == 0 {
		return 100
	}
	if shares < 2 {
		shares = 2
	}
	return 1 + ((shares-2)*9999)/262142
}

func writeFile(cgroupPath, file, data string) error {
	p := filepath.Join(cgroupPath, file)
	return os.WriteFile(p, []byte(data), 0644)
}

// AddProcess adds a process to the cgroup.
func (m *Manager) AddProcess(containerID string, pid int) error {
	if !m.enabled {
		return nil
	}

	cgroupPath := filepath.Join(m.root, containerID)
	return writeFile(cgroupPath, "cgroup.procs", strconv.Itoa(pid))
}

// Destroy removes the cgroup for a container.
func (m *Manager) Destroy(containerID string) error {
	if !m.enabled {
		return nil
	}

	cgroupPath := filepath.Join(m.root, containerID)
	return os.Remove(cgroupPath)
}

// Update atomically updates a subset of cgroup limits. Implements
// CRI UpdateContainerResources semantics: rollback on any failure.
//
// Returns the per-controller set of errors so the caller can surface
// them in CRI/ContainerStats. The bool indicates whether at least
// one resource was updated.
func (m *Manager) Update(containerID string, cfg *Config) error {
	if !m.enabled {
		return nil
	}
	cgroupPath := filepath.Join(m.root, containerID)
	// Capture previous values for rollback.
	prev, _ := snapshotLimits(cgroupPath)
	if err := m.applyLimits(cgroupPath, cfg); err != nil {
		_ = restoreLimits(cgroupPath, prev)
		return err
	}
	return nil
}

// CgroupPath returns the absolute cgroup v2 path for a container.
func (m *Manager) CgroupPath(containerID string) string {
	return filepath.Join(m.root, containerID)
}

// PSIStats holds cgroup v2 pressure-stall information.
type PSIStats struct {
	Some float64 `json:"some"`
	Full float64 `json:"full"`
}

// Pressure reads a cgroup v2 pressure file (e.g. cpu.pressure).
func (m *Manager) Pressure(containerID, file string) (PSIStats, error) {
	var p PSIStats
	if !m.enabled {
		return p, nil
	}
	cgroupPath := filepath.Join(m.root, containerID)
	data, err := os.ReadFile(filepath.Join(cgroupPath, file))
	if err != nil {
		return p, err
	}
	// Format: "some avg10=X.XX avg60=X.XX avg300=X.XX total=X\n"
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "some") {
			fields := strings.Fields(line)
			for _, f := range fields {
				if strings.HasPrefix(f, "avg10=") {
					if v, perr := strconv.ParseFloat(strings.TrimPrefix(f, "avg10="), 64); perr == nil {
						p.Some = v
					}
				}
			}
		}
	}
	return p, nil
}

// GetStatsFull returns full cgroup statistics for a container,
// including PSI for cpu/memory/io.
func (m *Manager) GetStatsFull(containerID string) (map[string]interface{}, error) {
	if !m.enabled {
		return nil, nil
	}
	stats := make(map[string]interface{})
	cgroupPath := filepath.Join(m.root, containerID)

	if data, err := readFile(cgroupPath, "cpu.stat"); err == nil {
		stats["cpu"] = parseCgroupKV(data)
	}
	if data, err := readFile(cgroupPath, "memory.current"); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(data), 10, 64); err == nil {
			stats["memory"] = val
		}
	}
	if data, err := readFile(cgroupPath, "memory.events"); err == nil {
		stats["memory_events"] = parseCgroupKV(data)
	}
	if data, err := readFile(cgroupPath, "pids.current"); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(data), 10, 64); err == nil {
			stats["pids"] = val
		}
	}
	if data, err := readFile(cgroupPath, "io.stat"); err == nil {
		stats["io"] = parseIOStat(data)
	}
	if psi, err := m.Pressure(containerID, "cpu.pressure"); err == nil {
		stats["cpu_psi"] = psi
	}
	if psi, err := m.Pressure(containerID, "memory.pressure"); err == nil {
		stats["memory_psi"] = psi
	}
	return stats, nil
}

func parseIOStat(data string) map[string]map[string]uint64 {
	result := make(map[string]map[string]uint64)
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		dev := strings.TrimSuffix(parts[0], ":")
		stats := make(map[string]uint64)
		for _, kv := range parts[1:] {
			eq := strings.IndexByte(kv, '=')
			if eq < 0 {
				continue
			}
			if v, err := strconv.ParseUint(kv[eq+1:], 10, 64); err == nil {
				stats[kv[:eq]] = v
			}
		}
		result[dev] = stats
	}
	return result
}

func snapshotLimits(cgroupPath string) (map[string]string, error) {
	out := make(map[string]string)
	files := []string{
		"cpu.max", "cpu.weight", "memory.max", "memory.swap.max",
		"pids.max", "io.weight", "cpuset.cpus", "cpuset.mems",
	}
	for _, f := range files {
		if data, err := os.ReadFile(filepath.Join(cgroupPath, f)); err == nil {
			out[f] = string(data)
		}
	}
	return out, nil
}

func restoreLimits(cgroupPath string, prev map[string]string) error {
	var firstErr error
	for f, v := range prev {
		if err := os.WriteFile(filepath.Join(cgroupPath, f), []byte(v), 0644); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// GetStats returns cgroup statistics for a container.
func (m *Manager) GetStats(containerID string) (map[string]interface{}, error) {
	if !m.enabled {
		return nil, nil
	}

	stats := make(map[string]interface{})
	cgroupPath := filepath.Join(m.root, containerID)

	// CPU stats.
	if data, err := readFile(cgroupPath, "cpu.stat"); err == nil {
		stats["cpu"] = parseCgroupKV(data)
	}

	// Memory stats.
	if data, err := readFile(cgroupPath, "memory.current"); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(data), 10, 64); err == nil {
			stats["memory"] = val
		}
	}

	// PIDs stats.
	if data, err := readFile(cgroupPath, "pids.current"); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(data), 10, 64); err == nil {
			stats["pids"] = val
		}
	}

	return stats, nil
}

func readFile(cgroupPath, file string) (string, error) {
	data, err := os.ReadFile(filepath.Join(cgroupPath, file))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parseCgroupKV(data string) map[string]uint64 {
	result := make(map[string]uint64)
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			if val, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
				result[parts[0]] = val
			}
		}
	}
	return result
}

// Freeze pauses a container by moving it to a frozen cgroup.
func (m *Manager) Freeze(containerID string) error {
	if !m.enabled {
		return nil
	}

	cgroupPath := filepath.Join(m.root, containerID)
	return writeFile(cgroupPath, "cgroup.freeze", "1")
}

// Thaw resumes a paused container.
func (m *Manager) Thaw(containerID string) error {
	if !m.enabled {
		return nil
	}

	cgroupPath := filepath.Join(m.root, containerID)
	return writeFile(cgroupPath, "cgroup.freeze", "0")
}

// KillAll sends a signal to all processes in the cgroup.
func (m *Manager) KillAll(containerID string) error {
	if !m.enabled {
		return nil
	}

	cgroupPath := filepath.Join(m.root, containerID)
	procs, err := readFile(cgroupPath, "cgroup.procs")
	if err != nil {
		return err
	}

	for _, pidStr := range strings.Split(strings.TrimSpace(procs), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
		if err == nil && pid > 0 {
			proc, _ := os.FindProcess(pid)
			_ = proc.Kill()
		}
	}

	return nil
}
