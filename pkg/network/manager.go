// Package network provides container networking.
package network

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/netlink"
)

// PortForwarder is anything that can be cleanly stopped: a socat process
// (wrapped in execCmdForwarder) or a DokiLink-Lite proxy (TCPProxy/UDPProxy).
// The interface lets us mix implementations in the same map and reaps
// zombie socat children consistently (BUG-11 fix).
type PortForwarder interface {
	Close() error
}

// execCmdForwarder wraps a socat (or any other external) *exec.Cmd to
// satisfy PortForwarder. Kill+Wait ensures the child is reaped.
type execCmdForwarder struct {
	cmd *exec.Cmd
}

func (e *execCmdForwarder) Close() error {
	if e == nil || e.cmd == nil || e.cmd.Process == nil {
		return nil
	}
	err := e.cmd.Process.Kill()
	_ = e.cmd.Wait()
	return err
}

// tcpProxyForwarder wraps a netlink.TCPProxy to satisfy PortForwarder.
type tcpProxyForwarder struct {
	p *netlink.TCPProxy
}

func (t *tcpProxyForwarder) Close() error {
	if t == nil || t.p == nil {
		return nil
	}
	return t.p.Close()
}

// udpProxyForwarder wraps a netlink.UDPProxy to satisfy PortForwarder.
type udpProxyForwarder struct {
	p *netlink.UDPProxy
}

func (u *udpProxyForwarder) Close() error {
	if u == nil || u.p == nil {
		return nil
	}
	return u.p.Close()
}

// Manager manages container networks.
type Manager struct {
	mu               sync.RWMutex
	root             string
	networks         map[string]*Network
	firewall         *FirewallManager
	dns              *DNSServer
	portForwardProcs map[string][]PortForwarder
	pfMu             sync.Mutex
}

// Network represents a container network.
type Network struct {
	ID         string               `json:"Id"`
	Name       string               `json:"Name"`
	Driver     string               `json:"Driver"`
	Subnet     string               `json:"Subnet"`
	Gateway    string               `json:"Gateway"`
	EnableIPv6 bool                 `json:"EnableIPv6"`
	Internal   bool                 `json:"Internal"`
	Options    map[string]string    `json:"Options"`
	Labels     map[string]string    `json:"Labels"`
	Containers map[string]*Endpoint `json:"Containers"`
	Created    time.Time            `json:"Created"`
}

// Endpoint represents a container endpoint in a network.
type Endpoint struct {
	EndpointID  string    `json:"EndpointID"`
	MacAddress  string    `json:"MacAddress"`
	IPv4Address string    `json:"IPv4Address"`
	IPv6Address string    `json:"IPv6Address"`
	VethHost    string    `json:"VethHost,omitempty"`
	VethPeer    string    `json:"VethPeer,omitempty"`
	Aliases     []string  `json:"Aliases,omitempty"`
	PortMapping []PortMap `json:"PortMapping,omitempty"`
}

// PortMap represents a port mapping for an endpoint.
type PortMap struct {
	HostPort      uint16 `json:"HostPort"`
	HostIP        string `json:"HostIP"`
	Proto         string `json:"Proto"`
	ContainerPort uint16 `json:"ContainerPort"`
}

// NetworkConfig holds configuration for creating a network.
type NetworkConfig struct {
	Name       string
	Driver     string
	Subnet     string
	Gateway    string
	EnableIPv6 bool
	Internal   bool
	Options    map[string]string
	Labels     map[string]string
}

// NewManager creates a new network manager.
func NewManager(root string, firewall *FirewallManager, dns *DNSServer) (*Manager, error) {
	_ = common.EnsureDir(root)

	m := &Manager{
		root:             root,
		networks:         make(map[string]*Network),
		firewall:         firewall,
		dns:              dns,
		portForwardProcs: make(map[string][]PortForwarder),
	}

	// Create default networks.
	m.createDefaultNetworks()

	return m, nil
}

func (m *Manager) createDefaultNetworks() {
	// Default bridge network.
	_, _ = m.CreateNetwork(&NetworkConfig{
		Name:   "bridge",
		Driver: "bridge",
		Subnet: "172.17.0.0/16",
	})

	// Host network.
	_, _ = m.CreateNetwork(&NetworkConfig{
		Name:   "host",
		Driver: "host",
	})

	// None network.
	_, _ = m.CreateNetwork(&NetworkConfig{
		Name:   "none",
		Driver: "null",
	})
}

// CreateNetwork creates a new network.
func (m *Manager) CreateNetwork(cfg *NetworkConfig) (*Network, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if exists.
	for _, nw := range m.networks {
		if nw.Name == cfg.Name {
			return nw, common.NewErrConflict("network", cfg.Name)
		}
	}

	network := &Network{
		ID:         common.GenerateID(64),
		Name:       cfg.Name,
		Driver:     cfg.Driver,
		Subnet:     cfg.Subnet,
		Gateway:    cfg.Gateway,
		EnableIPv6: cfg.EnableIPv6,
		Internal:   cfg.Internal,
		Options:    cfg.Options,
		Labels:     cfg.Labels,
		Containers: make(map[string]*Endpoint),
		Created:    time.Now(),
	}

	if cfg.Driver == "bridge" && cfg.Subnet != "" {
		network.Gateway = calculateGateway(cfg.Subnet)
	}

	if err := m.saveNetwork(network); err != nil {
		return nil, err
	}

	m.networks[network.ID] = network
	return network, nil
}

func calculateGateway(subnet string) string {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return ""
	}
	// BUG-06 fix: the previous code hardcoded byte[last]=1 which produces
	// a gateway outside the subnet for non-/24 subnets (e.g. 172.17.0.128/25
	// yields 172.17.0.1 which is outside the subnet). Use incrementIP on the
	// network address instead.
	gateway := make(net.IP, len(ipNet.IP))
	copy(gateway, ipNet.IP)
	incrementIP(gateway, ipNet.Mask)
	return gateway.String()
}

// GetNetwork returns a network by ID or name.
func (m *Manager) GetNetwork(idOrName string) (*Network, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nw, err := m.loadNetwork(idOrName)
	if err != nil {
		return nil, common.NewErrNotFound("network", idOrName)
	}
	return nw, nil
}

// ContainerIP returns the IPv4 address allocated to containerID on the named
// network, or "" if it has no endpoint there. Used by the CRI to report a pod's
// IP in PodSandboxStatus.
func (m *Manager) ContainerIP(networkName, containerID string) string {
	nw, err := m.GetNetwork(networkName)
	if err != nil {
		return ""
	}
	if ep, ok := nw.Containers[containerID]; ok {
		return ep.IPv4Address
	}
	return ""
}

// ListNetworks returns all networks.
func (m *Manager) ListNetworks() ([]common.NetworkInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var networks []common.NetworkInfo
	for _, nw := range m.networks {
		networks = append(networks, m.toNetworkInfo(nw))
	}
	return networks, nil
}

func (m *Manager) toNetworkInfo(nw *Network) common.NetworkInfo {
	containers := make(map[string]common.EndpointResource)
	for id, ep := range nw.Containers {
		containers[id] = common.EndpointResource{
			EndpointID:  ep.EndpointID,
			MacAddress:  ep.MacAddress,
			IPv4Address: ep.IPv4Address,
			IPv6Address: ep.IPv6Address,
		}
	}

	ipamConfigs := []common.IPAMConfig{}
	if nw.Subnet != "" {
		ipamConfigs = append(ipamConfigs, common.IPAMConfig{
			Subnet:  nw.Subnet,
			Gateway: nw.Gateway,
		})
	}

	return common.NetworkInfo{
		ID:         nw.ID,
		Name:       nw.Name,
		Driver:     nw.Driver,
		Scope:      "local",
		Internal:   nw.Internal,
		EnableIPv6: nw.EnableIPv6,
		IPAM: &common.IPAM{
			Driver: "default",
			Config: ipamConfigs,
		},
		Options:    nw.Options,
		Labels:     nw.Labels,
		Containers: containers,
		Created:    nw.Created,
	}
}

// ReRegisterDNS re-registers DNS entries for a container across all networks.
// Used after daemon restart to restore container name resolution.
func (m *Manager) ReRegisterDNS(containerID string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.dns == nil {
		return
	}

	for _, nw := range m.networks {
		if ep, ok := nw.Containers[containerID]; ok && ep.IPv4Address != "" {
			m.dns.AddEntry(containerID, ep.IPv4Address)
			for _, alias := range ep.Aliases {
				m.dns.AddEntry(alias, ep.IPv4Address)
			}
		}
	}
}

// Connect connects a container to a network and configures networking.
func (m *Manager) Connect(networkID, containerID string, ipAddr string, aliases []string, ports []PortMap, containerPid int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nw, err := m.loadNetwork(networkID)
	if err != nil {
		return err
	}

	existing, exists := nw.Containers[containerID]

	ep := &Endpoint{
		EndpointID:  common.GenerateID(64),
		MacAddress:  generateMacAddr(),
		IPv4Address: ipAddr,
		Aliases:     aliases,
		PortMapping: ports,
	}

	if ipAddr == "" && nw.Driver == "bridge" {
		ep.IPv4Address = m.allocateIP(nw)
	}

	if exists && existing != nil {
		ep.EndpointID = existing.EndpointID
		ep.MacAddress = existing.MacAddress
		if ep.IPv4Address == "" {
			ep.IPv4Address = existing.IPv4Address
		}
	}

	nw.Containers[containerID] = ep
	if err := m.saveNetwork(nw); err != nil {
		return err
	}

	// Register in DNS.
	if m.dns != nil && ep.IPv4Address != "" {
		m.dns.AddEntry(containerID, ep.IPv4Address)
		for _, alias := range aliases {
			m.dns.AddEntry(alias, ep.IPv4Address)
		}
	}

	// Configure actual networking if container is running.
	if containerPid > 0 && nw.Driver == "bridge" {
		if os.Geteuid() == 0 {
			_ = setupBridgeNetwork(containerPid, nw, ep, m.firewall)
		} else {
			_ = setupRootlessNetworking(containerPid)
		}
	}

	return nil
}

// Disconnect disconnects a container from a network and removes configuration.
func (m *Manager) Disconnect(networkID, containerID string, containerPid int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nw, err := m.loadNetwork(networkID)
	if err != nil {
		return err
	}

	ep := nw.Containers[containerID]

	// Remove from DNS.
	if m.dns != nil && ep != nil && ep.IPv4Address != "" {
		m.dns.RemoveEntry(containerID)
		for _, alias := range ep.Aliases {
			m.dns.RemoveEntry(alias)
		}
	}

	// Teardown networking if container is running.
	if containerPid > 0 && nw.Driver == "bridge" {
		_ = teardownBridgeNetwork(nw, ep, m.firewall)
	}

	delete(nw.Containers, containerID)
	return m.saveNetwork(nw)
}

// RemoveNetwork removes a network.
func (m *Manager) RemoveNetwork(idOrName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nw, err := m.loadNetwork(idOrName)
	if err != nil {
		return err
	}

	if len(nw.Containers) > 0 {
		return fmt.Errorf("network %s has active endpoints", nw.Name)
	}

	// Don't allow removing built-in networks.
	if nw.Name == "bridge" || nw.Name == "host" || nw.Name == "none" {
		return fmt.Errorf("cannot remove built-in network %s", nw.Name)
	}

	delete(m.networks, nw.ID)
	return os.Remove(m.networkPath(nw.ID))

}

// Prune removes all unused networks.
func (m *Manager) Prune() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var pruned []string
	for id, nw := range m.networks {
		if len(nw.Containers) == 0 &&
			nw.Name != "bridge" &&
			nw.Name != "host" &&
			nw.Name != "none" {
			pruned = append(pruned, nw.Name)
			delete(m.networks, id)
			_ = os.Remove(m.networkPath(id))
		}
	}

	return pruned, nil
}

func (m *Manager) allocateIP(nw *Network) string {
	if nw.Subnet == "" {
		return ""
	}

	_, ipNet, err := net.ParseCIDR(nw.Subnet)
	if err != nil {
		return ""
	}

	usedIPs := make(map[string]bool)
	for _, ep := range nw.Containers {
		usedIPs[ep.IPv4Address] = true
	}

	// Iterate all valid host addresses in subnet (skip broadcast).
	ip := ipNet.IP.To4()
	if ip == nil {
		return ""
	}
	mask := ipNet.Mask
	// Compute broadcast address (all ones in host portion)
	broadcast := make(net.IP, len(ip))
	copy(broadcast, ip)
	for j := range broadcast {
		broadcast[j] |= ^mask[j]
	}
	// Skip network address (all 0)
	incrementIP(ip, mask)
	// Skip gateway (.1)
	incrementIP(ip, mask)

	for ipNet.Contains(ip) && !ip.Equal(broadcast) {
		ipStr := ip.String()
		if !usedIPs[ipStr] && ipStr != nw.Gateway {
			return ipStr
		}
		incrementIP(ip, mask)
	}

	return ""
}

func incrementIP(ip net.IP, _ net.IPMask) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func generateMacAddr() string {
	mac := make([]byte, 6)
	mac[0] = 0x02 // Locally administered.
	mac[1] = 0x42
	// Read random bytes for remaining 4 bytes.
	randBytes := make([]byte, 4)
	if f, err := os.Open("/dev/urandom"); err == nil {
		if _, err := io.ReadFull(f, randBytes); err != nil {
			// BUG-18 fix: if /dev/urandom fails, fall back to crypto/rand
			// so MAC addresses are not all zeros (which causes collisions).
			_, _ = crypto_rand.Read(randBytes)
		}
		_ = f.Close()
	} else {
		_, _ = crypto_rand.Read(randBytes)
	}
	for i := 2; i < 6; i++ {
		mac[i] = randBytes[i-2]
	}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

func (m *Manager) networkPath(id string) string {
	return filepath.Join(m.root, id+".json")
}

func (m *Manager) saveNetwork(nw *Network) error {
	data, err := json.MarshalIndent(nw, "", "  ")
	if err != nil {
		return err
	}

	path := m.networkPath(nw.ID)
	m.networks[nw.ID] = nw

	return os.WriteFile(path, data, 0644)
}

func (m *Manager) loadNetwork(idOrName string) (*Network, error) {
	// Check by ID first.
	if nw, ok := m.networks[idOrName]; ok {
		return nw, nil
	}

	// Check by name.
	for _, nw := range m.networks {
		if nw.Name == idOrName {
			return nw, nil
		}
	}

	// Load from disk.
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(m.root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var nw Network
		if err := json.Unmarshal(data, &nw); err != nil {
			continue
		}

		if nw.ID == idOrName || nw.Name == idOrName {
			m.networks[nw.ID] = &nw
			return &nw, nil
		}
	}

	return nil, common.NewErrNotFound("network", idOrName)
}

// SetupNetwork configures networking for a container.
// If no endpoint exists yet (Connect was not called), creates one
// with an IP allocation and registers DNS entries.
func (m *Manager) SetupNetwork(containerID string, containerPid int, networkName string) error {
	if networkName == "none" {
		return nil
	}

	if networkName == "host" {
		return setupHostNetwork()
	}

	nw, err := m.GetNetwork(networkName)
	if err != nil {
		return err
	}

	if nw.Driver == "bridge" {
		// Lock to prevent race conditions when accessing/modifying nw.Containers
		m.mu.Lock()
		ep := nw.Containers[containerID]
		if ep == nil {
			// No endpoint yet — create one with IP allocation.
			ep = &Endpoint{
				EndpointID:  common.GenerateID(64),
				MacAddress:  generateMacAddr(),
				IPv4Address: m.allocateIP(nw),
			}
			nw.Containers[containerID] = ep
			if err := m.saveNetwork(nw); err != nil {
				m.mu.Unlock()
				return err
			}
		}
		m.mu.Unlock()

		// Register DNS entries (idempotent — AddEntry overwrites).
		if m.dns != nil && ep.IPv4Address != "" {
			m.dns.AddEntry(containerID, ep.IPv4Address)
			for _, alias := range ep.Aliases {
				m.dns.AddEntry(alias, ep.IPv4Address)
			}
		}

		return setupBridgeNetwork(containerPid, nw, ep, m.firewall)
	}

	return nil
}

// AE10: ValidatePortBinding checks if a port binding is allowed.
func ValidatePortBinding(hostPort uint16, privileged bool) error {
	if privileged {
		return nil
	}
	if hostPort > 0 && hostPort < 1024 {
		return fmt.Errorf("cannot bind to privileged port %d without root or Privileged flag", hostPort)
	}
	return nil
}

// TeardownNetwork removes networking for a container.
//
// BUG-05 fix: the previous implementation called GetNetwork (which releases
// m.mu.RLock() on return) and then accessed nw.Containers without holding
// any lock. Concurrent Connect/Disconnect calls (which take m.mu.Lock())
// could mutate nw.Containers during the read.
func (m *Manager) TeardownNetwork(containerID string, networkName string) error {
	if networkName == "none" || networkName == "host" {
		return nil
	}

	m.mu.RLock()
	nw, err := m.loadNetwork(networkName)
	if err != nil {
		m.mu.RUnlock()
		return err
	}
	ep := nw.Containers[containerID]
	driver := nw.Driver
	m.mu.RUnlock()

	if driver == "bridge" {
		return teardownBridgeNetwork(nw, ep, m.firewall)
	}

	return nil
}

func setupHostNetwork() error {
	// The container already shares the host netns via clone flags (no CLONE_NEWNET).
	// No additional configuration needed for host networking.
	return nil
}

func setupBridgeNetwork(pid int, nw *Network, ep *Endpoint, firewall *FirewallManager) error {
	if os.Geteuid() != 0 {
		return setupRootlessNetworking(pid)
	}

	bridgeName := "doki0"
	vethHost := fmt.Sprintf("doki%d", pid)
	vethContainer := "eth0" + fmt.Sprintf("p%d", pid)

	// 1. Ensure bridge exists and is up.
	if !bridgeExists(bridgeName) {
		if out, err := exec.Command("ip", "link", "add", bridgeName, "type", "bridge").CombinedOutput(); err != nil {
			return fmt.Errorf("create bridge %s: %s %w", bridgeName, string(out), err)
		}
		if out, err := exec.Command("ip", "link", "set", bridgeName, "up").CombinedOutput(); err != nil {
			return fmt.Errorf("set bridge %s up: %s %w", bridgeName, string(out), err)
		}
		// Assign gateway IP to the bridge.
		if nw.Gateway != "" && nw.Subnet != "" {
			_, ipNet, _ := net.ParseCIDR(nw.Subnet)
			if ipNet != nil {
				ones, _ := ipNet.Mask.Size()
				gwAddr := fmt.Sprintf("%s/%d", nw.Gateway, ones)
				_ = exec.Command("ip", "addr", "add", gwAddr, "dev", bridgeName).Run()
			}
		}
	}

	// 2. Create veth pair.
	if out, err := exec.Command("ip", "link", "add", vethHost, "type", "veth",
		"peer", "name", vethContainer).CombinedOutput(); err != nil {
		return fmt.Errorf("create veth: %s %w", string(out), err)
	}

	// 3. Move container-side into the container's netns.
	if out, err := exec.Command("ip", "link", "set", vethContainer, "netns",
		fmt.Sprintf("%d", pid)).CombinedOutput(); err != nil {
		return fmt.Errorf("move veth to netns %d: %s %w", pid, string(out), err)
	}

	// 4. Attach host-side to bridge and bring it up.
	if out, err := exec.Command("ip", "link", "set", vethHost, "master", bridgeName).CombinedOutput(); err != nil {
		return fmt.Errorf("attach veth to bridge: %s %w", string(out), err)
	}
	if out, err := exec.Command("ip", "link", "set", vethHost, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("set veth up: %s %w", string(out), err)
	}
	// Record the host-side veth name so teardown can clean it up.
	if ep != nil {
		ep.VethHost = vethHost
		ep.VethPeer = vethContainer
	}

	// 5. Inside container netns: rename to eth0, assign IP, bring up, add route.
	netnsFlag := fmt.Sprintf("%d", pid)
	containerIP := ""
	gateway := nw.Gateway
	prefixLen := 16
	if _, ipNet, err := net.ParseCIDR(nw.Subnet); err == nil && ipNet != nil {
		ones, _ := ipNet.Mask.Size()
		prefixLen = ones
	}
	if ep != nil && ep.IPv4Address != "" {
		containerIP = ep.IPv4Address
		ipAddr := fmt.Sprintf("%s/%d", containerIP, prefixLen)

		// Rename interface inside netns.
		_ = exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
			"ip", "link", "set", vethContainer, "name", "eth0").Run()

		// Assign IP.
		if out, err := exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
			"ip", "addr", "add", ipAddr, "dev", "eth0").CombinedOutput(); err != nil {
			return fmt.Errorf("assign ip %s: %s %w", ipAddr, string(out), err)
		}

		// Bring up eth0 and lo.
		_ = exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
			"ip", "link", "set", "eth0", "up").Run()
		_ = exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
			"ip", "link", "set", "lo", "up").Run()

		// Add default route.
		if gateway != "" {
			_ = exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
				"ip", "route", "add", "default", "via", gateway).Run()
		}
	}

	// 6. Set up port mappings.
	if firewall != nil && containerIP != "" && ep != nil {
		for _, pm := range ep.PortMapping {
			_ = firewall.AddPortMapping(containerIP, int(pm.HostPort), int(pm.ContainerPort), pm.Proto)
		}
	}

	// 7. DNAT DNS traffic from containers to the internal DNS server.
	// Redirects port 53 traffic to the configured DNS listen port.
	// Only runs as root (non-root Android cannot use iptables).
	dnsListen := os.Getenv("DOKI_DNS_LISTEN")
	if dnsListen == "" {
		dnsListen = "127.0.0.11:53"
	}
	_, dnsPort, _ := net.SplitHostPort(dnsListen)
	if dnsPort != "" && dnsPort != "53" {
		dnatArgs := []string{"-t", "nat", "-C", "OUTPUT", "-p", "udp", "--dport", "53",
			"-d", "127.0.0.11", "-j", "DNAT", "--to-destination", "127.0.0.11:" + dnsPort}
		if err := exec.Command("iptables", dnatArgs...).Run(); err != nil {
			addArgs := []string{"-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53",
				"-d", "127.0.0.11", "-j", "DNAT", "--to-destination", "127.0.0.11:" + dnsPort}
			_ = exec.Command("iptables", addArgs...).Run()
		}
		dnatArgsTCP := []string{"-t", "nat", "-C", "OUTPUT", "-p", "tcp", "--dport", "53",
			"-d", "127.0.0.11", "-j", "DNAT", "--to-destination", "127.0.0.11:" + dnsPort}
		if err := exec.Command("iptables", dnatArgsTCP...).Run(); err != nil {
			addArgsTCP := []string{"-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53",
				"-d", "127.0.0.11", "-j", "DNAT", "--to-destination", "127.0.0.11:" + dnsPort}
			_ = exec.Command("iptables", addArgsTCP...).Run()
		}
	}

	return nil
}

func teardownBridgeNetwork(_ *Network, ep *Endpoint, firewall *FirewallManager) error {
	if firewall != nil && ep != nil {
		for _, pm := range ep.PortMapping {
			_ = firewall.RemovePortMapping(ep.IPv4Address, int(pm.HostPort), int(pm.ContainerPort), pm.Proto)
		}
	}
	// Clean up the container-side veth interface so it does not leak across container restarts.
	if ep != nil && ep.VethHost != "" {
		_ = exec.Command("ip", "link", "del", ep.VethHost).Run()
	}
	return nil
}

func bridgeExists(name string) bool {
	_, err := os.Stat("/sys/class/net/" + name)
	return err == nil
}

// setupRootlessNetworking sets up networking for a rootless container.
// Priority: pasta (preferred on standard Linux) -> slirp4netns (legacy)
// -> shared host netns (fallback on all platforms).
//
// On standard Linux, installing either pasta or slirp4netns gives the
// container isolated networking with proper NAT. On Termux neither
// tool is functional: /dev/net/tun is unavailable without root and the
// kernel does not support unprivileged TUN/TAP creation. The expected
// behavior in Termux is the third branch: the container shares the host
// network namespace via proot (proot does not unshare netns in user-mode
// emulation on Android), so network setup is a no-op and the guest
// inherits the host's network.
func setupRootlessNetworking(pid int) error {
	// 1. Try pasta (preferred rootless networking).
	if path, err := exec.LookPath("pasta"); err == nil {
		cmd := exec.Command(path, "--pid", fmt.Sprintf("%d", pid))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
		slog.Debug("network setup: pasta failed, trying slirp4netns", "pid", pid, "err", err)
	}

	// 2. Try slirp4netns (legacy rootless networking).
	if path, err := exec.LookPath("slirp4netns"); err == nil {
		cmd := exec.Command(path, "--configure", "--mtu=65520",
			fmt.Sprintf("--netns-type=path=/proc/%d/ns/net", pid),
			"tap0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
		slog.Debug("network setup: slirp4netns failed, falling back to host netns", "pid", pid, "err", err)
	}

	// 3. Fallback: no isolated networking. The container shares the
	// host network namespace via proot (the common and expected
	// Termux case). On standard Linux, installing either tool gives
	// proper isolation; on Termux neither works and host-ns sharing
	// is the intended behavior.
	if common.IsTermux() {
		slog.Info("network setup: rootless networking tools unavailable on Termux; "+
			"passt and slirp4netns are not installable here and would not work if installed "+
			"(require root + /dev/net/tun which Termux does not provide); "+
			"the container shares the host network namespace via proot, which is functional but not isolated",
			"pid", pid)
		return nil
	}
	slog.Info("network setup: no rootless networking tool available (pasta/slirp4netns not found); "+
		"container will share host network namespace. "+
		"Install 'passt' (pasta) or 'slirp4netns' for proper network isolation",
		"pid", pid)
	return nil
}

// IsPastaAvailable checks if pasta is available for rootless networking.
func IsPastaAvailable() bool {
	_, err := exec.LookPath("pasta")
	return err == nil
}

// IsSlirp4netnsAvailable checks if slirp4netns is available.
func IsSlirp4netnsAvailable() bool {
	_, err := exec.LookPath("slirp4netns")
	return err == nil
}

// StartPortForwarding starts a port forward from hostPort to the container's
// (IP, containerPort) using DokiLink-Lite (pure-Go TCP/UDP proxy). If
// DokiLink is explicitly disabled via DOKI_USE_SOCAT=1, it falls back to
// the legacy socat process. The returned value implements PortForwarder
// (closeable, reaps child processes).
//
// DokiLink-Lite is preferred because:
//   - It does not require socat to be installed (Termux does not ship it
//     by default; users must `apt install socat`).
//   - It works on platforms where the kernel blocks fork+exec for
//     network-related syscalls (Android seccomp).
//   - It can be wrapped in TLS/secretbox in v0.9.3+ (see netlink/crypto.go).
func StartPortForwarding(hostPort int, containerIP string, containerPort int, proto string) (PortForwarder, error) {
	useSocat := strings.EqualFold(os.Getenv("DOKI_USE_SOCAT"), "1") ||
		strings.EqualFold(os.Getenv("DOKI_USE_SOCAT"), "true")

	if useSocat {
		return startPortForwardingSocat(hostPort, containerIP, containerPort, proto)
	}

	listenHost := "0.0.0.0"
	if common.UseProotHostNetworking() {
		// In proot+Termux the container shares the host netns and is
		// reachable on loopback. We still advertise 0.0.0.0 in Docker
		// API output for compatibility, but binding to 127.0.0.1 is
		// safer and faster (no surprise exposure to the LAN).
		listenHost = "127.0.0.1"
	}
	listenAddr := net.JoinHostPort(listenHost, fmt.Sprintf("%d", hostPort))
	upstreamAddr := net.JoinHostPort(common.ProotContainerIP(containerIP), fmt.Sprintf("%d", containerPort))

	if proto == "udp" {
		p := netlink.NewUDPProxy(netlink.UDPProxyConfig{
			ListenAddr: listenAddr,
			Upstream:   upstreamAddr,
		})
		if err := p.Start(context.Background()); err != nil {
			slog.Warn("doki-link: udp proxy failed, falling back to socat",
				"host", hostPort, "err", err)
			return startPortForwardingSocat(hostPort, containerIP, containerPort, proto)
		}
		return &udpProxyForwarder{p: p}, nil
	}
	p := netlink.NewTCPProxy(netlink.TCPProxyConfig{
		ListenAddr: listenAddr,
		Upstream:   upstreamAddr,
	})
	if err := p.Start(context.Background()); err != nil {
		slog.Warn("doki-link: tcp proxy failed, falling back to socat",
			"host", hostPort, "err", err)
		return startPortForwardingSocat(hostPort, containerIP, containerPort, proto)
	}
	return &tcpProxyForwarder{p: p}, nil
}

// startPortForwardingSocat is the legacy implementation. It is used as
// a fallback when DokiLink-Lite cannot bind (port already in use,
// permission denied, etc.) or when DOKI_USE_SOCAT=1 is set.
func startPortForwardingSocat(hostPort int, containerIP string, containerPort int, proto string) (PortForwarder, error) {
	if proto == "udp" {
		cmd := exec.Command("socat", fmt.Sprintf("UDP-LISTEN:%d,fork,reuseaddr", hostPort),
			fmt.Sprintf("UDP:%s:%d", common.ProotContainerIP(containerIP), containerPort))
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("udp port forward %d->%s:%d: %w", hostPort, containerIP, containerPort, err)
		}
		return &execCmdForwarder{cmd: cmd}, nil
	}
	cmd := exec.Command("socat", fmt.Sprintf("TCP-LISTEN:%d,fork,reuseaddr", hostPort),
		fmt.Sprintf("TCP:%s:%d", common.ProotContainerIP(containerIP), containerPort))
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("tcp port forward %d->%s:%d: %w", hostPort, containerIP, containerPort, err)
	}
	return &execCmdForwarder{cmd: cmd}, nil
}

// ApplyPortForwardings creates TCP/UDP proxies from host ports to container ports.
func (m *Manager) ApplyPortForwardings(containerID string, ports []common.Port, _ int) error {
	// BUG-01 fix: IPv4Address is stored as a bare IP (e.g. "172.17.0.2"),
	// never in CIDR notation. net.ParseCIDR requires "/mask" and returns
	// an error on bare IPs. Use net.ParseIP instead.
	//
	// BUG-02 fix: the previous code iterated ALL networks and ALL
	// endpoints, picking the first non-empty IP regardless of which
	// container it belongs to. Now we look up the specific containerID.
	//
	// Concurrency fix: m.networks is protected by m.mu, not m.pfMu.
	// Acquire m.mu.RLock() for the lookup, then release before starting
	// port forwards (which need m.pfMu).
	m.mu.RLock()
	containerIP := ""
	for _, nw := range m.networks {
		if ep, ok := nw.Containers[containerID]; ok && ep != nil && ep.IPv4Address != "" {
			if ip := net.ParseIP(ep.IPv4Address); ip != nil {
				containerIP = ip.String()
			}
			break
		}
	}
	m.mu.RUnlock()

	if containerIP == "" {
		return fmt.Errorf("no bridge endpoint for container %s; cannot forward ports", containerID)
	}

	m.pfMu.Lock()
	defer m.pfMu.Unlock()

	for _, port := range ports {
		if port.PublicPort <= 0 {
			continue
		}
		proto := string(port.Type)
		if proto == "" {
			proto = "tcp"
		}
		slog.Debug("doki-link: starting forwarder",
			"container", containerID,
			"containerIP", containerIP,
			"host", port.PublicPort,
			"containerPort", port.PrivatePort,
			"proto", proto,
			"useProotHost", common.UseProotHostNetworking())
		fw, err := StartPortForwarding(int(port.PublicPort), containerIP, int(port.PrivatePort), proto)
		if err != nil {
			slog.Warn("port forward failed", "host", port.PublicPort, "container", port.PrivatePort, "err", err)
			continue
		}
		m.portForwardProcs[containerID] = append(m.portForwardProcs[containerID], fw)
	}
	return nil
}

// RemovePortForwardings stops all port forwards for a container. Works
// for both DokiLink-Lite proxies (Close on TCPProxy/UDPProxy) and the
// legacy socat fallback (execCmdForwarder.Close kills and reaps).
func (m *Manager) RemovePortForwardings(containerID string) {
	m.pfMu.Lock()
	defer m.pfMu.Unlock()

	fws := m.portForwardProcs[containerID]
	for _, fw := range fws {
		_ = fw.Close()
	}
	delete(m.portForwardProcs, containerID)
}

// Inspect returns network details.
func (m *Manager) Inspect(idOrName string) (*common.NetworkInfo, error) {
	nw, err := m.GetNetwork(idOrName)
	if err != nil {
		return nil, err
	}
	info := m.toNetworkInfo(nw)
	return &info, nil
}
