package cri

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// This file implements the minimal server half of RFC 6455 (WebSocket) plus the
// Kubernetes remotecommand channel framing, with zero new dependencies. The
// upstream k8s.io/kubelet/pkg/cri/streaming library would do this for us but
// drags in client-go + apimachinery, which contradicts Doki's near-zero-dep
// design, so we hand-roll the ~200 lines instead.

// wsGUID is the RFC 6455 magic value appended to Sec-WebSocket-Key.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Kubernetes remotecommand channel bytes. Every frame's first payload byte is
// the channel; the rest is the data for that stream.
const (
	channelStdin  byte = 0
	channelStdout byte = 1
	channelStderr byte = 2
	channelError  byte = 3 // exit status / metav1.Status
	channelResize byte = 4 // JSON {"Width":W,"Height":H}
	channelClose  byte = 5 // v5 only: half-close a stream (EOF)
)

// wsConn is a hijacked connection speaking WebSocket frames.
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
	// v5 indicates the negotiated subprotocol supports the CLOSE channel, which
	// lets a client signal stdin EOF without dropping the whole connection.
	v5 bool
}

// acceptWebSocket validates the upgrade request, negotiates the Kubernetes
// channel subprotocol, and writes the 101 response. It returns a wsConn ready
// for framed I/O.
func acceptWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") {
		return nil, fmt.Errorf("not a websocket upgrade")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, fmt.Errorf("missing Sec-WebSocket-Key")
	}

	// Negotiate the subprotocol: prefer v5 (has the CLOSE channel) over v4.
	proto, v5 := negotiateChannelProtocol(r.Header.Get("Sec-WebSocket-Protocol"))
	if proto == "" {
		return nil, fmt.Errorf("no supported channel subprotocol offered")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("response writer does not support hijacking")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n" +
		"Sec-WebSocket-Protocol: " + proto + "\r\n\r\n"
	if _, err := conn.Write([]byte(resp)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, br: brw.Reader, v5: v5}, nil
}

// negotiateChannelProtocol picks the best Kubernetes channel subprotocol the
// client offered. Returns the chosen protocol and whether it is v5.
func negotiateChannelProtocol(header string) (string, bool) {
	offered := map[string]bool{}
	for _, p := range strings.Split(header, ",") {
		offered[strings.TrimSpace(p)] = true
	}
	if offered["v5.channel.k8s.io"] {
		return "v5.channel.k8s.io", true
	}
	if offered["v4.channel.k8s.io"] {
		return "v4.channel.k8s.io", false
	}
	if offered["channel.k8s.io"] {
		return "channel.k8s.io", false
	}
	return "", false
}

// readMessage reads one WebSocket data frame and returns its channel byte and
// payload. Control frames (close/ping/pong) are handled internally: a close or
// EOF returns io.EOF; a ping is answered with a pong and the read continues.
func (c *wsConn) readMessage() (channel byte, payload []byte, err error) {
	for {
		var hdr [2]byte
		if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
			return 0, nil, err
		}
		opcode := hdr[0] & 0x0f
		masked := hdr[1]&0x80 != 0
		length := uint64(hdr[1] & 0x7f)

		switch length {
		case 126:
			var ext [2]byte
			if _, err := io.ReadFull(c.br, ext[:]); err != nil {
				return 0, nil, err
			}
			length = uint64(binary.BigEndian.Uint16(ext[:]))
		case 127:
			var ext [8]byte
			if _, err := io.ReadFull(c.br, ext[:]); err != nil {
				return 0, nil, err
			}
			length = binary.BigEndian.Uint64(ext[:])
		}

		var maskKey [4]byte
		if masked {
			if _, err := io.ReadFull(c.br, maskKey[:]); err != nil {
				return 0, nil, err
			}
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(c.br, data); err != nil {
			return 0, nil, err
		}
		if masked {
			for i := range data {
				data[i] ^= maskKey[i%4]
			}
		}

		switch opcode {
		case 0x8: // close
			return 0, nil, io.EOF
		case 0x9: // ping -> pong
			_ = c.writeControl(0xA, data)
			continue
		case 0xA: // pong
			continue
		case 0x0, 0x1, 0x2: // continuation / text / binary
			if len(data) == 0 {
				// Empty frame carries no channel byte; ignore.
				continue
			}
			return data[0], data[1:], nil
		default:
			continue
		}
	}
}

// writeMessage writes a single binary frame carrying the channel byte followed
// by the payload. Server frames are never masked (RFC 6455 §5.1).
func (c *wsConn) writeMessage(channel byte, payload []byte) error {
	frame := make([]byte, 0, len(payload)+3)
	frame = append(frame, channel)
	frame = append(frame, payload...)
	return c.writeControl(0x2, frame)
}

// writeControl writes one FIN frame with the given opcode and (already framed)
// payload, unmasked.
func (c *wsConn) writeControl(opcode byte, payload []byte) error {
	var hdr []byte
	b0 := byte(0x80) | opcode // FIN + opcode
	n := len(payload)
	switch {
	case n < 126:
		hdr = []byte{b0, byte(n)}
	case n < 1<<16:
		hdr = []byte{b0, 126, byte(n >> 8), byte(n)}
	default:
		hdr = make([]byte, 10)
		hdr[0] = b0
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(n))
	}
	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	_, err := c.conn.Write(payload)
	return err
}

// Close sends a close frame and closes the underlying connection.
func (c *wsConn) Close() error {
	_ = c.writeControl(0x8, nil)
	return c.conn.Close()
}
