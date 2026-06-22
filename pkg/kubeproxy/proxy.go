// Package kubeproxy provides the Kubernetes kube-proxy.
package kubeproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

type ProxyMode string

const (
	ModeIPTables  ProxyMode = "iptables"
	ModeNFTables  ProxyMode = "nftables"
	ModeUserspace ProxyMode = "userspace"
)

type Proxy struct {
	store       store.Store
	mode        ProxyMode
	clusterCIDR string
	services    map[string]*ServiceProxy
	mu          sync.RWMutex
	logger      *slog.Logger

	// userspace proxies: ClusterIP:port -> *userspaceProxy
	usProxies   map[string]*userspaceProxy
	usProxiesMu sync.Mutex
}

type ServiceProxy struct {
	Name      string
	Namespace string
	ClusterIP string
	Ports     []k8s.ServicePort
	Endpoints []EndpointEntry
}

type EndpointEntry struct {
	IP   string
	Port int32
}

func NewProxy(s store.Store, mode ProxyMode, clusterCIDR string, logger *slog.Logger) *Proxy {
	if logger == nil {
		logger = slog.Default()
	}
	return &Proxy{
		store:       s,
		mode:        mode,
		clusterCIDR: clusterCIDR,
		services:    make(map[string]*ServiceProxy),
		logger:      logger,
		usProxies:   make(map[string]*userspaceProxy),
	}
}

func (p *Proxy) Run(ctx context.Context) error {
	go p.watchServices(ctx)
	go p.watchEndpoints(ctx)
	p.logger.Info("kube-proxy started", "mode", p.mode)
	<-ctx.Done()
	return nil
}

func (p *Proxy) watchServices(ctx context.Context) {
	prefix := store.KeyFor("", "services", "", "")
	ch, err := p.store.Watch(prefix, 0)
	if err != nil {
		p.logger.Error("watch services failed", "error", err)
		return
	}
	defer p.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			var svc k8s.Service
			if err := json.Unmarshal(event.Object.Value, &svc); err != nil {
				continue
			}
			p.syncService(&svc, event.Type)
		}
	}
}

func (p *Proxy) watchEndpoints(ctx context.Context) {
	prefix := store.KeyFor("", "endpoints", "", "")
	ch, err := p.store.Watch(prefix, 0)
	if err != nil {
		p.logger.Error("watch endpoints fails", "error", err)
		return
	}
	defer p.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			var ep k8s.Endpoints
			if err := json.Unmarshal(event.Object.Value, &ep); err != nil {
				continue
			}
			p.syncEndpoints(&ep)
		}
	}
}

func (p *Proxy) syncService(svc *k8s.Service, eventType string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := svc.Namespace + "/" + svc.Name

	if eventType == store.EventDeleted {
		delete(p.services, key)
		p.logger.Info("service removed", "service", key)
		return
	}

	sp := &ServiceProxy{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		ClusterIP: svc.Spec.ClusterIP,
		Ports:     svc.Spec.Ports,
	}

	p.services[key] = sp
	p.syncRules()
	p.logger.Info("service synced", "service", key, "clusterIP", svc.Spec.ClusterIP)
}

func (p *Proxy) syncEndpoints(ep *k8s.Endpoints) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := ep.Namespace + "/" + ep.Name
	sp, ok := p.services[key]
	if !ok {
		return
	}

	sp.Endpoints = nil
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			for _, port := range subset.Ports {
				sp.Endpoints = append(sp.Endpoints, EndpointEntry{
					IP:   addr.IP,
					Port: port.Port,
				})
			}
		}
	}
	p.syncRules()
}

func (p *Proxy) syncRules() {
	switch p.mode {
	case ModeIPTables:
		p.syncIPTables()
	case ModeNFTables:
		p.syncNFTables()
	case ModeUserspace:
		p.syncUserspace()
	}
}

func (p *Proxy) syncIPTables() {
	// Build iptables rules and apply them via the iptables binary.
	// On platforms without iptables (e.g., Termux without root),
	// log a warning and fall back to userspace mode.
	iptBin, err := exec.LookPath("iptables")
	if err != nil {
		p.logger.Warn("kube-proxy: iptables not found, skipping iptables sync", "err", err)
		return
	}

	// Ensure the DOKI-SERVICES chain exists in the nat table.
	ensureChain(iptBin, "nat", "DOKI-SERVICES")
	ensureChain(iptBin, "nat", "DOKI-MASQUERADE")

	// Flush the DOKI-SERVICES chain for a clean slate.
	runIPTables([]string{"-t", "nat", "-F", "DOKI-SERVICES"}, p.logger)

	// Hook DOKI-SERVICES into PREROUTING and OUTPUT if not already.
	ensureRule(iptBin, "-t", "nat", "-A", "PREROUTING", "-j", "DOKI-SERVICES")
	ensureRule(iptBin, "-t", "nat", "-A", "OUTPUT", "-j", "DOKI-SERVICES")

	for _, sp := range p.services {
		if sp.ClusterIP == "" || sp.ClusterIP == "None" {
			continue
		}
		for _, port := range sp.Ports {
			proto := strings.ToLower(port.Protocol)
			if proto == "" {
				proto = "tcp"
			}
			svcChain := fmt.Sprintf("DOKI-SVC-%s-%s", sp.Name, sp.Namespace)
			ensureChain(iptBin, "nat", svcChain)
			runIPTables([]string{"-t", "nat", "-F", svcChain}, p.logger)

			// Rule in DOKI-SERVICES to jump to the per-service chain.
			runIPTables([]string{"-t", "nat", "-A", "DOKI-SERVICES",
				"-d", sp.ClusterIP + "/32",
				"-p", proto,
				"--dport", fmt.Sprintf("%d", port.Port),
				"-j", svcChain}, p.logger)

			// DNAT rules to each endpoint inside the per-service chain.
			for _, ep := range sp.Endpoints {
				runIPTables([]string{"-t", "nat", "-A", svcChain,
					"-p", proto,
					"-j", "DNAT",
					"--to-destination", net.JoinHostPort(ep.IP, strconv.Itoa(int(ep.Port)))}, p.logger)
			}

			// MASQUERADE for pod egress (source NAT).
			if p.clusterCIDR != "" {
				ensureRule(iptBin, "-t", "nat", "-A", "DOKI-MASQUERADE",
					"-s", p.clusterCIDR,
					"-j", "MASQUERADE")
				ensureRule(iptBin, "-t", "nat", "-A", "POSTROUTING",
					"-j", "DOKI-MASQUERADE")
			}
		}
	}
}

func (p *Proxy) syncNFTables() {
	nftBin, err := exec.LookPath("nft")
	if err != nil {
		p.logger.Warn("kube-proxy: nft not found, skipping nftables sync", "err", err)
		return
	}

	// Build nft ruleset for service routing.
	var rules strings.Builder
	rules.WriteString("table ip doki { }\n")
	rules.WriteString("flush table ip doki\n")
	rules.WriteString("table ip doki {\n")
	rules.WriteString("  chain services {\n")
	for _, sp := range p.services {
		if sp.ClusterIP == "" || sp.ClusterIP == "None" {
			continue
		}
		for _, port := range sp.Ports {
			proto := strings.ToLower(port.Protocol)
			if proto == "" {
				proto = "tcp"
			}
			if len(sp.Endpoints) > 0 {
				ep := sp.Endpoints[0]
				rules.WriteString(fmt.Sprintf(
					"    ip daddr %s %s dport %d dnat to %s:%d\n",
					sp.ClusterIP, proto, port.Port, ep.IP, ep.Port))
			}
		}
	}
	rules.WriteString("  }\n")
	rules.WriteString("  chain prerouting { type nat hook prerouting priority -100; ip daddr 10.96.0.0/16 jump services }\n")
	rules.WriteString("  chain postrouting { type nat hook postrouting priority 100; ")
	if p.clusterCIDR != "" {
		rules.WriteString(fmt.Sprintf("ip saddr %s masquerade", p.clusterCIDR))
	}
	rules.WriteString(" }\n")
	rules.WriteString("}\n")

	cmd := exec.Command(nftBin, "-f", "-")
	cmd.Stdin = strings.NewReader(rules.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		p.logger.Warn("kube-proxy: nft apply failed", "err", err, "output", string(output))
	}
}

func (p *Proxy) syncUserspace() {
	// Userspace proxy: for each service port, bind to the ClusterIP:port
	// and round-robin connections to endpoints. This is the only mode
	// that works without root or iptables (e.g., on Termux).
	p.usProxiesMu.Lock()
	defer p.usProxiesMu.Unlock()

	active := make(map[string]bool)
	for _, sp := range p.services {
		if sp.ClusterIP == "" || sp.ClusterIP == "None" {
			continue
		}
		for _, port := range sp.Ports {
			proto := strings.ToLower(port.Protocol)
			if proto == "" {
				proto = "tcp"
			}
			key := fmt.Sprintf("%s:%d:%s", sp.ClusterIP, port.Port, proto)
			active[key] = true

			if _, exists := p.usProxies[key]; !exists {
				listenAddr := fmt.Sprintf("%s:%d", sp.ClusterIP, port.Port)
				us := newUserspaceProxy(listenAddr, proto, sp.Endpoints, p.logger)
				if err := us.Start(); err != nil {
					p.logger.Warn("kube-proxy: userspace proxy start failed",
						"addr", listenAddr, "err", err)
					continue
				}
				p.usProxies[key] = us
				p.logger.Info("kube-proxy: userspace proxy started",
					"addr", listenAddr, "proto", proto)
			} else {
				// Update endpoints.
				p.usProxies[key].UpdateEndpoints(sp.Endpoints)
			}
		}
	}

	// Stop proxies for services that no longer exist.
	for key, us := range p.usProxies {
		if !active[key] {
			us.Stop()
			delete(p.usProxies, key)
		}
	}
}

// --- iptables helpers ---

func ensureChain(bin, table, chain string) {
	cmd := exec.Command(bin, "-t", table, "-N", chain)
	_ = cmd.Run()
}

func ensureRule(bin string, args ...string) {
	logger := slog.Default()
	check := append([]string{"-C"}, args[1:]...)
	cmd := exec.Command(bin, check...)
	if err := cmd.Run(); err == nil {
		return
	}
	cmd = exec.Command(bin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Debug("kube-proxy: iptables ensureRule failed",
			"args", strings.Join(args, " "), "err", err, "output", string(output))
	}
}

func runIPTables(args []string, logger *slog.Logger) {
	bin := "iptables"
	cmd := exec.Command(bin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Debug("kube-proxy: iptables rule failed",
			"args", strings.Join(args, " "), "err", err, "output", string(output))
	}
}

// --- userspace proxy ---

type userspaceProxy struct {
	listenAddr string
	proto      string
	endpoints  []EndpointEntry
	rrIdx      int
	mu         sync.Mutex
	listener   net.Listener
	udpConn    *net.UDPConn
	logger     *slog.Logger
	stopCh     chan struct{}
	stopOnce   sync.Once
}

func newUserspaceProxy(listenAddr, proto string, endpoints []EndpointEntry, logger *slog.Logger) *userspaceProxy {
	return &userspaceProxy{
		listenAddr: listenAddr,
		proto:      proto,
		endpoints:  endpoints,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

func (u *userspaceProxy) Start() error {
	switch u.proto {
	case "tcp":
		ln, err := net.Listen("tcp", u.listenAddr)
		if err != nil {
			return err
		}
		u.listener = ln
		go u.acceptTCP()
	case "udp":
		addr, err := net.ResolveUDPAddr("udp", u.listenAddr)
		if err != nil {
			return err
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			return err
		}
		u.udpConn = conn
		go u.acceptUDP()
	}
	return nil
}

func (u *userspaceProxy) Stop() {
	u.stopOnce.Do(func() { close(u.stopCh) })
	if u.listener != nil {
		_ = u.listener.Close()
	}
	if u.udpConn != nil {
		_ = u.udpConn.Close()
	}
}

func (u *userspaceProxy) UpdateEndpoints(eps []EndpointEntry) {
	u.mu.Lock()
	u.endpoints = eps
	u.mu.Unlock()
}

func (u *userspaceProxy) nextEndpoint() (EndpointEntry, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.endpoints) == 0 {
		return EndpointEntry{}, false
	}
	ep := u.endpoints[u.rrIdx%len(u.endpoints)]
	u.rrIdx++
	return ep, true
}

func (u *userspaceProxy) acceptTCP() {
	for {
		conn, err := u.listener.Accept()
		if err != nil {
			select {
			case <-u.stopCh:
				return
			default:
				u.logger.Debug("kube-proxy: userspace accept error", "err", err)
				return
			}
		}
		go u.handleTCP(conn)
	}
}

func (u *userspaceProxy) handleTCP(client net.Conn) {
	defer func() { _ = client.Close() }()
	ep, ok := u.nextEndpoint()
	if !ok {
		return
	}
	upstream, err := net.Dial("tcp", net.JoinHostPort(ep.IP, strconv.Itoa(int(ep.Port))))
	if err != nil {
		u.logger.Debug("kube-proxy: userspace dial upstream failed", "err", err)
		return
	}
	defer func() { _ = upstream.Close() }()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
	<-done
}

func (u *userspaceProxy) acceptUDP() {
	buf := make([]byte, 65535)
	for {
		select {
		case <-u.stopCh:
			return
		default:
		}
		_ = u.udpConn.SetReadDeadline(time.Time{})
		n, peer, err := u.udpConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		ep, ok := u.nextEndpoint()
		if !ok {
			continue
		}
		go func(data []byte, peer *net.UDPAddr, ep EndpointEntry) {
			upstream, err := net.Dial("udp", net.JoinHostPort(ep.IP, strconv.Itoa(int(ep.Port))))
			if err != nil {
				return
			}
			defer func() { _ = upstream.Close() }()
			if _, err := upstream.Write(data); err != nil {
				return
			}
			resp := make([]byte, 65535)
			_ = upstream.SetReadDeadline(time.Time{})
			rn, err := upstream.Read(resp)
			if err != nil {
				return
			}
			_, _ = u.udpConn.WriteToUDP(resp[:rn], peer)
		}(buf[:n], peer, ep)
	}
}
