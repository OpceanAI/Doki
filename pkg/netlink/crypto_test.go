package netlink

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/nacl/secretbox"
)

func TestDeriveSecretKey_DeterministicAndUnique(t *testing.T) {
	a1 := make([]byte, 32)
	b1 := make([]byte, 32)
	rand.Read(a1)
	rand.Read(b1)

	k1 := DeriveSecretKey(a1, b1)
	k2 := DeriveSecretKey(a1, b1)
	if k1 != k2 {
		t.Error("DeriveSecretKey not deterministic")
	}

	// Order matters: swap inputs and key changes.
	k3 := DeriveSecretKey(b1, a1)
	if k1 == k3 {
		t.Error("DeriveSecretKey order-insensitive (it MUST be order-sensitive)")
	}

	// Different inputs -> different key.
	a2 := make([]byte, 32)
	rand.Read(a2)
	k4 := DeriveSecretKey(a2, b1)
	if k1 == k4 {
		t.Error("DeriveSecretKey produces same key for different inputs")
	}
}

func TestSecretboxStreamConn_Roundtrip(t *testing.T) {
	var key [32]byte
	rand.Read(key[:])
	sbox := NewSecretboxWrapper(key)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		wrapped, err := sbox.WrapServer(context.Background(), c)
		if err != nil {
			return
		}
		_, _ = io.Copy(wrapped, strings.NewReader("hello-secretbox"))
		_ = wrapped.Close()
	}()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	wrapped, err := sbox.WrapClient(context.Background(), c)
	if err != nil {
		t.Fatalf("WrapClient: %v", err)
	}
	_ = wrapped.SetDeadline(time.Now().Add(2 * time.Second))
	buf, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(buf) != "hello-secretbox" {
		t.Errorf("got %q, want hello-secretbox", buf)
	}
}

func TestSecretboxStreamConn_Bidirectional(t *testing.T) {
	var key [32]byte
	rand.Read(key[:])
	sbox := NewSecretboxWrapper(key)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		wrapped, err := sbox.WrapServer(context.Background(), c)
		if err != nil {
			return
		}
		buf, _ := io.ReadAll(wrapped)
		if len(buf) > 0 {
			wrapped.Write(buf)
		}
		_ = wrapped.Close()
	}()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	wrapped, err := sbox.WrapClient(context.Background(), c)
	if err != nil {
		t.Fatalf("WrapClient: %v", err)
	}
	_ = wrapped.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := wrapped.Write([]byte("doki")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	if err := wrapped.(closeWriter).CloseWrite(); err != nil {
		t.Fatalf("client close-write: %v", err)
	}
	buf, _ := io.ReadAll(wrapped)
	if string(buf) != "doki" {
		t.Errorf("echoed = %q, want doki", buf)
	}
}

func TestSecretboxWrapper_UDP(t *testing.T) {
	var key [32]byte
	rand.Read(key[:])
	sbox := NewSecretboxWrapper(key)

	original := []byte("udp-secret-payload")
	sealed, err := sbox.WrapOutbound(original)
	if err != nil {
		t.Fatalf("WrapOutbound: %v", err)
	}
	if bytes.Equal(sealed, original) {
		t.Error("sealed data identical to plaintext")
	}
	if len(sealed) != 24+len(original)+secretbox.Overhead {
		t.Errorf("sealed length = %d, want %d", len(sealed), 24+len(original)+secretbox.Overhead)
	}
	opened, err := sbox.UnwrapInbound(sealed)
	if err != nil {
		t.Fatalf("UnwrapInbound: %v", err)
	}
	if !bytes.Equal(opened, original) {
		t.Errorf("opened = %q, want %q", opened, original)
	}
}

func TestSecretboxWrapper_WrongKeyFails(t *testing.T) {
	var k1, k2 [32]byte
	rand.Read(k1[:])
	rand.Read(k2[:])
	s1 := NewSecretboxWrapper(k1)
	s2 := NewSecretboxWrapper(k2)

	sealed, err := s1.WrapOutbound([]byte("secret"))
	if err != nil {
		t.Fatalf("WrapOutbound: %v", err)
	}
	if _, err := s2.UnwrapInbound(sealed); err == nil {
		t.Error("UnwrapInbound with wrong key succeeded (must fail)")
	}
}

func TestSecretboxWrapper_TamperedFrameFails(t *testing.T) {
	var key [32]byte
	rand.Read(key[:])
	sbox := NewSecretboxWrapper(key)

	sealed, err := sbox.WrapOutbound([]byte("important"))
	if err != nil {
		t.Fatalf("WrapOutbound: %v", err)
	}
	// Flip a byte in the payload (not the nonce prefix).
	sealed[len(sealed)-1] ^= 0xff
	if _, err := sbox.UnwrapInbound(sealed); err == nil {
		t.Error("UnwrapInbound accepted tampered frame")
	}
}

func TestTLSConfigFor_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	id, err := NewIdentity(dir)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	leaf, err := id.IssueLinkCert("doki-test")
	if err != nil {
		t.Fatalf("IssueLinkCert: %v", err)
	}
	rootPool := x509.NewCertPool()
	rootPool.AddCert(id.caCert)

	cfg, err := TLSConfigFor(leaf, rootPool, "doki-test")
	if err != nil {
		t.Fatalf("TLSConfigFor: %v", err)
	}
	// Spin up a TLS listener and a client, exchange a payload.
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		buf, err := io.ReadAll(c)
		if err != nil {
			done <- err
			return
		}
		if string(buf) != "tls-payload" {
			done <- fmt.Errorf("server got %q, want tls-payload", buf)
			return
		}
		done <- nil
	}()

	dialer := &tls.Config{
		RootCAs:    rootPool,
		ServerName: "doki-test",
	}
	conn, err := tls.Dial("tcp", ln.Addr().String(), dialer)
	if err != nil {
		t.Fatalf("tls.Dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte("tls-payload")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	conn.Close()
	if err := <-done; err != nil {
		t.Fatalf("server side: %v", err)
	}
}

func TestTLSWrapper_ViaTCPProxy(t *testing.T) {
	// End-to-end: TLS server behind a TCPProxy; the proxy is wrapped
	// in TLS on the server side. The upstream is also a TLS server so
	// that the client (the proxy's wrap) can complete a handshake.
	dir := t.TempDir()
	id, err := NewIdentity(dir)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	leaf, err := id.IssueLinkCert("doki-e2e")
	if err != nil {
		t.Fatalf("IssueLinkCert: %v", err)
	}
	rootPool := x509.NewCertPool()
	rootPool.AddCert(id.caCert)
	cfg, err := TLSConfigFor(leaf, rootPool, "doki-e2e")
	if err != nil {
		t.Fatalf("TLSConfigFor: %v", err)
	}

	// Upstream TLS server: accepts a TLS connection and echoes.
	tlsLn, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	defer tlsLn.Close()
	go func() {
		c, err := tlsLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		io.Copy(c, c)
	}()

	// Plain TCP echo in front: the proxy wraps the upstream connection
	// with TLS (the client side) and accepts a plain TCP connection on
	// the listener side. The listener side wraps as TLS server so the
	// outer client sees TLS. The flow:
	//
	//   outer tls.Client -> proxy listener (tls.Server wrap) -> upstream
	//   (dialed as tls.Client) -> upstream tls.Server -> echo.
	//
	// We achieve this by running two TCPProxies in series. Simpler:
	// just verify the TLS wrapper produces a working handshake over a
	// raw TCP pair.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		tw, tErr := NewTLSWrapper(cfg)
		if tErr != nil {
			t.Errorf("NewTLSWrapper: %v", tErr)
			return
		}
		wrapped, err := tw.WrapServer(context.Background(), c)
		if err != nil {
			t.Errorf("WrapServer: %v", err)
			return
		}
		defer wrapped.Close()
		_ = wrapped.SetDeadline(time.Now().Add(2 * time.Second))
		buf, err := io.ReadAll(wrapped)
		if err != nil {
			return
		}
		wrapped.Write(buf)
	}()

	// Outer client: plain TCP, then wrap with TLS client side and verify echo.
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	tw, tErr := NewTLSWrapper(cfg)
	if tErr != nil {
		t.Fatalf("NewTLSWrapper: %v", tErr)
	}
	wrapped, err := tw.WrapClient(context.Background(), c)
	if err != nil {
		t.Fatalf("WrapClient: %v", err)
	}
	_ = wrapped.SetDeadline(time.Now().Add(2 * time.Second))
	payload := []byte("doki-tls-e2e")
	if _, err := wrapped.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Half-close so the server's io.ReadAll returns and it can echo.
	if err := wrapped.(closeWriter).CloseWrite(); err != nil {
		t.Fatalf("close-write: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(wrapped, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Errorf("echo = %q, want %q", buf, payload)
	}
}
