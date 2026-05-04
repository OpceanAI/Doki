package dokivm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// VsockClient communicates with a microVM guest via vsock.
type VsockClient struct {
	cid  uint32
	port uint32
}

// NewVsockClient creates a vsock client for the given CID.
func NewVsockClient(cid, port uint32) *VsockClient {
	return &VsockClient{cid: cid, port: port}
}

// VsockMessage represents a message exchanged via vsock.
type VsockMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage  `json:"data,omitempty"`
	Cmd  []string        `json:"cmd,omitempty"`
	Env  []string        `json:"env,omitempty"`
	Cwd  string          `json:"cwd,omitempty"`
	Tty  bool            `json:"tty,omitempty"`
	Code int             `json:"code,omitempty"`
	Sig  string          `json:"sig,omitempty"`
}

// Exec sends an exec command to the guest via vsock.
func (v *VsockClient) Exec(ctx context.Context, cmd []string, env []string, cwd string, tty bool, stdout, stderr io.Writer) error {
	conn, err := v.dial()
	if err != nil {
		return fmt.Errorf("vsock dial: %w", err)
	}
	defer conn.Close()

	msg := &VsockMessage{
		Type: "exec",
		Cmd:  cmd,
		Env:  env,
		Cwd:  cwd,
		Tty:  tty,
	}
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return fmt.Errorf("send exec: %w", err)
	}

	// Read responses.
	dec := json.NewDecoder(conn)
	for {
		var resp VsockMessage
		if err := dec.Decode(&resp); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		switch resp.Type {
		case "stdout":
			if stdout != nil {
				stdout.Write([]byte(string(resp.Data)))
			}
		case "stderr":
			if stderr != nil {
				stderr.Write([]byte(string(resp.Data)))
			}
		case "exit":
			if resp.Code != 0 {
				return fmt.Errorf("exit code %d", resp.Code)
			}
			return nil
		}
	}
	return nil
}

// Signal sends a signal to the guest process.
func (v *VsockClient) Signal(ctx context.Context, sig string) error {
	conn, err := v.dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	msg := &VsockMessage{Type: "signal", Sig: sig}
	return json.NewEncoder(conn).Encode(msg)
}

// HealthCheck sends a health check probe.
func (v *VsockClient) HealthCheck(ctx context.Context) (string, error) {
	conn, err := v.dial()
	if err != nil {
		return "unhealthy", err
	}
	defer conn.Close()

	msg := &VsockMessage{Type: "health"}
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return "unhealthy", err
	}

	var resp VsockMessage
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return "unhealthy", err
	}
	return string(resp.Data), nil
}

// dial establishes a vsock connection.
func (v *VsockClient) dial() (net.Conn, error) {
	// On Linux, vsock uses AF_VSOCK (family 40).
	// The address format is CID + Port.
	addr := fmt.Sprintf("%d:%d", v.cid, v.port)

	// Try /dev/vsock first (Android/Linux).
	if _, err := os.Stat("/dev/vsock"); err == nil {
		return net.Dial("unix", "/dev/vsock")
	}

	// Try VM sockets via CID:port.
	d := &net.Dialer{Timeout: 5 * time.Second}
	return d.Dial("vsock", addr)
}
