package cri

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"testing"
)

// The Sec-WebSocket-Accept value is defined by RFC 6455 §1.3 using a known test
// vector: key "dGhlIHNhbXBsZSBub25jZQ==" must produce
// "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=".
func TestWebSocketAccept(t *testing.T) {
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	sum := sha1.Sum([]byte(key + wsGUID))
	got := base64.StdEncoding.EncodeToString(sum[:])
	if got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("accept = %q, want RFC 6455 test vector", got)
	}
}

func TestNegotiateChannelProtocol(t *testing.T) {
	// v5 is preferred when offered alongside v4.
	if proto, v5 := negotiateChannelProtocol("v4.channel.k8s.io, v5.channel.k8s.io"); proto != "v5.channel.k8s.io" || !v5 {
		t.Errorf("got %q v5=%v, want v5", proto, v5)
	}
	if proto, v5 := negotiateChannelProtocol("v4.channel.k8s.io"); proto != "v4.channel.k8s.io" || v5 {
		t.Errorf("got %q v5=%v, want v4", proto, v5)
	}
	if proto, _ := negotiateChannelProtocol("some.other.proto"); proto != "" {
		t.Errorf("unsupported protocol negotiated: %q", proto)
	}
}

// maskFrame builds a client-style masked WebSocket binary frame carrying the
// channel byte and payload, as a real client would send.
func maskFrame(channel byte, payload []byte) []byte {
	body := append([]byte{channel}, payload...)
	var buf bytes.Buffer
	buf.WriteByte(0x82) // FIN + binary
	n := len(body)
	switch {
	case n < 126:
		buf.WriteByte(0x80 | byte(n))
	case n < 1<<16:
		buf.WriteByte(0x80 | 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		buf.Write(ext[:])
	}
	mask := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	buf.Write(mask)
	for i, b := range body {
		buf.WriteByte(b ^ mask[i%4])
	}
	return buf.Bytes()
}

// A masked client frame must be unmasked and split into channel + payload.
func TestReadMessageUnmasks(t *testing.T) {
	frame := maskFrame(channelStdin, []byte("echo hi\n"))
	ws := &wsConn{br: bufio.NewReader(bytes.NewReader(frame))}
	ch, payload, err := ws.readMessage()
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	if ch != channelStdin {
		t.Errorf("channel = %d, want %d", ch, channelStdin)
	}
	if string(payload) != "echo hi\n" {
		t.Errorf("payload = %q", payload)
	}
}

// A close frame surfaces as io.EOF so the reader loop terminates.
func TestReadMessageClose(t *testing.T) {
	ws := &wsConn{br: bufio.NewReader(bytes.NewReader([]byte{0x88, 0x00}))}
	if _, _, err := ws.readMessage(); err != io.EOF {
		t.Errorf("close frame err = %v, want io.EOF", err)
	}
}

// writeMessage must emit an unmasked server frame that a client can read back.
func TestWriteMessageRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	ws := &wsConn{conn: server}

	go func() {
		_ = ws.writeMessage(channelStdout, []byte("output-data"))
		_ = server.Close()
	}()

	br := bufio.NewReader(client)
	var hdr [2]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil {
		t.Fatal(err)
	}
	if hdr[0] != 0x82 {
		t.Errorf("first byte = %#x, want 0x82 (FIN+binary)", hdr[0])
	}
	if hdr[1]&0x80 != 0 {
		t.Error("server frame must not be masked")
	}
	length := int(hdr[1] & 0x7f)
	body := make([]byte, length)
	if _, err := io.ReadFull(br, body); err != nil {
		t.Fatal(err)
	}
	if body[0] != channelStdout {
		t.Errorf("channel byte = %d, want %d", body[0], channelStdout)
	}
	if string(body[1:]) != "output-data" {
		t.Errorf("payload = %q", body[1:])
	}
}

// A large payload (>125 bytes) must use the 16-bit extended length path in both
// directions.
func TestExtendedLengthRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 500)
	frame := maskFrame(channelStdin, payload)
	ws := &wsConn{br: bufio.NewReader(bytes.NewReader(frame))}
	ch, got, err := ws.readMessage()
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	if ch != channelStdin || !bytes.Equal(got, payload) {
		t.Errorf("extended-length frame round-trip failed (len=%d)", len(got))
	}
}
