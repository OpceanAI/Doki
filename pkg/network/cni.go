package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	if f.backend == FirewallNftables {
		return f.addNftablesPortMapping(containerIP, hostPort, containerPort, proto)
	}
	return f.addIptablesPortMapping(containerIP, hostPort, containerPort, proto)
}

func (f *FirewallManager) addNftablesPortMapping(containerIP string, hostPort, containerPort int, proto string) error {
	cmd := exec.Command("nft",
		"add", "rule", "ip", "nat", "DOKI",
		fmt.Sprintf("%s", proto), "dport", fmt.Sprintf("%d", hostPort),
		"dnat", "to", fmt.Sprintf("%s:%d", containerIP, containerPort),
	)
	return cmd.Run()
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

func (f *FirewallManager) removeNftablesPortMapping(containerIP string, hostPort, containerPort int, proto string) error {
	// nft delete requires a numeric handle - use iptables as reliable fallback
	cmd := exec.Command("iptables", "-t", "nat", "-D", "DOKI",
		"-p", proto, "--dport", fmt.Sprintf("%d", hostPort),
		"-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", containerIP, containerPort))
	return cmd.Run()
}

func (f *FirewallManager) removeIptablesPortMapping(containerIP string, hostPort, containerPort int, proto string) error {
	cmd := exec.Command("iptables",
		"-t", "nat", "-D", "DOKI",
		"-p", proto, "--dport", fmt.Sprintf("%d", hostPort),
		"-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", containerIP, containerPort),
	)
	_ = cmd.Run()
	return nil
}

// ─── DNS server for container name resolution ──────────────────────

// DNSServer provides internal DNS resolution for containers.
type DNSServer struct {
	entries map[string]string
	mu      sync.RWMutex
	conn    *net.UDPConn
	done    chan struct{}
}

// NewDNSServer creates a DNS server.
func NewDNSServer() *DNSServer {
	return &DNSServer{
		entries: make(map[string]string),
		done:    make(chan struct{}),
	}
}

func (d *DNSServer) Start(addr string) error {
	var err error
	d.conn, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(addr), Port: 53})
	if err != nil {
		return err
	}
	go d.serve()
	return nil
}

func (d *DNSServer) serve() {
	buf := make([]byte, 512)
	for {
		select {
		case <-d.done:
			return
		default:
		}
		d.conn.SetReadDeadline(time.Now().Add(time.Second))
		n, clientAddr, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}
		go d.handleQuery(buf[:n], clientAddr)
	}
}

func (d *DNSServer) handleQuery(query []byte, clientAddr *net.UDPAddr) {
	if len(query) < 13 {
		return
	}
	// Extract domain name from DNS query (simple, non-recursive)
	var domain strings.Builder
	offset := 12
	for offset < len(query) {
		length := int(query[offset])
		if length == 0 {
			break
		}
		offset++
		if domain.Len() > 0 {
			domain.WriteByte('.')
		}
		domain.Write(query[offset : offset+length])
		offset += length
	}
	name := domain.String()

	d.mu.RLock()
	ip, ok := d.entries[name]
	d.mu.RUnlock()

	if !ok {
		return // NXDOMAIN (silent drop)
	}

	// Build response
	response := make([]byte, 512)
	copy(response, query[:2])                          // Transaction ID
	response[2] = 0x81; response[3] = 0x80             // Flags: response, no error
	response[4] = 0x00; response[5] = 0x01             // Questions: 1
	response[6] = 0x00; response[7] = 0x01             // Answers: 1
	copy(response[8:12], query[4:8])                    // Authority/Additional: 0
	copy(response[12:], query[12:offset])               // Original question
	answerStart := offset + 5                          // Skip question + null
	response[answerStart-5+1] = 0x00; response[answerStart-5+3] = 0x01 // Type A
	response[answerStart-5+5] = 0xC0; response[answerStart-5+6] = 0x0C // Pointer to name
	response[answerStart-5+7] = 0x00; response[answerStart-5+8] = 0x01 // Type A
	response[answerStart-5+9] = 0x00; response[answerStart-5+10] = 0x01 // Class IN
	ttl := answerStart
	response[ttl] = 0; response[ttl+1] = 0; response[ttl+2] = 0; response[ttl+3] = 60 // TTL 60s
	response[ttl+4] = 0; response[ttl+5] = 4 // Data length
	parsedIP := net.ParseIP(ip).To4()
	if parsedIP != nil {
		copy(response[ttl+6:], parsedIP)
	}
	d.conn.WriteToUDP(response[:ttl+10], clientAddr)
}

func (d *DNSServer) Stop() {
	close(d.done)
	if d.conn != nil {
		d.conn.Close()
	}
}

// AddEntry adds a DNS entry.
func (d *DNSServer) AddEntry(name, ip string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[name] = ip
}

// RemoveEntry removes a DNS entry.
func (d *DNSServer) RemoveEntry(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, name)
}

// Resolve resolves a container name to IP.
func (d *DNSServer) Resolve(name string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ip, ok := d.entries[name]
	return ip, ok
}

// ─── Helpers ───────────────────────────────────────────────────────

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
