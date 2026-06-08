package netlink

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// UDPProxy is a stateless UDP packet forwarder plus a small session
// tracker for return traffic. Each incoming datagram from the host side
// is forwarded to the upstream address, and each datagram from the
// upstream is forwarded back to the most-recent sender for the matching
// session.
//
// Unlike TCP, UDP is connectionless: we do not maintain a 1:1 proxy
// connection. Instead, we track sessions by (hostAddr, upstreamAddr)
// pair and time them out after sessionTimeout of inactivity.
type UDPProxy struct {
	listenAddr string
	upstream   string

	conn *net.UDPConn

	closed atomic.Bool
	wrap   TransportWrapper

	sessionTimeout time.Duration
	maxSessions    int
	readBufSize    int

	sessionsMu sync.RWMutex
	sessions   map[string]*udpSession

	mu       sync.Mutex
	upstreamConn *net.UDPConn

	log *slog.Logger
}

type udpSession struct {
	lastSeen time.Time
	peer    *net.UDPAddr
}

// UDPProxyConfig configures a UDPProxy.
type UDPProxyConfig struct {
	ListenAddr     string
	Upstream       string
	Wrap           TransportWrapper
	SessionTimeout time.Duration // default 30s
	MaxSessions    int           // default 1024
	ReadBufSize    int           // default 65535
	Logger         *slog.Logger
}

// NewUDPProxy creates a UDPProxy in the "configured but not listening"
// state. Call Start to bind the socket and begin forwarding.
func NewUDPProxy(cfg UDPProxyConfig) *UDPProxy {
	if cfg.SessionTimeout <= 0 {
		cfg.SessionTimeout = 30 * time.Second
	}
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = 1024
	}
	if cfg.ReadBufSize <= 0 {
		cfg.ReadBufSize = 65535
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	wrap := cfg.Wrap
	if wrap == nil {
		wrap = noopWrapper{}
	}
	return &UDPProxy{
		listenAddr:     cfg.ListenAddr,
		upstream:       cfg.Upstream,
		wrap:           wrap,
		sessionTimeout: cfg.SessionTimeout,
		maxSessions:    cfg.MaxSessions,
		readBufSize:    cfg.ReadBufSize,
		sessions:       make(map[string]*udpSession),
		log:            logger,
	}
}

// Start binds the UDP socket and spawns the forwarder goroutines.
func (p *UDPProxy) Start(ctx context.Context) error {
	if p.closed.Load() {
		return errors.New("netlink: UDPProxy already closed")
	}
	addr, err := net.ResolveUDPAddr("udp", p.listenAddr)
	if err != nil {
		return fmt.Errorf("netlink: resolve %s: %w", p.listenAddr, err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("netlink: listen udp %s: %w", p.listenAddr, err)
	}
	p.conn = conn

	upAddr, err := net.ResolveUDPAddr("udp", p.upstream)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("netlink: resolve upstream %s: %w", p.upstream, err)
	}
	upConn, err := net.DialUDP("udp", nil, upAddr)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("netlink: dial upstream %s: %w", p.upstream, err)
	}
	p.mu.Lock()
	p.upstreamConn = upConn
	p.mu.Unlock()

	go p.readLoop(ctx)
	go p.upstreamReadLoop(ctx)
	go p.sessionJanitor(ctx)
	return nil
}

// Addr returns the bound listen address.
func (p *UDPProxy) Addr() string {
	if p.conn == nil {
		return ""
	}
	return p.conn.LocalAddr().String()
}

// Close stops forwarding and closes all sockets. Safe to call multiple
// times; subsequent calls are no-ops.
func (p *UDPProxy) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.mu.Lock()
	if p.upstreamConn != nil {
		_ = p.upstreamConn.Close()
	}
	p.mu.Unlock()
	return nil
}

// readLoop reads datagrams from the host-facing socket and forwards them
// upstream. Each packet also creates/refreshes a session entry so the
// return traffic can be routed back to the original sender.
func (p *UDPProxy) readLoop(ctx context.Context) {
	buf := make([]byte, p.readBufSize)
	for {
		if p.closed.Load() {
			return
		}
		_ = p.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, peer, err := p.conn.ReadFromUDP(buf)
		if err != nil {
			if p.closed.Load() {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			p.log.Warn("doki-link: udp read error", "err", err)
			continue
		}

		p.upsertSession(peer, fmt.Sprintf("client:%s", peer.String()))

		// Wrap if requested. UDP wrapping is uncommon; only wrap the
		// payload (we keep the UDP envelope intact on the wire).
		payload := buf[:n]
		if w, ok := p.wrap.(udpWrapper); ok {
			encoded, err := w.WrapOutbound(payload)
			if err != nil {
				p.log.Warn("doki-link: udp wrap outbound failed", "err", err)
				continue
			}
			payload = encoded
		}

		p.mu.Lock()
		up := p.upstreamConn
		p.mu.Unlock()
		if up == nil {
			continue
		}
		if _, err := up.Write(payload); err != nil {
			p.log.Warn("doki-link: udp write upstream failed", "err", err)
		}
	}
}

// upstreamReadLoop reads datagrams coming back from upstream and routes
// them to the most-recent sender for the session.
func (p *UDPProxy) upstreamReadLoop(_ context.Context) {
	buf := make([]byte, p.readBufSize)
	for {
		if p.closed.Load() {
			return
		}
		p.mu.Lock()
		up := p.upstreamConn
		p.mu.Unlock()
		if up == nil {
			return
		}
		_ = up.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _, err := up.ReadFromUDP(buf)
		if err != nil {
			if p.closed.Load() {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			p.log.Warn("doki-link: udp upstream read error", "err", err)
			continue
		}

		payload := buf[:n]
		if w, ok := p.wrap.(udpWrapper); ok {
			decoded, err := w.UnwrapInbound(payload)
			if err != nil {
				p.log.Warn("doki-link: udp unwrap inbound failed", "err", err)
				continue
			}
			payload = decoded
		}

		// Route to the most-recent sender we saw. Single-upstream
		// simplification: broadcast back to all known peers. This
		// matches socat's UDP-LISTEN,fork behavior for typical
		// request/response services.
		p.sessionsMu.RLock()
		peers := make([]*net.UDPAddr, 0, len(p.sessions))
		for _, s := range p.sessions {
			peers = append(peers, s.peer)
		}
		p.sessionsMu.RUnlock()
		for _, peer := range peers {
			if _, err := p.conn.WriteToUDP(payload, peer); err != nil {
				p.log.Warn("doki-link: udp write peer failed",
					"peer", peer, "err", err)
			}
		}
	}
}

// sessionJanitor evicts sessions that have not been seen for
// sessionTimeout. It runs until Close is called.
func (p *UDPProxy) sessionJanitor(_ context.Context) {
	ticker := time.NewTicker(p.sessionTimeout / 2)
	defer ticker.Stop()
	for {
		if p.closed.Load() {
			return
		}
		<-ticker.C
		cutoff := time.Now().Add(-p.sessionTimeout)
		p.sessionsMu.Lock()
		for k, s := range p.sessions {
			if s.lastSeen.Before(cutoff) {
				delete(p.sessions, k)
			}
		}
		p.sessionsMu.Unlock()
	}
}

func (p *UDPProxy) upsertSession(peer *net.UDPAddr, key string) {
	p.sessionsMu.Lock()
	defer p.sessionsMu.Unlock()
	if len(p.sessions) >= p.maxSessions {
		// Evict the oldest entry to make room.
		var oldestKey string
		var oldestTime time.Time
		for k, s := range p.sessions {
			if oldestKey == "" || s.lastSeen.Before(oldestTime) {
				oldestKey = k
				oldestTime = s.lastSeen
			}
		}
		if oldestKey != "" {
			delete(p.sessions, oldestKey)
		}
	}
	p.sessions[key] = &udpSession{
		lastSeen: time.Now(),
		peer:     peer,
	}
}

// udpWrapper is an optional interface for TransportWrappers that also
// support per-datagram encryption (e.g. secretbox). The default
// noopWrapper does NOT implement it, so UDP forwarding is unencrypted
// unless a crypto wrapper is plugged in.
type udpWrapper interface {
	WrapOutbound(payload []byte) ([]byte, error)
	UnwrapInbound(payload []byte) ([]byte, error)
}
