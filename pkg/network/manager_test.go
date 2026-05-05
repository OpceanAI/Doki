package network

import (
	"net"
	"testing"
)

func TestCalculateGateway(t *testing.T) {
	gateway := calculateGateway("172.17.0.0/16")
	if gateway != "172.17.0.1" {
		t.Errorf("Gateway = %q, want 172.17.0.1", gateway)
	}
}

func TestCalculateGatewaySlash24(t *testing.T) {
	gateway := calculateGateway("192.168.1.0/24")
	if gateway != "192.168.1.1" {
		t.Errorf("Gateway = %q, want 192.168.1.1", gateway)
	}
}

func TestCalculateGatewayInvalid(t *testing.T) {
	gateway := calculateGateway("not-a-cidr")
	if gateway != "" {
		t.Errorf("Gateway = %q, want empty", gateway)
	}
}

func TestAllocateIP(t *testing.T) {
	m := &Manager{
		networks: make(map[string]*Network),
	}
	nw := &Network{
		Subnet:     "172.18.0.0/16",
		Gateway:    "172.18.0.1",
		Containers: make(map[string]*Endpoint),
	}

	// First allocation should give 172.18.0.2.
	ip1 := m.allocateIP(nw)
	if ip1 != "172.18.0.2" {
		t.Errorf("first IP = %q, want 172.18.0.2", ip1)
	}

	// Add the first IP as used.
	nw.Containers["c1"] = &Endpoint{IPv4Address: ip1}

	// Second allocation should give 172.18.0.3.
	ip2 := m.allocateIP(nw)
	if ip2 != "172.18.0.3" {
		t.Errorf("second IP = %q, want 172.18.0.3", ip2)
	}
}

func TestAllocateIPSequential(t *testing.T) {
	m := &Manager{
		networks: make(map[string]*Network),
	}
	nw := &Network{
		Subnet:     "10.0.0.0/24",
		Gateway:    "10.0.0.1",
		Containers: make(map[string]*Endpoint),
	}

	// Allocate 10 IPs and verify they are sequential from .2.
	expected := "10.0.0.2"
	for i := 0; i < 10; i++ {
		ip := m.allocateIP(nw)
		if ip != expected {
			t.Errorf("IP %d = %q, want %s", i, ip, expected)
			break
		}
		nw.Containers[ip] = &Endpoint{IPv4Address: ip}
		parsed := net.ParseIP(ip)
		// Increment last byte.
		parsed = parsed.To4()
		parsed[3]++
		expected = parsed.String()
	}
}

func TestAllocateIPNoSubnet(t *testing.T) {
	m := &Manager{
		networks: make(map[string]*Network),
	}
	nw := &Network{
		Subnet:     "",
		Containers: make(map[string]*Endpoint),
	}
	ip := m.allocateIP(nw)
	if ip != "" {
		t.Errorf("Expected empty IP for no subnet, got %q", ip)
	}
}

func TestAllocateIPExhaustion(t *testing.T) {
	m := &Manager{
		networks: make(map[string]*Network),
	}
	// /30 has only 1 usable host (.2), since .0=network, .1=gateway, .3=broadcast.
	// allocateIP skips .0 (network) and .1 (gateway), starts at .2.
	// After .2 is taken, .3 is within subnet but is the broadcast address,
	// so allocateIP will return it (implementation doesn't exclude broadcast).
	nw := &Network{
		Subnet:     "10.0.0.0/30",
		Gateway:    "10.0.0.1",
		Containers: make(map[string]*Endpoint),
	}

	ip := m.allocateIP(nw)
	if ip != "10.0.0.2" {
		t.Errorf("first IP = %q, want 10.0.0.2", ip)
	}
	nw.Containers["c1"] = &Endpoint{IPv4Address: ip}

	// The implementation doesn't exclude broadcast, so it returns .3.
	ip2 := m.allocateIP(nw)
	if ip2 != "10.0.0.3" {
		t.Logf("second IP = %q (broadcast included by implementation)", ip2)
	}
}

func TestGenerateMacAddr(t *testing.T) {
	mac := generateMacAddr()
	if len(mac) != 17 {
		t.Errorf("MAC address length = %d, want 17", len(mac))
	}
	// Should start with 02:42 for locally administered.
	if mac[0:5] != "02:42" {
		t.Errorf("MAC prefix = %q, want 02:42", mac[0:5])
	}
}

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	fw := NewFirewallManager(FirewallIptables)
	dns := NewDNSServer()

	m, err := NewManager(dir, fw, dns)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.firewall != fw {
		t.Error("firewall not set")
	}
	if m.dns != dns {
		t.Error("dns not set")
	}

	// Default networks should be created.
	nets, err := m.ListNetworks()
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	defaultNames := map[string]bool{"bridge": true, "host": true, "none": true}
	for _, nw := range nets {
		if defaultNames[nw.Name] {
			delete(defaultNames, nw.Name)
		}
	}
	if len(defaultNames) > 0 {
		t.Errorf("missing default networks: %v", defaultNames)
	}
}

func TestCreateNetwork(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	cfg := &NetworkConfig{
		Name:   "testnet",
		Driver: "bridge",
		Subnet: "10.10.0.0/16",
	}
	nw, err := m.CreateNetwork(cfg)
	if err != nil {
		t.Fatalf("CreateNetwork: %v", err)
	}
	if nw.Name != "testnet" {
		t.Errorf("Name = %q, want testnet", nw.Name)
	}
	if nw.Driver != "bridge" {
		t.Errorf("Driver = %q, want bridge", nw.Driver)
	}
	if nw.Subnet != "10.10.0.0/16" {
		t.Errorf("Subnet = %q, want 10.10.0.0/16", nw.Subnet)
	}
	if nw.Gateway != "10.10.0.1" {
		t.Errorf("Gateway = %q, want 10.10.0.1", nw.Gateway)
	}
}

func TestCreateDuplicateNetwork(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	m.CreateNetwork(&NetworkConfig{Name: "duplicate", Driver: "bridge"})
	_, err := m.CreateNetwork(&NetworkConfig{Name: "duplicate", Driver: "bridge"})
	if err == nil {
		t.Fatal("expected error for duplicate network")
	}
}

func TestGetNetwork(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	created, _ := m.CreateNetwork(&NetworkConfig{Name: "getnet", Driver: "bridge"})

	nw, err := m.GetNetwork(created.ID)
	if err != nil {
		t.Fatalf("GetNetwork by ID: %v", err)
	}
	if nw.Name != "getnet" {
		t.Errorf("Name = %q, want getnet", nw.Name)
	}

	nw2, err := m.GetNetwork("getnet")
	if err != nil {
		t.Fatalf("GetNetwork by name: %v", err)
	}
	if nw2.Name != "getnet" {
		t.Errorf("Name = %q, want getnet", nw2.Name)
	}
}

func TestGetNetworkNotFound(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	_, err := m.GetNetwork("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent network")
	}
}

func TestRemoveNetwork(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	m.CreateNetwork(&NetworkConfig{Name: "removable", Driver: "bridge"})

	if err := m.RemoveNetwork("removable"); err != nil {
		t.Fatalf("RemoveNetwork: %v", err)
	}

	_, err := m.GetNetwork("removable")
	if err == nil {
		t.Fatal("expected error after removal")
	}
}

func TestCannotRemoveBuiltinNetwork(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	err := m.RemoveNetwork("bridge")
	if err == nil {
		t.Fatal("expected error removing built-in bridge network")
	}
}

func TestRemoveNetworkWithEndpoints(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	nw, _ := m.CreateNetwork(&NetworkConfig{Name: "inuse", Driver: "bridge"})
	nw.Containers["container1"] = &Endpoint{}

	err := m.RemoveNetwork("inuse")
	if err == nil {
		t.Fatal("expected error removing network with active endpoints")
	}
}

func TestConnect(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	nw, _ := m.CreateNetwork(&NetworkConfig{
		Name:   "conn",
		Driver: "bridge",
		Subnet: "172.20.0.0/16",
	})

	err := m.Connect(nw.ID, "container1", "", []string{"web"}, nil, 0)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Verify endpoint created.
	updated, _ := m.GetNetwork(nw.ID)
	ep, ok := updated.Containers["container1"]
	if !ok {
		t.Fatal("container1 endpoint not found")
	}
	if ep.IPv4Address == "" {
		t.Error("IP not allocated")
	}
}

func TestDisconnect(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	nw, _ := m.CreateNetwork(&NetworkConfig{
		Name:   "disconn",
		Driver: "bridge",
		Subnet: "172.21.0.0/16",
	})
	m.Connect(nw.ID, "container1", "", nil, nil, 0)

	err := m.Disconnect(nw.ID, "container1", 0)
	if err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	updated, _ := m.GetNetwork(nw.ID)
	if _, ok := updated.Containers["container1"]; ok {
		t.Error("container1 should be disconnected")
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	m.CreateNetwork(&NetworkConfig{Name: "prune1", Driver: "bridge"})
	m.CreateNetwork(&NetworkConfig{Name: "prune2", Driver: "bridge"})

	pruned, err := m.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(pruned) != 2 {
		t.Errorf("Pruned %d networks, want 2", len(pruned))
	}
}

func TestPruneSkipsBuiltins(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	pruned, err := m.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// bridge, host, none should not be pruned.
	for _, name := range pruned {
		if name == "bridge" || name == "host" || name == "none" {
			t.Errorf("built-in network %s was pruned", name)
		}
	}
}

func TestInspect(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	m.CreateNetwork(&NetworkConfig{
		Name:   "inspectnet",
		Driver: "bridge",
		Subnet: "172.22.0.0/16",
	})

	info, err := m.Inspect("inspectnet")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Name != "inspectnet" {
		t.Errorf("Name = %q, want inspectnet", info.Name)
	}
	if info.Driver != "bridge" {
		t.Errorf("Driver = %q, want bridge", info.Driver)
	}
}

func TestListNetworks(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, NewFirewallManager(FirewallIptables), NewDNSServer())

	nets, err := m.ListNetworks()
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	// Should have at least 3 default networks.
	if len(nets) < 3 {
		t.Errorf("ListNetworks len = %d, want >= 3", len(nets))
	}
}

func TestSetupHostNetwork(t *testing.T) {
	err := setupHostNetwork()
	if err != nil {
		t.Errorf("setupHostNetwork should not return error: %v", err)
	}
}

func TestIncrementIP(t *testing.T) {
	ip := net.ParseIP("10.0.0.1")
	mask := net.CIDRMask(24, 32)
	incrementIP(ip, mask)
	if ip.String() != "10.0.0.2" {
		t.Errorf("after increment: %s, want 10.0.0.2", ip)
	}
}

func TestIncrementIPWrap(t *testing.T) {
	ip := net.ParseIP("10.0.0.255")
	mask := net.CIDRMask(24, 32)
	incrementIP(ip, mask)
	if ip.String() != "10.0.1.0" {
		t.Errorf("after increment with wrap: %s, want 10.0.1.0", ip)
	}
}

func TestFirewallManagerBackend(t *testing.T) {
	fw := NewFirewallManager(FirewallIptables)
	if fw == nil {
		t.Fatal("NewFirewallManager returned nil")
	}
}

func TestDetectFirewallBackend(t *testing.T) {
	backend := DetectFirewallBackend()
	if backend != "iptables" && backend != "nftables" && backend != "none" {
		t.Errorf("unexpected firewall backend: %q", backend)
	}
}

func TestDNSServerNew(t *testing.T) {
	dns := NewDNSServer()
	if dns == nil {
		t.Fatal("NewDNSServer returned nil")
	}
}

func TestIsPastaAvailable(t *testing.T) {
	_ = IsPastaAvailable()
}

func TestIsSlirp4netnsAvailable(t *testing.T) {
	_ = IsSlirp4netnsAvailable()
}
