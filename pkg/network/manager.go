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
	EndpointID  string `json:"EndpointID"`
	MacAddress  string `json:"MacAddress"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
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
func NewManager(root string) (*Manager, error) {
	common.EnsureDir(root)

	m := &Manager{
		root:     root,
		networks: make(map[string]*Network),
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

// Connect connects a container to a network.
func (m *Manager) Connect(networkID, containerID string, ipAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nw, err := m.loadNetwork(networkID)
	if err != nil {
		return err
	}

	if _, exists := nw.Containers[containerID]; exists {
		return nil
	}

	ep := &Endpoint{
		EndpointID:  common.GenerateID(64),
		MacAddress:  generateMacAddr(),
		IPv4Address: ipAddr,
	}

	if ipAddr == "" && nw.Driver == "bridge" {
		ep.IPv4Address = m.allocateIP(nw)
	}

	nw.Containers[containerID] = ep
	return m.saveNetwork(nw)
}

// Disconnect disconnects a container from a network.
func (m *Manager) Disconnect(networkID, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nw, err := m.loadNetwork(networkID)
	if err != nil {
		return err
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

	// Iterate all valid host addresses in subnet.
	ip := ipNet.IP.To4()
	if ip == nil {
		return ""
	}
	mask := ipNet.Mask
	// Skip network address (all 0)
	incrementIP(ip, mask)
	// Skip gateway (.1)
	incrementIP(ip, mask)

	for ipNet.Contains(ip) {
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
func (m *Manager) SetupNetwork(containerID, containerPid int, networkName string) error {
	if networkName == "none" {
		return nil
	}

	if networkName == "host" {
		return setupHostNetwork(containerPid)
	}

	nw, err := m.GetNetwork(networkName)
	if err != nil {
		return err
	}

	if nw.Driver == "bridge" {
		return setupBridgeNetwork(containerID, containerPid, nw)
	}

	return nil
}

func setupHostNetwork(pid int) error {
	// Join the host's network namespace.
	nsPath := fmt.Sprintf("/proc/1/ns/net")
	targetPath := fmt.Sprintf("/proc/%d/ns/net", pid)

	// Copy the namespace by using nsenter.
	cmd := exec.Command("nsenter", "--net="+nsPath, "true")
	_ = targetPath
	return cmd.Run()
}

func setupBridgeNetwork(containerID, pid int, nw *Network) error {
	// For rootless, use pasta or slirp4netns.
	if os.Geteuid() != 0 {
		return setupRootlessNetworking(pid)
	}

	// Create veth pair.
	vethName := fmt.Sprintf("doki%d", pid)

	cmd := exec.Command("ip", "link", "add", vethName, "type", "veth",
		"peer", "name", "eth0")
	if err := cmd.Run(); err != nil {
		return err
	}

	// Move eth0 into container namespace.
	cmd = exec.Command("ip", "link", "set", "eth0", "netns",
		fmt.Sprintf("%d", pid))
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
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
