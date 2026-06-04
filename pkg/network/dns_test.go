package network

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// TestDNSServer_LocalResolution verifies that a name registered via AddEntry
// is resolved without going to the upstream.
func TestDNSServer_LocalResolution(t *testing.T) {
	d := NewDNSServer([]string{"127.0.0.1:1"}) // bogus upstream
	defer d.Stop()

	if err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ip, ok := d.Resolve("my-container")
	if ok {
		t.Errorf("Resolve before AddEntry should fail, got %s", ip)
	}
	d.AddEntry("my-container", "172.17.0.5")
	got, ok := d.Resolve("my-container")
	if !ok {
		t.Fatal("Resolve after AddEntry should succeed")
	}
	if got != "172.17.0.5" {
		t.Errorf("Resolve = %q, want 172.17.0.5", got)
	}
	d.RemoveEntry("my-container")
	if _, ok := d.Resolve("my-container"); ok {
		t.Error("Resolve after RemoveEntry should fail")
	}
}

// TestDNSServer_EntryCount verifies entry counting.
func TestDNSServer_EntryCount(t *testing.T) {
	d := NewDNSServer(nil)
	defer d.Stop()
	if d.EntryCount() != 0 {
		t.Errorf("EntryCount = %d, want 0", d.EntryCount())
	}
	d.AddEntry("a", "1.1.1.1")
	d.AddEntry("b", "2.2.2.2")
	if d.EntryCount() != 2 {
		t.Errorf("EntryCount = %d, want 2", d.EntryCount())
	}
	d.RemoveEntry("a")
	if d.EntryCount() != 1 {
		t.Errorf("EntryCount after remove = %d, want 1", d.EntryCount())
	}
}

// TestDNSServer_QueryLocalHit sends a real DNS query for an A record
// of a locally-registered name and verifies we get a NOERROR response
// with the right IP in the answer.
func TestDNSServer_QueryLocalHit(t *testing.T) {
	d := NewDNSServer(nil)
	defer d.Stop()
	if err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	d.AddEntry("web.test", "10.0.0.42")

	query := buildTestQuery(0x1234, "web.test", dnsTypeA)
	client, err := net.Dial("udp", d.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Write(query); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp := make([]byte, 512)
	n, err := client.Read(resp)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	resp = resp[:n]

	if len(resp) < 12 {
		t.Fatalf("response too short: %d bytes", len(resp))
	}
	flags := uint16(resp[2])<<8 | uint16(resp[3])
	if flags&0x8000 == 0 {
		t.Errorf("response bit not set: flags=0x%04x", flags)
	}
	rcode := flags & 0x000F
	if rcode != 0 {
		t.Errorf("rcode = %d, want 0", rcode)
	}
	if !strings.Contains(string(resp), string([]byte{10, 0, 0, 42})) {
		t.Errorf("response missing answer IP 10.0.0.42: %v", resp)
	}
}

// TestDNSServer_TCPQuery verifies TCP DNS works (RFC 5966).
func TestDNSServer_TCPQuery(t *testing.T) {
	d := NewDNSServer(nil)
	defer d.Stop()
	if err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	d.AddEntry("tcp-test", "10.0.0.99")

	query := buildTestQuery(0xABCD, "tcp-test", dnsTypeA)

	conn, err := net.Dial("tcp", d.Addr())
	if err != nil {
		t.Fatalf("tcp dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	// Send with 2-byte length prefix.
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(query)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		t.Fatalf("write len: %v", err)
	}
	if _, err := conn.Write(query); err != nil {
		t.Fatalf("write query: %v", err)
	}

	// Read response with length prefix.
	var respLenBuf [2]byte
	if _, err := conn.Read(respLenBuf[:]); err != nil {
		t.Fatalf("read resp len: %v", err)
	}
	respLen := binary.BigEndian.Uint16(respLenBuf[:])
	if respLen == 0 || respLen > 4096 {
		t.Fatalf("invalid resp length: %d", respLen)
	}
	resp := make([]byte, respLen)
	if _, err := conn.Read(resp); err != nil {
		t.Fatalf("read resp: %v", err)
	}

	if len(resp) < 12 {
		t.Fatalf("response too short: %d", len(resp))
	}
	flags := uint16(resp[2])<<8 | uint16(resp[3])
	if flags&0x8000 == 0 {
		t.Error("QR bit not set")
	}
	if !strings.Contains(string(resp), string([]byte{10, 0, 0, 99})) {
		t.Error("response missing 10.0.0.99")
	}

	// Verify TCP metric.
	if d.tcpQueries.Load() == 0 {
		t.Error("tcpQueries metric not incremented")
	}
}

// TestDNSServer_UpstreamFailover verifies failover to second upstream.
func TestDNSServer_UpstreamFailover(t *testing.T) {
	// First upstream: bogus (port 1 = no listener).
	// Second upstream: also bogus, but tests the failover path.
	d := NewDNSServer([]string{"127.0.0.1:1", "127.0.0.1:2"})
	defer d.Stop()
	if err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	query := buildTestQuery(0x5678, "nonexistent.example", dnsTypeA)
	client, err := net.Dial("udp", d.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Write(query); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp := make([]byte, 512)
	n, err := client.Read(resp)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	resp = resp[:n]

	// Should get SERVFAIL (both upstreams are bogus).
	if len(resp) < 4 {
		t.Fatalf("response too short: %d", len(resp))
	}
	flags := uint16(resp[2])<<8 | uint16(resp[3])
	rcode := flags & 0x000F
	if rcode != dnsRcodeServFail {
		t.Errorf("rcode = %d, want SERVFAIL (%d)", rcode, dnsRcodeServFail)
	}

	// Verify failover metric.
	if d.forwardFails.Load() == 0 {
		t.Error("forwardFails metric not incremented")
	}
}

// TestDNSServer_AAAAHandling verifies AAAA queries are forwarded.
func TestDNSServer_AAAAHandling(t *testing.T) {
	d := NewDNSServer(nil)
	defer d.Stop()
	if err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// AAAA query for a name that has a local A entry.
	// Should NOT match locally (type mismatch), should forward.
	d.AddEntry("dual.example", "10.0.0.1")

	query := buildTestQuery(0x9999, "dual.example", dnsTypeAAAA)
	client, err := net.Dial("udp", d.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Write(query); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp := make([]byte, 512)
	n, err := client.Read(resp)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	resp = resp[:n]

	// Should get a response (either forwarded or SERVFAIL).
	if len(resp) < 4 {
		t.Fatalf("response too short: %d", len(resp))
	}
	// The query was forwarded (AAAA not handled locally).
	if d.forwards.Load() == 0 && d.forwardFails.Load() == 0 {
		t.Error("expected forward attempt for AAAA query")
	}
}

// TestDNSServer_CacheSetGet exercises the LRU cache directly.
func TestDNSServer_CacheSetGet(t *testing.T) {
	c := newDNSCache(4, 100*time.Millisecond)
	if c.len() != 0 {
		t.Errorf("fresh cache should be empty, got %d", c.len())
	}
	payload := []byte{0x12, 0x34}
	c.set("foo.example", dnsTypeA, payload, 50*time.Millisecond)
	if c.len() != 1 {
		t.Errorf("len after set = %d, want 1", c.len())
	}
	got, _, ok := c.get("foo.example", dnsTypeA)
	if !ok {
		t.Fatal("expected hit")
	}
	if string(got) != string(payload) {
		t.Errorf("got %x, want %x", got, payload)
	}
	// Wait for expiry.
	time.Sleep(80 * time.Millisecond)
	if _, _, ok := c.get("foo.example", dnsTypeA); ok {
		t.Error("expected miss after TTL")
	}
}

// TestDNSServer_CacheEviction confirms the LRU evicts in FIFO order.
func TestDNSServer_CacheEviction(t *testing.T) {
	c := newDNSCache(2, time.Second)
	c.set("a", dnsTypeA, []byte("1"), time.Second)
	c.set("b", dnsTypeA, []byte("2"), time.Second)
	c.set("c", dnsTypeA, []byte("3"), time.Second) // should evict "a"
	if _, _, ok := c.get("a", dnsTypeA); ok {
		t.Error("a should have been evicted")
	}
	if _, _, ok := c.get("b", dnsTypeA); !ok {
		t.Error("b should still be cached")
	}
	if _, _, ok := c.get("c", dnsTypeA); !ok {
		t.Error("c should still be cached")
	}
}

// TestDNSServer_NegativeCaching verifies NXDOMAIN responses are cached.
func TestDNSServer_NegativeCaching(t *testing.T) {
	c := newDNSCache(16, time.Minute)
	// Simulate a NXDOMAIN response.
	nxdomain := buildServFail(
		buildTestQuery(0x1111, "nx.example", dnsTypeA),
		24, "nx.example", dnsTypeA,
	)
	// Fix rcode to NXDOMAIN.
	nxdomain[2] = 0x81
	nxdomain[3] = 0x83

	c.set("nx.example", dnsTypeA, nxdomain, 30*time.Second)
	got, _, ok := c.get("nx.example", dnsTypeA)
	if !ok {
		t.Fatal("expected negative cache hit")
	}
	if len(got) != len(nxdomain) {
		t.Errorf("cached length = %d, want %d", len(got), len(nxdomain))
	}
}

// TestDNSServer_Stats verifies metrics are populated.
func TestDNSServer_Stats(t *testing.T) {
	d := NewDNSServer(nil)
	defer d.Stop()
	if err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	d.AddEntry("stats-test", "1.2.3.4")

	// Send a query to generate metrics.
	query := buildTestQuery(0xAAAA, "stats-test", dnsTypeA)
	client, err := net.Dial("udp", d.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = client.Write(query)
	resp := make([]byte, 512)
	_, _ = client.Read(resp)

	stats := d.Stats()
	if stats["queries"] == 0 {
		t.Error("queries metric is 0")
	}
	if stats["entries"] == 0 {
		t.Error("entries metric is 0")
	}
}

// TestDNSServer_WellKnownAddress verifies the constant.
func TestDNSServer_WellKnownAddress(t *testing.T) {
	addr := DNSWellKnownAddress()
	if addr != "127.0.0.11:53" && addr != "127.0.0.11:8053" {
		t.Errorf("DNSWellKnownAddress() = %s, want 127.0.0.11:53 or 127.0.0.11:8053", addr)
	}
}

// TestDNSServer_AddEntryEmpty verifies empty inputs are ignored.
func TestDNSServer_AddEntryEmpty(t *testing.T) {
	d := NewDNSServer(nil)
	d.AddEntry("", "1.1.1.1")
	d.AddEntry("valid", "")
	d.AddEntry("", "")
	if d.EntryCount() != 0 {
		t.Errorf("EntryCount = %d, want 0", d.EntryCount())
	}
}

// TestDNSServer_CaseInsensitive verifies DNS lookups are case-insensitive.
func TestDNSServer_CaseInsensitive(t *testing.T) {
	d := NewDNSServer(nil)
	d.AddEntry("MyContainer", "10.0.0.1")
	ip, ok := d.Resolve("mycontainer")
	if !ok {
		t.Fatal("expected case-insensitive resolve")
	}
	if ip != "10.0.0.1" {
		t.Errorf("ip = %s, want 10.0.0.1", ip)
	}
}

// TestDNSServer_StopIdempotent verifies multiple Stop() calls don't panic.
func TestDNSServer_StopIdempotent(t *testing.T) {
	d := NewDNSServer(nil)
	if err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	d.Stop()
	d.Stop() // should not panic
}

// ─── DNS wire-format helper for tests ──────────────────────────────

func buildTestQuery(id uint16, name string, qtype uint16) []byte {
	buf := make([]byte, 0, 64)
	buf = append(buf,
		byte(id>>8), byte(id),
		0x01, 0x00, // RD=1
		0x00, 0x01, // QDCOUNT=1
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	)
	for _, label := range strings.Split(name, ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0x00) // root label
	buf = append(buf, byte(qtype>>8), byte(qtype))
	buf = append(buf, 0x00, 0x01) // CLASS IN
	return buf
}
