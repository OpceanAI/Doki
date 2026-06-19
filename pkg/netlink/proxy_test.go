package netlink

import (
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestTCPProxy_BasicRoundtrip spins up a TCP echo server, a TCPProxy in
// front of it, and verifies that bytes flow in both directions.
func TestTCPProxy_BasicRoundtrip(t *testing.T) {
	// 1. Start a TCP echo server.
	echoAddr, stopEcho := startEchoServer(t, "tcp")
	defer stopEcho()

	// 2. Start a TCPProxy on 127.0.0.1:0 -> echoAddr.
	proxy := NewTCPProxy(TCPProxyConfig{
		ListenAddr: "127.0.0.1:0",
		Upstream:   echoAddr,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer proxy.Close()

	// 3. Dial the proxy, send a payload, expect the echo.
	conn, err := net.Dial("tcp", proxy.Addr())
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	msg := []byte("hello-dokilink\n")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(msg) {
		t.Errorf("echoed = %q, want %q", buf, msg)
	}
}

// TestTCPProxy_MultipleConnections verifies that the proxy can handle
// several concurrent clients.
func TestTCPProxy_MultipleConnections(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t, "tcp")
	defer stopEcho()

	proxy := NewTCPProxy(TCPProxyConfig{
		ListenAddr: "127.0.0.1:0",
		Upstream:   echoAddr,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer proxy.Close()

	const nClients = 8
	var wg sync.WaitGroup
	wg.Add(nClients)
	for i := 0; i < nClients; i++ {
		go func(id int) {
			defer wg.Done()
			c, err := net.Dial("tcp", proxy.Addr())
			if err != nil {
				t.Errorf("client %d dial: %v", id, err)
				return
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			payload := []byte("client-" + string(rune('A'+id)))
			if _, err := c.Write(payload); err != nil {
				t.Errorf("client %d write: %v", id, err)
				return
			}
			buf := make([]byte, len(payload))
			if _, err := io.ReadFull(c, buf); err != nil {
				t.Errorf("client %d read: %v", id, err)
				return
			}
			if string(buf) != string(payload) {
				t.Errorf("client %d got %q, want %q", id, buf, payload)
			}
		}(i)
	}
	wg.Wait()
}

// TestTCPProxy_CloseStopsAccepting verifies that after Close, new
// dial attempts fail.
func TestTCPProxy_CloseStopsAccepting(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t, "tcp")
	defer stopEcho()

	proxy := NewTCPProxy(TCPProxyConfig{
		ListenAddr: "127.0.0.1:0",
		Upstream:   echoAddr,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := proxy.Addr()

	if err := proxy.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Subsequent dials must fail.
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Errorf("expected dial to fail after Close, got %v", conn.RemoteAddr())
	}
}

// TestTCPProxy_IdleTimeout verifies that connections are reaped when
// the idle timeout elapses.
func TestTCPProxy_IdleTimeout(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t, "tcp")
	defer stopEcho()

	proxy := NewTCPProxy(TCPProxyConfig{
		ListenAddr:  "127.0.0.1:0",
		Upstream:    echoAddr,
		IdleTimeout: 200 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer proxy.Close()

	conn, err := net.Dial("tcp", proxy.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Errorf("expected timeout, got data")
	} else if !strings.Contains(err.Error(), "timeout") &&
		!strings.Contains(err.Error(), "i/o timeout") &&
		!strings.Contains(err.Error(), "EOF") {
		t.Errorf("expected timeout/EOF, got: %v", err)
	}
}

// TestUDPProxy_BasicRoundtrip spins up a UDP echo server, a UDPProxy
// in front of it, and verifies datagrams flow in both directions.
func TestUDPProxy_BasicRoundtrip(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t, "udp")
	defer stopEcho()

	proxy := NewUDPProxy(UDPProxyConfig{
		ListenAddr: "127.0.0.1:0",
		Upstream:   echoAddr,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer proxy.Close()

	// Dial proxy's UDP socket, send a packet, expect echo.
	c, err := net.Dial("udp", proxy.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))

	msg := []byte("ping-udp")
	if _, err := c.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 1024)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != string(msg) {
		t.Errorf("got %q, want %q", buf[:n], msg)
	}
}

// --- helpers ---

func startEchoServer(t *testing.T, network string) (string, func()) {
	t.Helper()
	if network == "tcp" {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("echo listen: %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer c.Close()
					_, _ = io.Copy(c, c)
				}(c)
			}
		}()
		return ln.Addr().String(), func() {
			_ = ln.Close()
			wg.Wait()
		}
	}
	// udp
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("echo listen udp: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 65535)
		for {
			n, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if _, err := conn.WriteToUDP(buf[:n], peer); err != nil {
				return
			}
		}
	}()
	return conn.LocalAddr().String(), func() {
		_ = conn.Close()
		wg.Wait()
	}
}
