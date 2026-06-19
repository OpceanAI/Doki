// Package netlink implements DokiLink-Lite: a pure-Go network proxy and
// mesh layer that replaces external tools (socat, slirp4netns, pasta) for
// container port forwarding in environments where they are not available
// (notably Termux on Android, which lacks network namespaces and iptables).
//
// DokiLink-Lite is intentionally minimal:
//
//   - TCPProxy / UDPProxy: bidirectional byte-stream proxies using only
//     the Go standard library. No gVisor, no WireGuard, no netstack.
//   - Optional TLS 1.3 wrapping (see crypto.go).
//   - Optional NaCl secretbox payload encryption (see crypto.go).
//   - Optional LAN-only mesh via mDNS or static peer JSON (see mesh.go).
//
// The package is designed to be safe to import on every platform: when
// the caller does not invoke any of the proxy/mesh entry points, no
// network resources are opened.
package netlink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TCPProxy is a TCP byte-stream proxy. It listens on a host address and,
// for each accepted client connection, dials the upstream (container)
// address and pumps bytes in both directions until either side closes.
type TCPProxy struct {
	listenAddr string
	upstream   string

	listener net.Listener
	closed   atomic.Bool
	connsMu  sync.Mutex
	conns    map[net.Conn]struct{}

	// wrap is an optional transport wrapper (TLS, secretbox, ...).
	// If non-nil, it is applied to both ends of every connection.
	// See crypto.go for the canonical implementation.
	wrap TransportWrapper

	// idleTimeout closes connections that have been idle for this long.
	// Zero means no timeout.
	idleTimeout time.Duration

	// log is the structured logger. Defaults to slog.Default().
	log *slog.Logger
}

// TransportWrapper optionally wraps a net.Conn to add encryption or
// authentication. It must be safe to call from multiple goroutines.
type TransportWrapper interface {
	// WrapServer wraps the server-side (host) of a connection. If the
	// implementation performs a handshake (e.g. TLS), it MUST respect
	// ctx.Done().
	WrapServer(ctx context.Context, c net.Conn) (net.Conn, error)
	// WrapClient wraps the client-side (container/upstream) of a connection.
	WrapClient(ctx context.Context, c net.Conn) (net.Conn, error)
}

// noopWrapper is the default TransportWrapper; it returns the conn unchanged.
type noopWrapper struct{}

func (noopWrapper) WrapServer(_ context.Context, c net.Conn) (net.Conn, error) {
	return c, nil
}
func (noopWrapper) WrapClient(_ context.Context, c net.Conn) (net.Conn, error) {
	return c, nil
}

// TCPProxyConfig configures a TCPProxy.
type TCPProxyConfig struct {
	ListenAddr  string            // e.g. "0.0.0.0:8888" or "127.0.0.1:0"
	Upstream    string            // e.g. "127.0.0.1:80"
	Wrap        TransportWrapper  // optional
	IdleTimeout time.Duration     // 0 = no timeout
	Logger      *slog.Logger      // nil = slog.Default()
}

// NewTCPProxy creates a TCPProxy in the "configured but not listening"
// state. Call Start to bind the listener and begin accepting connections.
func NewTCPProxy(cfg TCPProxyConfig) *TCPProxy {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	wrap := cfg.Wrap
	if wrap == nil {
		wrap = noopWrapper{}
	}
	return &TCPProxy{
		listenAddr:  cfg.ListenAddr,
		upstream:    cfg.Upstream,
		wrap:        wrap,
		idleTimeout: cfg.IdleTimeout,
		conns:       make(map[net.Conn]struct{}),
		log:         logger,
	}
}

// Start binds the listener and spawns the accept loop. Returns an error
// if the listen address is invalid or already in use.
func (p *TCPProxy) Start(ctx context.Context) error {
	if p.closed.Load() {
		return errors.New("netlink: TCPProxy already closed")
	}
	ln, err := net.Listen("tcp", p.listenAddr)
	if err != nil {
		return fmt.Errorf("netlink: listen %s: %w", p.listenAddr, err)
	}
	p.listener = ln
	go p.acceptLoop(ctx)
	return nil
}

// Addr returns the bound listen address. Useful when ListenAddr contained
// port 0 (OS-assigned). Returns empty string if Start has not succeeded.
func (p *TCPProxy) Addr() string {
	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

// Close stops accepting new connections and terminates all active
// connections. Safe to call multiple times; subsequent calls are no-ops.
func (p *TCPProxy) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
	if p.listener != nil {
		_ = p.listener.Close()
	}
	p.connsMu.Lock()
	for c := range p.conns {
		_ = c.Close()
	}
	p.connsMu.Unlock()
	return nil
}

func (p *TCPProxy) acceptLoop(ctx context.Context) {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			if p.closed.Load() {
				return
			}
			// Transient accept error; brief backoff to avoid hot loop.
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
				continue
			}
		}
		p.trackConn(client)
		go p.handleConn(ctx, client)
	}
}

func (p *TCPProxy) trackConn(c net.Conn) {
	p.connsMu.Lock()
	p.conns[c] = struct{}{}
	p.connsMu.Unlock()
}

func (p *TCPProxy) untrackConn(c net.Conn) {
	p.connsMu.Lock()
	delete(p.conns, c)
	p.connsMu.Unlock()
}

func (p *TCPProxy) handleConn(ctx context.Context, client net.Conn) {
	defer p.untrackConn(client)
	defer func() { _ = client.Close() }()

	if p.idleTimeout > 0 {
		_ = client.SetDeadline(time.Now().Add(p.idleTimeout))
	}

	upstream, err := net.Dial("tcp", p.upstream)
	if err != nil {
		p.log.Warn("doki-link: upstream dial failed",
			"upstream", p.upstream, "err", err)
		return
	}
	defer func() { _ = upstream.Close() }()

	if p.idleTimeout > 0 {
		_ = upstream.SetDeadline(time.Now().Add(p.idleTimeout))
	}

	serverSide, err := p.wrap.WrapServer(ctx, client)
	if err != nil {
		p.log.Warn("doki-link: wrap server failed", "err", err)
		return
	}
	clientSide, err := p.wrap.WrapClient(ctx, upstream)
	if err != nil {
		p.log.Warn("doki-link: wrap client failed", "err", err)
		return
	}

	// Bidirectional pump. Wait for both halves so we close both ends.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientSide, serverSide)
		// Half-close: signal the other side that no more writes are coming.
		if cw, ok := clientSide.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(serverSide, clientSide)
		if cw, ok := serverSide.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
	}()
	wg.Wait()
}

// closeWriter is implemented by *net.TCPConn on POSIX-like systems and
// by *crypto/tls.Conn. It allows a graceful half-close.
type closeWriter interface {
	CloseWrite() error
}
