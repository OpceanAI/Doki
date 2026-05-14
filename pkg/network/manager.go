package network

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Manager manages container networks.
type Manager struct {
	mu       sync.RWMutex
	root     string
	networks map[string]*Network
	firewall *FirewallManager
	dns      *DNSServer
}

// Network represents a container network.
type Network struct {
	ID         string              `json:"Id"`
	Name       string              `json:"Name"`
	Driver     string              `json:"Driver"`
	Subnet     string              `json:"Subnet"`
	Gateway    string              `json:"Gateway"`
	EnableIPv6 bool                `json:"EnableIPv6"`
	Internal   bool                `json:"Internal"`
	Options    map[string]string   `json:"Options"`
	Labels     map[string]string   `json:"Labels"`
	Containers map[string]*Endpoint `json:"Containers"`
	Created    time.Time           `json:"Created"`
}

// Endpoint represents a container endpoint in a network.
type Endpoint struct {
	EndpointID  string    `json:"EndpointID"`
	MacAddress  string    `json:"MacAddress"`
	IPv4Address string    `json:"IPv4Address"`
	IPv6Address string    `json:"IPv6Address"`
	Aliases     []string  `json:"Aliases,omitempty"`
	PortMapping []PortMap `json:"PortMapping,omitempty"`
}

// PortMap represents a port mapping for an endpoint.
type PortMap struct {
	HostPort  uint16 `json:"HostPort"`
	HostIP    string `json:"HostIP"`
	Proto     string `json:"Proto"`
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
	common.EnsureDir(root)

	m := &Manager{
		root:     root,
		networks: make(map[string]*Network),
		firewall: firewall,
		dns:      dns,
	}

	// Create default networks.
	m.createDefaultNetworks()

	return m, nil
}

func (m *Manager) createDefaultNetworks() {
	// Default bridge network.
	m.CreateNetwork(&NetworkConfig{
		Name:   "bridge",
		Driver: "bridge",
		Subnet: "172.17.0.0/16",
	})

	// Host network.
	m.CreateNetwork(&NetworkConfig{
		Name:   "host",
		Driver: "host",
	})

	// None network.
	m.CreateNetwork(&NetworkConfig{
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
	gateway := make(net.IP, len(ipNet.IP))
	copy(gateway, ipNet.IP)
	gateway[len(gateway)-1] = 1
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
			setupBridgeNetwork(containerPid, nw, ep, m.firewall)
		} else {
			setupRootlessNetworking(containerPid)
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
		teardownBridgeNetwork(nw, ep, m.firewall)
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
			os.Remove(m.networkPath(id))
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

func incrementIP(ip net.IP, mask net.IPMask) {
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
		io.ReadFull(f, randBytes)
		f.Close()
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
		ep := nw.Containers[containerID]
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
func (m *Manager) TeardownNetwork(containerID string, networkName string) error {
	if networkName == "none" || networkName == "host" {
		return nil
	}

	nw, err := m.GetNetwork(networkName)
	if err != nil {
		return err
	}

	if nw.Driver == "bridge" {
		ep := nw.Containers[containerID]
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
				exec.Command("ip", "addr", "add", gwAddr, "dev", bridgeName).Run()
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
		exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
			"ip", "link", "set", vethContainer, "name", "eth0").Run()

		// Assign IP.
		if out, err := exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
			"ip", "addr", "add", ipAddr, "dev", "eth0").CombinedOutput(); err != nil {
			return fmt.Errorf("assign ip %s: %s %w", ipAddr, string(out), err)
		}

		// Bring up eth0 and lo.
		exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
			"ip", "link", "set", "eth0", "up").Run()
		exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
			"ip", "link", "set", "lo", "up").Run()

		// Add default route.
		if gateway != "" {
			exec.Command("nsenter", "--net=/proc/"+netnsFlag+"/ns/net", "--",
				"ip", "route", "add", "default", "via", gateway).Run()
		}
	}

	// 6. Set up port mappings.
	if firewall != nil && containerIP != "" && ep != nil {
		for _, pm := range ep.PortMapping {
			firewall.AddPortMapping(containerIP, int(pm.HostPort), int(pm.ContainerPort), pm.Proto)
		}
	}

	return nil
}

func teardownBridgeNetwork(nw *Network, ep *Endpoint, firewall *FirewallManager) error {
	if firewall != nil && ep != nil {
		for _, pm := range ep.PortMapping {
			firewall.RemovePortMapping(ep.IPv4Address, int(pm.HostPort), int(pm.ContainerPort), pm.Proto)
		}
	}
	return nil
}

func bridgeExists(name string) bool {
	_, err := os.Stat("/sys/class/net/" + name)
	return err == nil
}

func setupRootlessNetworking(pid int) error {
	cmd := exec.Command("pasta", "--pid", fmt.Sprintf("%d", pid))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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

// Inspect returns network details.
func (m *Manager) Inspect(idOrName string) (*common.NetworkInfo, error) {
	nw, err := m.GetNetwork(idOrName)
	if err != nil {
		return nil, err
	}
	info := m.toNetworkInfo(nw)
	return &info, nil
}
