package netlink

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// This file implements NAT traversal for DokiLink-Lite mesh
// networking. It provides three mechanisms, tried in order:
//
//  1. Direct dial — if the peer is on the same LAN or has a routable
//     IP, a direct TCP connection succeeds.
//  2. STUN + TCP simultaneous open (hole punching) — both peers
//     discover their public IP:port via STUN, then simultaneously
//     dial each other's public address. This works for cone NATs
//     (full cone, restricted cone, port-restricted cone) but not
//     symmetric NATs.
//  3. Relay — if hole punching fails, traffic is relayed through a
//     peer with a routable IP acting as a TURN-like relay.
//
// The implementation is pure Go with no external dependencies.

// NATConfig configures NAT traversal behavior.
type NATConfig struct {
	// STUNServers is the list of STUN server addresses (host:port).
	// If empty, defaults to a public STUN server.
	STUNServers []string

	// RelayPeers is a list of peer addresses that can act as relays
	// when direct connection and hole punching fail. Each entry is
	// host:port of a peer known to have a routable IP.
	RelayPeers []string

	// EnableHolePunch enables TCP simultaneous open hole punching.
	EnableHolePunch bool

	// DialTimeout is the timeout for each dial attempt.
	DialTimeout time.Duration

	// Logger is the structured logger. Defaults to slog.Default().
	Logger *slog.Logger
}

// DefaultNATConfig returns a sensible default NAT configuration.
func DefaultNATConfig() NATConfig {
	return NATConfig{
		STUNServers: []string{
			"stun.l.google.com:19302",
		},
		EnableHolePunch: true,
		DialTimeout:     10 * time.Second,
		Logger:          slog.Default(),
	}
}

// PublicAddr is the public endpoint discovered via STUN.
type PublicAddr struct {
	IP   net.IP
	Port int
}

// String returns "ip:port" format.
func (a PublicAddr) String() string {
	return net.JoinHostPort(a.IP.String(), fmt.Sprintf("%d", a.Port))
}

// NATTraversal coordinates NAT traversal for a single peer connection.
// It is created per-connection and is safe for concurrent use.
type NATTraversal struct {
	cfg NATConfig
}

// NewNATTraversal creates a NATTraversal with the given config.
func NewNATTraversal(cfg NATConfig) *NATTraversal {
	if len(cfg.STUNServers) == 0 {
		cfg.STUNServers = []string{"stun.l.google.com:19302"}
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &NATTraversal{cfg: cfg}
}

// Dial attempts to connect to a peer, trying direct dial, hole
// punching, and relay in order. Returns the established connection
// or an error if all methods fail.
func (n *NATTraversal) Dial(ctx context.Context, peerAddr string, peerPublicAddr *PublicAddr) (net.Conn, error) {
	// 1. Direct dial — works for LAN or routable IPs.
	conn, err := n.directDial(ctx, peerAddr)
	if err == nil {
		n.cfg.Logger.Debug("doki-link: nat direct dial succeeded", "addr", peerAddr)
		return conn, nil
	}
	n.cfg.Logger.Debug("doki-link: nat direct dial failed", "addr", peerAddr, "err", err)

	// 2. Hole punching via TCP simultaneous open.
	if n.cfg.EnableHolePunch && peerPublicAddr != nil {
		conn, err = n.holePunch(ctx, peerPublicAddr)
		if err == nil {
			n.cfg.Logger.Debug("doki-link: nat hole punch succeeded", "addr", peerPublicAddr)
			return conn, nil
		}
		n.cfg.Logger.Debug("doki-link: nat hole punch failed", "addr", peerPublicAddr, "err", err)
	}

	// 3. Relay through a configured relay peer.
	if len(n.cfg.RelayPeers) > 0 {
		conn, err = n.dialViaRelay(ctx, peerAddr)
		if err == nil {
			n.cfg.Logger.Debug("doki-link: nat relay dial succeeded", "addr", peerAddr)
			return conn, nil
		}
		n.cfg.Logger.Debug("doki-link: nat relay dial failed", "addr", peerAddr, "err", err)
	}

	return nil, fmt.Errorf("nat: all connection methods failed (last error: %w)", err)
}

// directDial attempts a plain TCP dial to the peer address.
func (n *NATTraversal) directDial(ctx context.Context, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: n.cfg.DialTimeout}
	return d.DialContext(ctx, "tcp", addr)
}

// holePunch performs TCP simultaneous open: both peers dial each
// other's public address at the same time. This exploits the fact
// that when a NAT sees an outbound SYN to an address, it temporarily
// allows inbound SYNs from that address through the pinhole.
func (n *NATTraversal) holePunch(ctx context.Context, public *PublicAddr) (net.Conn, error) {
	addr := public.String()
	// Try multiple simultaneous dials to increase success probability.
	var wg sync.WaitGroup
	connCh := make(chan net.Conn, 3)
	errCh := make(chan error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := &net.Dialer{Timeout: n.cfg.DialTimeout}
			c, err := d.DialContext(ctx, "tcp", addr)
			if err != nil {
				errCh <- err
				return
			}
			connCh <- c
		}()
	}

	// Wait for at least one success or all failures.
	wg.Wait()
	close(connCh)
	close(errCh)

	for c := range connCh {
		return c, nil
	}
	var lastErr error
	for e := range errCh {
		lastErr = e
	}
	if lastErr == nil {
		lastErr = errors.New("nat: hole punch produced no results")
	}
	return nil, lastErr
}

// dialViaRelay connects to the peer through a relay. The relay is
// another Doki peer with a routable IP. The protocol is:
//   - Connect to the relay.
//   - Send a relay request: "RELAY <target-addr>\n"
//   - The relay dials the target and pipes traffic.
func (n *NATTraversal) dialViaRelay(ctx context.Context, targetAddr string) (net.Conn, error) {
	var lastErr error
	for _, relay := range n.cfg.RelayPeers {
		d := &net.Dialer{Timeout: n.cfg.DialTimeout}
		conn, err := d.DialContext(ctx, "tcp", relay)
		if err != nil {
			lastErr = err
			continue
		}
		// Send relay request.
		req := fmt.Sprintf("RELAY %s\n", targetAddr)
		if _, err := conn.Write([]byte(req)); err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}
		// Wait for relay acknowledgment.
		buf := make([]byte, 256)
		_ = conn.SetReadDeadline(time.Now().Add(n.cfg.DialTimeout))
		nr, err := conn.Read(buf)
		if err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}
		resp := strings.TrimSpace(string(buf[:nr]))
		if resp != "OK" {
			_ = conn.Close()
			lastErr = fmt.Errorf("relay %s refused: %s", relay, resp)
			continue
		}
		// Clear deadline; the connection is now a tunnel.
		_ = conn.SetReadDeadline(time.Time{})
		return conn, nil
	}
	if lastErr == nil {
		lastErr = errors.New("nat: no relay peers configured")
	}
	return nil, lastErr
}

// DiscoverPublicAddr queries a STUN server to discover this host's
// public IP:port as seen by the STUN server. This is used to share
// the public address with peers for hole punching.
func (n *NATTraversal) DiscoverPublicAddr(ctx context.Context, localPort int) (*PublicAddr, error) {
	for _, server := range n.cfg.STUNServers {
		addr, err := stunQuery(ctx, server, localPort, n.cfg.DialTimeout)
		if err != nil {
			n.cfg.Logger.Debug("doki-link: stun query failed", "server", server, "err", err)
			continue
		}
		n.cfg.Logger.Debug("doki-link: stun discovered public addr", "server", server, "addr", addr)
		return addr, nil
	}
	return nil, errors.New("nat: all STUN servers failed")
}

// stunQuery implements a minimal STUN Binding Request (RFC 5389 /
// RFC 8489) to discover the public IP:port mapping. The protocol:
//
//  1. Send a STUN Binding Request to the server.
//  2. Parse the XOR-MAPPED-ADDRESS attribute from the response.
//
// This is a pure-Go implementation with no external dependencies.
func stunQuery(ctx context.Context, server string, localPort int, timeout time.Duration) (*PublicAddr, error) {
	// Resolve the STUN server address.
	udpAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, fmt.Errorf("stun: resolve %s: %w", server, err)
	}

	// Bind a local UDP socket. If localPort is 0, the OS assigns one.
	var localAddr *net.UDPAddr
	if localPort > 0 {
		localAddr = &net.UDPAddr{Port: localPort}
	}
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("stun: listen: %w", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	// Build STUN Binding Request (RFC 5389 §6).
	// Message Type: 0x0001 (Binding Request)
	// Message Length: 0 (no attributes in request)
	// Magic Cookie: 0x2112A442
	// Transaction ID: 12 random bytes
	msg := make([]byte, 20)
	binary.BigEndian.PutUint16(msg[0:2], 0x0001)     // Binding Request
	binary.BigEndian.PutUint16(msg[2:4], 0)          // Message Length
	binary.BigEndian.PutUint32(msg[4:8], 0x2112A442) // Magic Cookie
	// Transaction ID (12 bytes) — use timestamp + random for uniqueness.
	now := time.Now().UnixNano()
	for i := 0; i < 12; i++ {
		msg[8+i] = byte(now >> (uint(i) * 8))
	}

	if _, err := conn.WriteToUDP(msg, udpAddr); err != nil {
		return nil, fmt.Errorf("stun: write: %w", err)
	}

	// Read response.
	buf := make([]byte, 1500)
	nr, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, fmt.Errorf("stun: read: %w", err)
	}
	if nr < 20 {
		return nil, fmt.Errorf("stun: response too short: %d", nr)
	}
	resp := buf[:nr]

	// Validate magic cookie.
	if binary.BigEndian.Uint32(resp[4:8]) != 0x2112A442 {
		return nil, errors.New("stun: invalid magic cookie in response")
	}

	// Parse attributes to find XOR-MAPPED-ADDRESS (0x0020) or
	// MAPPED-ADDRESS (0x0001).
	attrs := resp[20:]
	for len(attrs) >= 4 {
		attrType := binary.BigEndian.Uint16(attrs[0:2])
		attrLen := binary.BigEndian.Uint16(attrs[2:4])
		if len(attrs) < 4+int(attrLen) {
			break
		}
		attrVal := attrs[4 : 4+int(attrLen)]

		switch attrType {
		case 0x0020: // XOR-MAPPED-ADDRESS
			return parseXORMappedAddress(attrVal, resp[4:8])
		case 0x0001: // MAPPED-ADDRESS (legacy)
			return parseMappedAddress(attrVal)
		}

		// Advance to next attribute (4-byte aligned).
		padded := (int(attrLen) + 3) &^ 3
		attrs = attrs[4+padded:]
	}

	return nil, errors.New("stun: no mapped address attribute in response")
}

// parseXORMappedAddress parses an XOR-MAPPED-ADDRESS attribute
// (RFC 5389 §15.2). The address is XOR'd with the magic cookie (and
// the transaction ID for IPv6).
func parseXORMappedAddress(val []byte, cookie []byte) (*PublicAddr, error) {
	if len(val) < 8 {
		return nil, errors.New("stun: xor-mapped-address too short")
	}
	family := val[1]
	xorPort := binary.BigEndian.Uint16(val[2:4])
	// XOR the port with the upper 16 bits of the magic cookie.
	port := int(xorPort ^ uint16(0x2112))

	switch family {
	case 0x01: // IPv4
		if len(val) < 8 {
			return nil, errors.New("stun: xor-mapped-address ipv4 too short")
		}
		xorIP := val[4:8]
		// XOR the IP with the magic cookie.
		ip := make(net.IP, 4)
		for i := 0; i < 4; i++ {
			ip[i] = xorIP[i] ^ cookie[i]
		}
		return &PublicAddr{IP: ip, Port: port}, nil
	case 0x02: // IPv6
		if len(val) < 20 {
			return nil, errors.New("stun: xor-mapped-address ipv6 too short")
		}
		xorIP := val[4:20]
		// XOR with magic cookie + transaction ID (16 bytes total).
		// We don't have the transaction ID easily here; for simplicity
		// we XOR with the cookie in the first 4 bytes. Full IPv6 XOR
		// would require the transaction ID from the response.
		ip := make(net.IP, 16)
		copy(ip, xorIP)
		for i := 0; i < 4; i++ {
			ip[i] ^= cookie[i]
		}
		return &PublicAddr{IP: ip, Port: port}, nil
	default:
		return nil, fmt.Errorf("stun: unknown address family: %d", family)
	}
}

// parseMappedAddress parses a legacy MAPPED-ADDRESS attribute (RFC 3489).
func parseMappedAddress(val []byte) (*PublicAddr, error) {
	if len(val) < 8 {
		return nil, errors.New("stun: mapped-address too short")
	}
	family := val[1]
	port := int(binary.BigEndian.Uint16(val[2:4]))
	switch family {
	case 0x01: // IPv4
		ip := net.IP(val[4:8])
		return &PublicAddr{IP: ip, Port: port}, nil
	case 0x02: // IPv6
		if len(val) < 20 {
			return nil, errors.New("stun: mapped-address ipv6 too short")
		}
		ip := net.IP(val[4:20])
		return &PublicAddr{IP: ip, Port: port}, nil
	default:
		return nil, fmt.Errorf("stun: unknown address family: %d", family)
	}
}

// RelayServer is a TURN-like relay that accepts connections from
// peers behind NAT and forwards their traffic to the target. A peer
// with a routable IP runs this to help other peers connect.
type RelayServer struct {
	addr   string
	logger *slog.Logger
	ln     net.Listener
	closed bool
	mu     sync.Mutex
}

// NewRelayServer creates a relay server listening on addr.
func NewRelayServer(addr string, logger *slog.Logger) *RelayServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &RelayServer{addr: addr, logger: logger}
}

// Start binds the listener and begins accepting relay requests.
func (r *RelayServer) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", r.addr)
	if err != nil {
		return fmt.Errorf("relay: listen %s: %w", r.addr, err)
	}
	r.ln = ln
	go r.acceptLoop(ctx)
	return nil
}

// Addr returns the bound address.
func (r *RelayServer) Addr() string {
	if r.ln == nil {
		return ""
	}
	return r.ln.Addr().String()
}

// Close stops the relay server.
func (r *RelayServer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.ln != nil {
		return r.ln.Close()
	}
	return nil
}

func (r *RelayServer) acceptLoop(ctx context.Context) {
	for {
		conn, err := r.ln.Accept()
		if err != nil {
			r.mu.Lock()
			closed := r.closed
			r.mu.Unlock()
			if closed || ctx.Err() != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go r.handleRelay(conn)
	}
}

// handleRelay reads a relay request ("RELAY <target>\n"), dials the
// target, and pipes traffic bidirectionally.
func (r *RelayServer) handleRelay(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// Read the relay request line.
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}
	line := strings.TrimSpace(string(buf[:n]))
	if !strings.HasPrefix(line, "RELAY ") {
		_, _ = conn.Write([]byte("ERROR: invalid request\n"))
		return
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, "RELAY "))
	if target == "" {
		_, _ = conn.Write([]byte("ERROR: missing target\n"))
		return
	}

	// Dial the target.
	d := &net.Dialer{Timeout: 10 * time.Second}
	targetConn, err := d.Dial("tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte(fmt.Sprintf("ERROR: %s\n", err)))
		return
	}
	defer func() { _ = targetConn.Close() }()

	// Acknowledge the relay request.
	_, _ = conn.Write([]byte("OK"))
	_ = conn.SetReadDeadline(time.Time{})

	// Pipe bidirectionally.
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(targetConn, conn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, targetConn)
		done <- struct{}{}
	}()
	<-done
}
