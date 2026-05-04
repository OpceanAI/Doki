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
	CPUPeriod    uint64
	CPUQuota     int64
	CPUShares    uint64
	CpusetCpus   string
	CpusetMems   string
	Memory       int64
	MemorySwap   int64
	MemorySwappiness *uint64
	PidsLimit    int64
	BlkioWeight  uint16
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
		// Default: swap == memory.
		if err := writeFile(cgroupPath, "memory.swap.max", strconv.FormatInt(cfg.Memory, 10)); err != nil {
			return err
		}
	}

	if cfg.PidsLimit > 0 {
		if err := writeFile(cgroupPath, "pids.max", strconv.FormatInt(cfg.PidsLimit, 10)); err != nil {
			return err
		}
	}

	return nil
}

func convertCPUSharesToV2Value(shares uint64) uint64 {
	if shares == 0 {
		return 100
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
			proc.Kill()
		}
	}

	return nil
}
