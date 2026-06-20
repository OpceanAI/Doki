package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)


// CNIPlugin represents a CNI (Container Network Interface) plugin.
type CNIPlugin struct {
	Name    string
	Path    string
	Version string
}

// CNIManager manages CNI plugins and network configuration.
type CNIManager struct {
	mu        sync.RWMutex
	pluginDir string
	configDir string
	plugins   map[string]*CNIPlugin
}

// NewCNIManager creates a CNI manager.
func NewCNIManager(pluginDir, configDir string) *CNIManager {
	return &CNIManager{
		pluginDir: pluginDir,
		configDir: configDir,
		plugins:   make(map[string]*CNIPlugin),
	}
}

// LoadPlugins discovers available CNI plugins.
func (c *CNIManager) LoadPlugins() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !pathExists(c.pluginDir) {
		return fmt.Errorf("CNI plugin dir not found: %s", c.pluginDir)
	}

	entries, err := os.ReadDir(c.pluginDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		c.plugins[entry.Name()] = &CNIPlugin{
			Name: entry.Name(),
			Path: filepath.Join(c.pluginDir, entry.Name()),
		}
	}
	return nil
}

// AvailablePlugins returns list of available plugins.
func (c *CNIManager) AvailablePlugins() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var names []string
	for name := range c.plugins {
		names = append(names, name)
	}
	return names
}

// DefaultCNIPlugins returns standard CNI plugin names.
func DefaultCNIPlugins() []string {
	return []string{
		"bridge",
		"host-local",
		"portmap",
		"loopback",
		"bandwidth",
		"firewall",
		"macvlan",
		"ipvlan",
		"dhcp",
		"static",
		"tuning",
		"vlan",
	}
}

// IsCNIAvailable checks if CNI plugins are installed.
func IsCNIAvailable() bool {
	dirs := []string{
		"/usr/lib/cni",
		"/usr/libexec/cni",
		"/opt/cni/bin",
		filepath.Join(os.Getenv("HOME"), ".doki/cni/bin"),
	}
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			return true
		}
	}
	return false
}

// ─── Pasta / rootless networking ──────────────────────────────────

// PastaManager manages pasta-based rootless networking.
type PastaManager struct {
	pastaPath string
}

// NewPastaManager creates a pasta manager.
func NewPastaManager() *PastaManager {
	path, _ := exec.LookPath("pasta")
	if path == "" {
		path, _ = exec.LookPath("passt")
	}
	return &PastaManager{pastaPath: path}
}

// IsAvailable checks if pasta is available.
func (p *PastaManager) IsAvailable() bool {
	return p.pastaPath != ""
}

// Start starts pasta for a container PID.
func (p *PastaManager) Start(pid int, opts ...string) (*exec.Cmd, error) {
	if !p.IsAvailable() {
		return nil, fmt.Errorf("pasta not available")
	}
	args := append([]string{
		"--pid", fmt.Sprintf("%d", pid),
		"--tcp-ports", "auto",
		"--udp-ports", "auto",
	}, opts...)
	cmd := exec.Command(p.pastaPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// ─── Firewall management ───────────────────────────────────────────

// FirewallBackend represents a firewall backend.
type FirewallBackend string

const (
	FirewallIptables  FirewallBackend = "iptables"
	FirewallNftables  FirewallBackend = "nftables"
)

// FirewallManager manages firewall rules for containers.
type FirewallManager struct {
	backend FirewallBackend
}

// NewFirewallManager creates a firewall manager.
func NewFirewallManager(backend FirewallBackend) *FirewallManager {
	return &FirewallManager{backend: backend}
}

// IsNftablesAvailable checks if nftables is available.
func IsNftablesAvailable() bool {
	_, err := exec.LookPath("nft")
	return err == nil
}

// IsIptablesAvailable checks if iptables is available.
func IsIptablesAvailable() bool {
	_, err := exec.LookPath("iptables")
	return err == nil
}

// DetectFirewallBackend auto-detects the best firewall backend.
func DetectFirewallBackend() FirewallBackend {
	if IsNftablesAvailable() {
		return FirewallNftables
	}
	return FirewallIptables
}

// AddPortMapping adds a port mapping rule.
func (f *FirewallManager) AddPortMapping(containerIP string, hostPort, containerPort int, proto string) error {
	if err := f.ensureChains(); err != nil {
		return err
	}
	if f.backend == FirewallNftables {
		return f.addNftablesPortMapping(containerIP, hostPort, containerPort, proto)
	}
	return f.addIptablesPortMapping(containerIP, hostPort, containerPort, proto)
}

// ensureChains creates the DOKI chain in the nat table and wires PREROUTING/OUTPUT into it.
// Idempotent: a chain that already exists returns an error from the binary but we ignore it.
func (f *FirewallManager) ensureChains() error {
	if f.backend == FirewallIptables {
		_ = exec.Command("iptables", "-t", "nat", "-N", "DOKI").Run()
		if exec.Command("iptables", "-t", "nat", "-C", "PREROUTING", "-j", "DOKI").Run() != nil {
			_ = exec.Command("iptables", "-t", "nat", "-A", "PREROUTING", "-j", "DOKI").Run()
		}
		if exec.Command("iptables", "-t", "nat", "-C", "OUTPUT", "-j", "DOKI").Run() != nil {
			_ = exec.Command("iptables", "-t", "nat", "-A", "OUTPUT", "-j", "DOKI").Run()
		}
		return nil
	}
	_ = exec.Command("nft", "add", "table", "ip", "nat").Run()
	_ = exec.Command("nft", "add", "chain", "ip", "nat", "DOKI",
		"{", "type", "nat", "hook", "prerouting", "priority", "0", ";", "}").Run()
	_ = exec.Command("nft", "add", "chain", "ip", "nat", "DOKI_OUTPUT",
		"{", "type", "nat", "hook", "output", "priority", "0", ";", "}").Run()
	return nil
}

func (f *FirewallManager) addNftablesPortMapping(containerIP string, hostPort, containerPort int, proto string) error {
	args := []string{
		"add", "rule", "ip", "nat", "DOKI",
		proto, "dport", fmt.Sprintf("%d", hostPort),
		"dnat", "to", fmt.Sprintf("%s:%d", containerIP, containerPort),
	}
	if err := exec.Command("nft", args...).Run(); err != nil {
		return err
	}
	outArgs := []string{
		"add", "rule", "ip", "nat", "DOKI_OUTPUT",
		proto, "dport", fmt.Sprintf("%d", hostPort),
		"dnat", "to", fmt.Sprintf("%s:%d", containerIP, containerPort),
	}
	return exec.Command("nft", outArgs...).Run()
}

func (f *FirewallManager) addIptablesPortMapping(containerIP string, hostPort, containerPort int, proto string) error {
	cmd := exec.Command("iptables",
		"-t", "nat", "-A", "DOKI",
		"-p", proto, "--dport", fmt.Sprintf("%d", hostPort),
		"-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", containerIP, containerPort),
	)
	return cmd.Run()
}

// RemovePortMapping removes a port mapping rule.
func (f *FirewallManager) RemovePortMapping(containerIP string, hostPort, containerPort int, proto string) error {
	if f.backend == FirewallNftables {
		return f.removeNftablesPortMapping(containerIP, hostPort, containerPort, proto)
	}
	return f.removeIptablesPortMapping(containerIP, hostPort, containerPort, proto)
}

func (f *FirewallManager) removeNftablesPortMapping(containerIP string, hostPort, _ int, proto string) error {
	// Find the rule handle by listing rules and matching
	listCmd := exec.Command("nft", "-a", "list", "chain", "ip", "nat", "DOKI")
	output, err := listCmd.Output()
	if err != nil {
		// Try DOKI_OUTPUT chain as well
		listCmd = exec.Command("nft", "-a", "list", "chain", "ip", "nat", "DOKI_OUTPUT")
		output, err = listCmd.Output()
		if err != nil {
			return err
		}
	}
	// Parse output to find handle number for matching rule
	matchStr := fmt.Sprintf("%s dport %d", proto, hostPort)
	handle := ""
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, matchStr) && strings.Contains(line, containerIP) {
			// Extract handle number (last number after # handle)
			if idx := strings.Index(line, "# handle"); idx >= 0 {
				handle = strings.TrimSpace(line[idx+8:])
			}
			break
		}
	}
	if handle == "" {
		return nil // Rule not found, nothing to remove
	}
	// Delete by handle
	cmd := exec.Command("nft", "delete", "rule", "ip", "nat", "DOKI", "handle", handle)
	if err := cmd.Run(); err != nil {
		// Try DOKI_OUTPUT chain
		cmd = exec.Command("nft", "delete", "rule", "ip", "nat", "DOKI_OUTPUT", "handle", handle)
		return cmd.Run()
	}
	return nil
}

func (f *FirewallManager) removeIptablesPortMapping(containerIP string, hostPort, containerPort int, proto string) error {
	cmd := exec.Command("iptables",
		"-t", "nat", "-D", "DOKI",
		"-p", proto, "--dport", fmt.Sprintf("%d", hostPort),
		"-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", containerIP, containerPort),
	)
	return cmd.Run()
}

// ─── Helpers ───────────────────────────────────────────────────────

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
