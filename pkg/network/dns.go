// Package network: container DNS resolution.
//
// DNSServer is a small authoritative+forwarding DNS server that:
//  1. Resolves container names (a/b -> 172.17.0.x) locally.
//  2. Forwards everything else to upstream resolvers (host resolv.conf by default).
//  3. Caches both positive and negative responses with TTL.
//  4. Handles concurrent queries safely.
//  5. Supports both UDP and TCP (RFC 5966).
//  6. Handles A and AAAA queries.
//
// It binds to 127.0.0.11:53 (well-known, like Docker) by default and is
// exposed on the container's loopback inside the namespace via the runtime's
// network setup. Containers get 127.0.0.11 in their /etc/resolv.conf.
package network

import (
	"container/list"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DNSWellKnownAddress returns the well-known address for the internal DNS server.
// Docker uses 127.0.0.11:53; on Android we use 127.0.0.11:8053 (port 53 is blocked).
func DNSWellKnownAddress() string {
	return "127.0.0.11:53"
}

// DNSServer provides internal DNS resolution for containers.
type DNSServer struct {
	mu      sync.RWMutex
	entries map[string]string // name → IPv4
	entriesV6 map[string]string // name → IPv6 (AAAA)
	reverse   map[string]string // IP → name (PTR)
	cache   *dnsCache

	upstream     []string
	positiveTTL  time.Duration
	negativeTTL  time.Duration
	queryTimeout time.Duration

	listener    *net.UDPConn
	tcpListener *net.TCPListener
	addr        string // actual bound address (after Start)
	started     atomic.Bool
	stopCh      chan struct{}
	wg          sync.WaitGroup
	log         *slog.Logger

	// metrics
	queriesServed atomic.Uint64
	cacheHits     atomic.Uint64
	cacheMisses   atomic.Uint64
	forwards      atomic.Uint64
	forwardFails  atomic.Uint64
	tcpQueries    atomic.Uint64
}

// NewDNSServer creates a DNS server with the given upstream resolvers.
// If upstreams is empty, tries Android DNS discovery (getprop),
// then host /etc/resolv.conf, falls back to 8.8.8.8 + 1.1.1.1.
func NewDNSServer(upstream []string) *DNSServer {
	if len(upstream) == 0 {
		// Android DNS via getprop (no /etc/resolv.conf on Android).
		if dns := AndroidDNSServers(); len(dns) > 0 {
			upstream = dns
		} else {
			// Read host resolv.conf for upstream servers.
			rc := HostResolvConf()
			upstream = rc.NameserverList()
		}
	}
	// Ensure port is set.
	for i, u := range upstream {
		if !strings.Contains(u, ":") {
			upstream[i] = net.JoinHostPort(u, "53")
		}
	}
	return &DNSServer{
		entries:      make(map[string]string),
		entriesV6:    make(map[string]string),
		reverse:      make(map[string]string),
		cache:        newDNSCache(256, 5*time.Minute),
		upstream:     upstream,
		positiveTTL:  5 * time.Minute,
		negativeTTL:  30 * time.Second,
		queryTimeout: 3 * time.Second,
		stopCh:       make(chan struct{}),
		log:          slog.Default().With("component", "dns"),
	}
}

// AddEntry registers a name -> IP mapping. Supports both IPv4 and IPv6.
// id-qualified names (a/b, container_id) take precedence over plain aliases.
func (d *DNSServer) AddEntry(name, ip string) {
	if name == "" || ip == "" {
		return
	}
	name = strings.ToLower(name)
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return
	}
	d.mu.Lock()
	if parsed.To4() != nil {
		d.entries[name] = ip
	} else {
		d.entriesV6[name] = ip
	}
	d.reverse[ip] = name
	d.mu.Unlock()
}

// RemoveEntry removes a name -> ip mapping.
func (d *DNSServer) RemoveEntry(name string) {
	d.mu.Lock()
	name = strings.ToLower(name)
	if ip, ok := d.entries[name]; ok {
		delete(d.reverse, ip)
		delete(d.entries, name)
	}
	if ip, ok := d.entriesV6[name]; ok {
		delete(d.reverse, ip)
		delete(d.entriesV6, name)
	}
	d.mu.Unlock()
}

// Resolve returns the locally-registered IPv4 for name, or empty string + false.
func (d *DNSServer) Resolve(name string) (string, bool) {
	d.mu.RLock()
	ip, ok := d.entries[strings.ToLower(name)]
	d.mu.RUnlock()
	return ip, ok
}

// ResolveAAAA returns the locally-registered IPv6 for name, or empty string + false.
func (d *DNSServer) ResolveAAAA(name string) (string, bool) {
	d.mu.RLock()
	ip, ok := d.entriesV6[strings.ToLower(name)]
	d.mu.RUnlock()
	return ip, ok
}

// ResolvePTR returns the name for a given IP address (reverse lookup).
// IP can be in standard form (e.g. "172.17.0.2" or "::1").
func (d *DNSServer) ResolvePTR(ip string) (string, bool) {
	d.mu.RLock()
	name, ok := d.reverse[ip]
	d.mu.RUnlock()
	return name, ok
}

// EntryCount returns the number of registered DNS entries.
func (d *DNSServer) EntryCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entries) + len(d.entriesV6)
}

// Addr returns the actual bound address (e.g. "127.0.0.11:53").
// Empty string if not yet started.
func (d *DNSServer) Addr() string {
	return d.addr
}

// CacheCapacity returns the number of cached entries.
func (d *DNSServer) CacheCapacity() int { return d.cache.len() }

// Stats returns a snapshot of metrics.
func (d *DNSServer) Stats() map[string]uint64 {
	return map[string]uint64{
		"queries":     d.queriesServed.Load(),
		"cache_hits":  d.cacheHits.Load(),
		"cache_miss":  d.cacheMisses.Load(),
		"forwards":    d.forwards.Load(),
		"forward_err": d.forwardFails.Load(),
		"tcp_queries": d.tcpQueries.Load(),
		"cache_size":  uint64(d.cache.len()),
		"entries":     uint64(d.EntryCount()),
	}
}

// Upstreams returns the configured upstream servers.
func (d *DNSServer) Upstreams() []string {
	return d.upstream
}

// Start binds the UDP and TCP sockets and begins serving.
// addr can be "127.0.0.11:53" or "127.0.0.1:0" for ephemeral.
func (d *DNSServer) Start(addr string) error {
	if !d.started.CompareAndSwap(false, true) {
		return errors.New("dns server: already started")
	}
	if addr == "" {
		addr = DNSWellKnownAddress()
	}

	// UDP listener.
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		d.started.Store(false)
		return fmt.Errorf("dns resolve udp %s: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		d.started.Store(false)
		return fmt.Errorf("dns listen udp %s: %w", udpAddr, err)
	}
	d.listener = conn
	d.addr = conn.LocalAddr().String()

	// TCP listener (RFC 5966: required for responses > 512 bytes).
	tcpAddr, err := net.ResolveTCPAddr("tcp", d.addr)
	if err == nil {
		d.tcpListener, err = net.ListenTCP("tcp", tcpAddr)
		if err != nil {
			d.log.Warn("dns tcp listen failed (non-fatal)", "err", err)
		}
	}

	d.wg.Add(1)
	go d.serve()

	if d.tcpListener != nil {
		d.wg.Add(1)
		go d.serveTCP()
	}

	d.log.Info("dns server started",
		"addr", d.addr,
		"tcp", d.tcpListener != nil,
		"upstream", d.upstream,
		"cache_capacity", d.cache.cap,
	)
	return nil
}

// Stop signals the serve goroutines to exit and closes sockets.
// Safe to call multiple times.
func (d *DNSServer) Stop() {
	if !d.started.Load() {
		return
	}
	select {
	case <-d.stopCh:
	default:
		close(d.stopCh)
	}
	if d.listener != nil {
		_ = d.listener.Close()
	}
	if d.tcpListener != nil {
		_ = d.tcpListener.Close()
	}
	d.wg.Wait()
	d.log.Info("dns server stopped",
		"queries", d.queriesServed.Load(),
		"cache_hits", d.cacheHits.Load(),
		"forwards", d.forwards.Load(),
	)
}

// serve is the main UDP read loop.
func (d *DNSServer) serve() {
	defer d.wg.Done()
	buf := make([]byte, 4096) // EDNS-safe buffer

	// Use a goroutine to signal stop via closing the connection.
	go func() {
		<-d.stopCh
		if d.listener != nil {
			d.listener.Close()
		}
	}()

	for {
		n, clientAddr, err := d.listener.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			d.log.Warn("dns udp read", "err", err)
			continue
		}
		req := make([]byte, n)
		copy(req, buf[:n])
		go d.handleQuery(req, clientAddr)
	}
}

// serveTCP is the TCP listener loop (RFC 5966).
func (d *DNSServer) serveTCP() {
	defer d.wg.Done()
	for {
		select {
		case <-d.stopCh:
			return
		default:
		}
		conn, err := d.tcpListener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go d.handleTCPConn(conn)
	}
}

// handleTCPConn processes a single TCP DNS query.
func (d *DNSServer) handleTCPConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	d.tcpQueries.Add(1)

	// TCP DNS: 2-byte length prefix + DNS message.
	var lenBuf [2]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return
	}
	msgLen := binary.BigEndian.Uint16(lenBuf[:])
	if msgLen > 4096 {
		return
	}
	query := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, query); err != nil {
		return
	}

	// Process same as UDP.
	name, qtype, qEnd, err := parseDNSQuestion(query)
	if err != nil {
		return
	}

	var resp []byte
	if ip, ok := d.Resolve(name); ok && qtype == dnsTypeA {
		resp = buildAResponse(query, qEnd, name, net.ParseIP(ip).To4(), d.positiveTTL)
	} else if cached, _, found := d.cache.get(name, qtype); found {
		resp = cached
	} else {
		resp, err = d.forward(query)
		if err != nil {
			d.forwardFails.Add(1)
			resp = buildServFail(query, qEnd, name, qtype)
		} else {
			d.forwards.Add(1)
			isSuccess := isNoError(resp)
			ttl := d.positiveTTL
			if !isSuccess {
				ttl = d.negativeTTL
			}
			d.cache.set(name, qtype, resp, ttl)
		}
	}

	// Send with length prefix.
	var respLen [2]byte
	binary.BigEndian.PutUint16(respLen[:], uint16(len(resp)))
	_, _ = conn.Write(respLen[:])
	_, _ = conn.Write(resp)
}

// handleQuery processes a single UDP DNS query.
func (d *DNSServer) handleQuery(query []byte, clientAddr *net.UDPAddr) {
	d.queriesServed.Add(1)

	name, qtype, qEnd, err := parseDNSQuestion(query)
	if err != nil {
		d.log.Debug("dns parse", "err", err)
		return
	}

	// 1. Local registry hit.
	switch qtype {
	case dnsTypeA:
		if ip, ok := d.Resolve(name); ok {
			resp := buildAResponse(query, qEnd, name, net.ParseIP(ip).To4(), d.positiveTTL)
			d.writeResponse(resp, clientAddr)
			return
		}
	case dnsTypeAAAA:
		if ip, ok := d.ResolveAAAA(name); ok {
			resp := buildAAAAResponse(query, qEnd, name, net.ParseIP(ip), d.positiveTTL)
			d.writeResponse(resp, clientAddr)
			return
		}
	case dnsTypePTR:
		// Convert ARPA name to IP address.
		if ip := arpaToIP(name); ip != "" {
			if ptrName, ok := d.ResolvePTR(ip); ok {
				resp := buildPTRResponse(query, qEnd, name, ptrName, d.positiveTTL)
				d.writeResponse(resp, clientAddr)
				return
			}
		}
	}

	// 2. Cache hit (positive or negative, any record type).
	if cached, _, found := d.cache.get(name, qtype); found {
		d.cacheHits.Add(1)
		d.writeResponse(cached, clientAddr)
		return
	}
	d.cacheMisses.Add(1)

	// 3. Forward to upstream (with failover).
	resp, err := d.forward(query)
	if err != nil {
		d.forwardFails.Add(1)
		d.log.Debug("dns forward failed", "name", name, "qtype", qtype, "err", err)
		resp = buildServFail(query, qEnd, name, qtype)
	} else {
		d.forwards.Add(1)
		// Cache the raw response.
		isSuccess := isNoError(resp)
		ttl := d.positiveTTL
		if !isSuccess {
			ttl = d.negativeTTL
		}
		d.cache.set(name, qtype, resp, ttl)
	}
	d.writeResponse(resp, clientAddr)
}

func (d *DNSServer) writeResponse(resp []byte, clientAddr *net.UDPAddr) {
	if d.listener == nil {
		return
	}
	if _, err := d.listener.WriteToUDP(resp, clientAddr); err != nil {
		d.log.Debug("dns udp write", "err", err)
	}
}

// forward sends a query to upstream servers with failover.
// Tries each upstream in order until one responds.
// If a UDP response has the TC bit set, retries with TCP (RFC 5966).
func (d *DNSServer) forward(query []byte) ([]byte, error) {
	var lastErr error
	for _, upstream := range d.upstream {
		// Try UDP first.
		resp, err := d.tryUpstream("udp", upstream, query)
		if err != nil {
			lastErr = err
			continue
		}
		// Check TC bit — if set, retry with TCP.
		if len(resp) > 2 && (resp[2]&0x02) != 0 {
			tcpResp, err := d.tryUpstreamTCP(upstream, query)
			if err != nil {
				lastErr = err
				continue
			}
			return tcpResp, nil
		}
		return resp, nil
	}
	return nil, fmt.Errorf("all %d upstreams failed: %w", len(d.upstream), lastErr)
}

func (d *DNSServer) tryUpstream(network, upstream string, query []byte) ([]byte, error) {
	client, err := net.DialTimeout(network, upstream, d.queryTimeout)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(d.queryTimeout))
	if _, err := client.Write(query); err != nil {
		return nil, err
	}
	resp := make([]byte, 4096)
	n, err := client.Read(resp)
	if err != nil {
		return nil, err
	}
	return resp[:n], nil
}

func (d *DNSServer) tryUpstreamTCP(upstream string, query []byte) ([]byte, error) {
	client, err := net.DialTimeout("tcp", upstream, d.queryTimeout)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(d.queryTimeout))
	// TCP DNS: 2-byte length prefix (RFC 1035 §4.2.2).
	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(query)))
	if _, err := client.Write(length[:]); err != nil {
		return nil, err
	}
	if _, err := client.Write(query); err != nil {
		return nil, err
	}
	// Read 2-byte response length.
	if _, err := io.ReadFull(client, length[:]); err != nil {
		return nil, err
	}
	respLen := int(binary.BigEndian.Uint16(length[:]))
	if respLen > 65535 {
		return nil, fmt.Errorf("dns tcp response too large: %d", respLen)
	}
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(client, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ─── DNS wire helpers ──────────────────────────────────────────────

const (
	dnsTypeA    uint16 = 1
	dnsTypeAAAA uint16 = 28
	dnsTypeCNAME uint16 = 5
	dnsTypeTXT  uint16 = 16
	dnsTypeSRV  uint16 = 33
	dnsTypePTR  uint16 = 12
	dnsClassIN  uint16 = 1

	dnsFlagResponse  uint16 = 0x8000
	dnsFlagRD        uint16 = 0x0100
	dnsRcodeNoError  uint16 = 0
	dnsRcodeServFail uint16 = 2
	dnsRcodeNXDOMAIN uint16 = 3
)

// parseDNSQuestion returns the QNAME (lowercased), QTYPE, and the index right
// after the QCLASS field in the original query (useful for response building).
func parseDNSQuestion(msg []byte) (name string, qtype uint16, qEnd int, err error) {
	if len(msg) < 12 {
		return "", 0, 0, errors.New("dns: header too short")
	}
	// Skip header.
	off := 12
	name, off, err = parseDNSName(msg, off)
	if err != nil {
		return "", 0, 0, err
	}
	if off+4 > len(msg) {
		return "", 0, 0, errors.New("dns: question truncated")
	}
	qtype = binary.BigEndian.Uint16(msg[off : off+2])
	off += 4 // QTYPE + QCLASS
	return name, qtype, off, nil
}

// parseDNSName walks the QNAME starting at off, handling labels and
// compression pointers. Returns the lowercased name and the index of the
// byte right after the name.
func parseDNSName(msg []byte, off int) (string, int, error) {
	var b strings.Builder
	first := true
	const maxJumps = 5
	jumps := 0
	cur := off
	followed := false
	originalOff := off
	for {
		if cur >= len(msg) {
			return "", 0, errors.New("dns: name out of bounds")
		}
		l := int(msg[cur])
		if l == 0 {
			cur++
			break
		}
		if l&0xC0 == 0xC0 {
			// Compression pointer.
			if cur+1 >= len(msg) {
				return "", 0, errors.New("dns: pointer truncated")
			}
			ptr := int(binary.BigEndian.Uint16(msg[cur:cur+2]) & 0x3FFF)
			if !followed {
				originalOff = cur + 2
			}
			cur = ptr
			followed = true
			jumps++
			if jumps > maxJumps {
				return "", 0, errors.New("dns: pointer loop")
			}
			continue
		}
		cur++
		if cur+l > len(msg) {
			return "", 0, errors.New("dns: label out of bounds")
		}
		if !first {
			b.WriteByte('.')
		}
		first = false
		b.Write(msg[cur : cur+l])
		cur += l
	}
	if followed {
		return strings.ToLower(b.String()), originalOff, nil
	}
	return strings.ToLower(b.String()), cur, nil
}

func buildDNSHeader(query []byte, rcode uint16) []byte {
	hdr := make([]byte, 12)
	copy(hdr, query[:2]) // Transaction ID
	flags := dnsFlagResponse | dnsFlagRD
	if rcode != 0 {
		flags |= rcode & 0x000F
	}
	binary.BigEndian.PutUint16(hdr[2:4], flags)
	binary.BigEndian.PutUint16(hdr[4:6], 1)  // QDCOUNT
	binary.BigEndian.PutUint16(hdr[6:8], 1)  // ANCOUNT
	binary.BigEndian.PutUint16(hdr[8:10], 0) // NSCOUNT
	binary.BigEndian.PutUint16(hdr[10:12], 0)
	return hdr
}

func buildAResponse(query []byte, qEnd int, name string, ip net.IP, ttl time.Duration) []byte {
	hdr := buildDNSHeader(query, dnsRcodeNoError)
	resp := make([]byte, 0, 512)
	resp = append(resp, hdr...)
	resp = append(resp, query[12:qEnd]...) // question section
	// Answer.
	resp = appendName(resp, name)
	resp = append(resp, 0x00, 0x01)             // TYPE A
	resp = append(resp, 0x00, 0x01)             // CLASS IN
	resp = appendUint32(resp, uint32(ttl.Seconds())) // TTL
	resp = append(resp, 0x00, 0x04)             // RDLENGTH
	resp = append(resp, ip.To4()...)
	return resp
}

func buildAAAAResponse(query []byte, qEnd int, name string, ip net.IP, ttl time.Duration) []byte {
	hdr := buildDNSHeader(query, dnsRcodeNoError)
	resp := make([]byte, 0, 512)
	resp = append(resp, hdr...)
	resp = append(resp, query[12:qEnd]...)
	resp = appendName(resp, name)
	resp = append(resp, 0x00, 0x1c)             // TYPE AAAA
	resp = append(resp, 0x00, 0x01)             // CLASS IN
	resp = appendUint32(resp, uint32(ttl.Seconds()))
	resp = append(resp, 0x00, 0x10)             // RDLENGTH = 16
	resp = append(resp, ip.To16()...)
	return resp
}

func buildPTRResponse(query []byte, qEnd int, arpaName, ptrName string, ttl time.Duration) []byte {
	hdr := buildDNSHeader(query, dnsRcodeNoError)
	resp := make([]byte, 0, 512)
	resp = append(resp, hdr...)
	resp = append(resp, query[12:qEnd]...)
	resp = appendName(resp, arpaName)
	resp = append(resp, 0x00, 0x0c)             // TYPE PTR
	resp = append(resp, 0x00, 0x01)             // CLASS IN
	resp = appendUint32(resp, uint32(ttl.Seconds()))
	// RDLENGTH + target name.
	ptrWire := nameToWire(ptrName)
	resp = appendUint16(resp, uint16(len(ptrWire)))
	resp = append(resp, ptrWire...)
	return resp
}

func nameToWire(name string) []byte {
	var buf []byte
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			continue
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0)
	return buf
}

func buildServFail(query []byte, qEnd int, name string, qtype uint16) []byte {
	hdr := buildDNSHeader(query, dnsRcodeServFail)
	resp := make([]byte, 0, 512)
	resp = append(resp, hdr...)
	resp = append(resp, query[12:qEnd]...)
	return resp
}

func isNoError(resp []byte) bool {
	if len(resp) < 4 {
		return false
	}
	return (binary.BigEndian.Uint16(resp[2:4]) & 0x000F) == dnsRcodeNoError
}

func appendName(buf []byte, name string) []byte {
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			continue
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	return append(buf, 0)
}

func appendUint32(buf []byte, v uint32) []byte {
	return append(buf,
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v),
	)
}

func appendUint16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

// arpaToIP converts a reverse lookup DNS name (e.g. "2.0.17.172.in-addr.arpa")
// to a standard IP address string. Returns "" on parse failure.
func arpaToIP(arpa string) string {
	arpa = strings.TrimSuffix(arpa, ".in-addr.arpa")
	arpa = strings.TrimSuffix(arpa, ".ip6.arpa")
	parts := strings.Split(arpa, ".")
	if len(parts) == 4 {
		// IPv4: "2.0.17.172" -> "172.17.0.2"
		ip := net.ParseIP(parts[3] + "." + parts[2] + "." + parts[1] + "." + parts[0])
		if ip != nil {
			return ip.String()
		}
	}
	return ""
}

// ─── LRU cache for DNS responses ──────────────────────────────────

type dnsCacheEntry struct {
	key   string
	value []byte
	exp   time.Time
	elem  *list.Element
}

type dnsCache struct {
	mu       sync.RWMutex
	cap      int
	defaultT time.Duration
	ll       *list.List
	items    map[string]*dnsCacheEntry
	hits     atomic.Uint64
	misses   atomic.Uint64
}

func newDNSCache(capacity int, defaultTTL time.Duration) *dnsCache {
	return &dnsCache{
		cap:      capacity,
		defaultT: defaultTTL,
		ll:       list.New(),
		items:    make(map[string]*dnsCacheEntry, capacity),
	}
}

func (c *dnsCache) cacheKey(name string, qtype uint16) string {
	return fmt.Sprintf("%s|%d", name, qtype)
}

func (c *dnsCache) get(name string, qtype uint16) ([]byte, time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := c.cacheKey(name, qtype)
	e, ok := c.items[k]
	if !ok {
		c.misses.Add(1)
		return nil, 0, false
	}
	if time.Now().After(e.exp) {
		c.ll.Remove(e.elem)
		delete(c.items, k)
		c.misses.Add(1)
		return nil, 0, false
	}
	c.ll.MoveToFront(e.elem)
	c.hits.Add(1)
	out := make([]byte, len(e.value))
	copy(out, e.value)
	return out, time.Until(e.exp), true
}

func (c *dnsCache) set(name string, qtype uint16, value []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := c.cacheKey(name, qtype)
	if ttl <= 0 {
		ttl = c.defaultT
	}
	if e, ok := c.items[k]; ok {
		e.value = value
		e.exp = time.Now().Add(ttl)
		c.ll.MoveToFront(e.elem)
		return
	}
	val := make([]byte, len(value))
	copy(val, value)
	e := &dnsCacheEntry{key: k, value: val, exp: time.Now().Add(ttl)}
	e.elem = c.ll.PushFront(e)
	c.items[k] = e
	if c.ll.Len() > c.cap {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.items, oldest.Value.(*dnsCacheEntry).key)
		}
	}
}

func (c *dnsCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ll.Len()
}
