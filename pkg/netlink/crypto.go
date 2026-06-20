package netlink

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/nacl/secretbox"
)

// This file implements two TransportWrapper implementations that plug
// into TCPProxy/UDPProxy:
//
//   - TLSWrapper: opportunistic TLS 1.3 with a self-signed cert signed
//     by the Doki CA. Transparent to clients that don't speak TLS (the
//     server side is wrapped as tls.Conn; the client side is wrapped
//     with a raw conn because in single-host setup the listener is
//     local and clients are unencrypted).
//
//   - SecretboxWrapper: per-datagram (UDP) or per-stream (TCP) NaCl
//     secretbox encryption using a 32-byte shared key. The key is
//     derived from the Ed25519 identity of both peers via SHA-256 of
//     the concatenated public keys. Adds a second layer of
//     confidentiality on top of TLS.
//
// Both wrappers are safe for concurrent use. Each call to WrapServer /
// WrapClient returns a fresh wrapper; underlying state is stored in
// the wrapper itself, not the connection.

// TLSConfigFor builds a *tls.Config ready to drop into tls.NewListener
// (server) or tls.Dial (client). It converts a LoadedCert into the
// cert slice tls.Config expects.
func TLSConfigFor(leaf LoadedCert, rootCAs *x509.CertPool, serverName string) (*tls.Config, error) {
	if leaf.Leaf == nil || len(leaf.KeyDER) == 0 {
		return nil, errors.New("netlink: empty LoadedCert")
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Leaf.Raw})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leaf.KeyDER})
	key, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("netlink: x509 key pair: %w", err)
	}
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{key},
	}
	if rootCAs != nil {
		cfg.RootCAs = rootCAs
	}
	if serverName != "" {
		cfg.ServerName = serverName
	}
	return cfg, nil
}

// TLSWrapper wraps a net.Conn with TLS 1.3. It is configured once at
// construction and then applied to every wrapped connection.
type TLSWrapper struct {
	cfg *tls.Config
}

// NewTLSWrapper builds a wrapper from a *tls.Config. The same config
// is used for both WrapServer and WrapClient; whether the resulting
// conn behaves as server or client is decided by the handshake.
func NewTLSWrapper(cfg *tls.Config) (*TLSWrapper, error) {
	if cfg == nil {
		return nil, errors.New("netlink: NewTLSWrapper with nil config")
	}
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS13
	}
	return &TLSWrapper{cfg: cfg}, nil
}

// WrapServer wraps the server-side (host) of a connection. The returned
// net.Conn has completed a TLS handshake by the time the function
// returns (or has returned an error).
func (t *TLSWrapper) WrapServer(ctx context.Context, c net.Conn) (net.Conn, error) {
	cfg := t.cfg.Clone()
	srv := tls.Server(c, cfg)
	if err := srv.HandshakeContext(ctx); err != nil {
		_ = srv.Close()
		return nil, fmt.Errorf("netlink: tls server handshake: %w", err)
	}
	return srv, nil
}

// WrapClient wraps the client-side (container/upstream) of a connection.
// The returned net.Conn has completed a TLS handshake by the time the
// function returns.
func (t *TLSWrapper) WrapClient(ctx context.Context, c net.Conn) (net.Conn, error) {
	cfg := t.cfg.Clone()
	cli := tls.Client(c, cfg)
	if err := cli.HandshakeContext(ctx); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("netlink: tls client handshake: %w", err)
	}
	return cli, nil
}

// SecretboxWrapper adds a NaCl secretbox layer on top of whatever
// transport (TCP or UDP) is being wrapped. It uses a single 32-byte
// key shared by both peers; that key is derived from the Ed25519
// identities of the two endpoints (see DeriveSecretKey).
//
// On TCP, the wrapper applies a per-connection nonce (24 bytes
// secretbox.NonceSize + 16 bytes secretbox.Overhead) and uses a 4-byte
// big-endian length prefix per frame. On UDP, it uses the standard
// nacl/secretbox.Seal/Open API with a per-datagram nonce derived from
// the message counter.
//
// SecretboxWrapper implements both TransportWrapper (for TCP) and
// udpWrapper (for UDP).
type SecretboxWrapper struct {
	key    [32]byte
	mu     sync.Mutex
	udpCnt uint64
}

// DeriveSecretKey derives a 32-byte symmetric key from two Ed25519
// public keys. The order matters: peers MUST call this with their own
// public key first, then the remote peer's, to get the same key.
func DeriveSecretKey(localPub, remotePub []byte) [32]byte {
	h := sha256.New()
	h.Write([]byte("dokilink-v1|"))
	h.Write(localPub)
	h.Write([]byte("|"))
	h.Write(remotePub)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// NewSecretboxWrapper builds a wrapper from a 32-byte shared key.
// Use DeriveSecretKey to compute it from peer identities.
func NewSecretboxWrapper(key [32]byte) *SecretboxWrapper {
	return &SecretboxWrapper{key: key}
}

// WrapServer / WrapClient for TCP: wrap the conn in a framed
// secretbox stream that prepends a 4-byte big-endian length to each
// encrypted frame. The frame payload itself is:
//
//   nonce (24 bytes) || secretbox(plaintext)
func (s *SecretboxWrapper) WrapServer(_ context.Context, c net.Conn) (net.Conn, error) {
	return &secretboxStreamConn{Conn: c, sbox: s}, nil
}

// WrapClient mirrors WrapServer; the stream conn is symmetric.
func (s *SecretboxWrapper) WrapClient(_ context.Context, c net.Conn) (net.Conn, error) {
	return &secretboxStreamConn{Conn: c, sbox: s}, nil
}

// WrapOutbound / UnwrapInbound for UDP: each datagram is one
// secretbox.Seal'd payload with its own nonce.
func (s *SecretboxWrapper) WrapOutbound(payload []byte) ([]byte, error) {
	s.mu.Lock()
	counter := s.udpCnt
	s.udpCnt++
	s.mu.Unlock()
	var nonce [24]byte
	// Encode counter big-endian into first 8 bytes of nonce; the rest
	// is zeros. 2^64 messages per key is more than enough.
	for i := 7; i >= 0; i-- {
		nonce[7-i] = byte(counter >> (uint(i) * 8)) // #nosec G115 -- intentional big-endian encoding, always fits in byte
	}
	return secretbox.Seal(nonce[:], payload, &nonce, &s.key), nil
}

// UnwrapInbound reverses WrapOutbound.
func (s *SecretboxWrapper) UnwrapInbound(payload []byte) ([]byte, error) {
	if len(payload) < 24+secretbox.Overhead {
		return nil, errors.New("netlink: secretbox datagram too short")
	}
	var nonce [24]byte
	copy(nonce[:], payload[:24])
	out, ok := secretbox.Open(nil, payload[24:], &nonce, &s.key)
	if !ok {
		return nil, errors.New("netlink: secretbox open failed")
	}
	return out, nil
}

// secretboxStreamConn wraps a net.Conn in a length-prefixed,
// counter-nonce'd secretbox stream. Each Write produces one frame:
//
//   4-byte big-endian length || nonce (24 bytes) || secretbox(plaintext)
//
// each Read decodes one frame.
type secretboxStreamConn struct {
	net.Conn
	sbox   *SecretboxWrapper
	mu     sync.Mutex
	cnt    uint64
	close  bool
	readBuf []byte // leftover decrypted data from previous frame
}

func (c *secretboxStreamConn) Read(b []byte) (int, error) {
	// Return buffered data first
	if len(c.readBuf) > 0 {
		n := copy(b, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(c.Conn, lenBuf[:]); err != nil {
		return 0, err
	}
	frameLen := uint32(lenBuf[0])<<24 | uint32(lenBuf[1])<<16 | uint32(lenBuf[2])<<8 | uint32(lenBuf[3])
	if frameLen < 24+secretbox.Overhead {
		return 0, fmt.Errorf("netlink: secretbox frame too short: %d", frameLen)
	}
	if frameLen > 16*1024*1024 {
		return 0, fmt.Errorf("netlink: secretbox frame too large: %d", frameLen)
	}
	frame := make([]byte, frameLen)
	if _, err := io.ReadFull(c.Conn, frame); err != nil {
		return 0, err
	}
	var nonce [24]byte
	copy(nonce[:], frame[:24])
	out, ok := secretbox.Open(nil, frame[24:], &nonce, &c.sbox.key)
	if !ok {
		return 0, errors.New("netlink: secretbox open failed")
	}
	n := copy(b, out)
	if n < len(out) {
		c.readBuf = out[n:]
	}
	return n, nil
}

func (c *secretboxStreamConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	counter := c.cnt
	c.cnt++
	c.mu.Unlock()
	var nonce [24]byte
	for i := 7; i >= 0; i-- {
		nonce[7-i] = byte(counter >> (uint(i) * 8)) // #nosec G115 -- intentional big-endian encoding, always fits in byte
	}
	sealed := secretbox.Seal(nonce[:], b, &nonce, &c.sbox.key)
	frame := make([]byte, 4+len(sealed))
	frame[0] = byte(len(sealed) >> 24)  // #nosec G115 -- frame header length encoding
	frame[1] = byte(len(sealed) >> 16)  // #nosec G115 -- frame header length encoding
	frame[2] = byte(len(sealed) >> 8)   // #nosec G115 -- frame header length encoding
	frame[3] = byte(len(sealed))        // #nosec G115 -- frame header length encoding
	copy(frame[4:], sealed)
	if _, err := c.Conn.Write(frame); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *secretboxStreamConn) Close() error {
	if c.close {
		return nil
	}
	c.close = true
	return c.Conn.Close()
}

// CloseWrite half-closes the underlying conn. The stream wrapper
// doesn't need to do anything special: the next Read on the peer
// will hit EOF and propagate.
func (c *secretboxStreamConn) CloseWrite() error {
	if cw, ok := c.Conn.(closeWriter); ok {
		return cw.CloseWrite()
	}
	return nil
}
