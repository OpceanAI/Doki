package proot

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"
)

// IPCClient communicates with the doki-proot daemon via Unix socket.
type IPCClient struct {
	socketPath string
	conn       net.Conn
	scanner    *bufio.Scanner
	mu         sync.Mutex
	events     *EventBus
	log        *slog.Logger
	connected  bool
}

// NewIPCClient creates a new IPC client.
func NewIPCClient(socketPath string) *IPCClient {
	return &IPCClient{
		socketPath: socketPath,
		events:     NewEventBus(),
		log:        slog.Default().With("component", "proot.ipc"),
	}
}

// Connect connects to the daemon socket.
// If the socket doesn't exist, attempts to start the daemon.
func (c *IPCClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// If socket doesn't exist, try to start daemon.
	if _, err := os.Stat(c.socketPath); os.IsNotExist(err) {
		if err := c.startDaemon(ctx); err != nil {
			return fmt.Errorf("start daemon: %w", err)
		}
	}

	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.socketPath, err)
	}
	c.conn = conn
	c.scanner = bufio.NewScanner(conn)
	c.scanner.Buffer(make([]byte, 65536), 65536)
	c.connected = true

	// Start read loop.
	go c.readLoop()

	c.log.Info("connected to doki-proot daemon", "socket", c.socketPath)
	return nil
}

// startDaemon starts the doki-proot daemon if not running.
func (c *IPCClient) startDaemon(ctx context.Context) error {
	prootBin := FindProotBinary()
	cmd := exec.CommandContext(ctx, prootBin, "--daemon", "--socket", c.socketPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start doki-proot daemon: %w", err)
	}
	// Wait for socket to appear.
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(c.socketPath); err == nil {
			return nil
		}
	}
	return fmt.Errorf("timeout waiting for socket %s", c.socketPath)
}

// Exec sends an exec command to the daemon and returns a channel of events.
func (c *IPCClient) Exec(ctx context.Context, id string, cmd []string, env []string, cwd string) (<-chan ContainerEvent, error) {
	req := NewExecRequest(id, cmd, env, cwd)
	if err := c.send(req); err != nil {
		return nil, err
	}

	// Create event channel for this container.
	events := make(chan ContainerEvent, 64)
	c.events.Subscribe(id, func(event ContainerEvent) {
		select {
		case events <- event:
		case <-ctx.Done():
		}
	})

	// Close channel when container exits.
	go func() {
		defer func() {
			recover() // protect against send on closed channel
			close(events)
		}()
		for event := range events {
			if event.Type == EventExit {
				c.events.Unsubscribe(id)
				return
			}
		}
	}()

	return events, nil
}

// Signal sends a signal to a container.
func (c *IPCClient) Signal(id string, sig string) error {
	return c.send(NewSignalRequest(id, sig))
}

// Health checks daemon health.
func (c *IPCClient) Health() (*Response, error) {
	return c.sendAndWait(NewHealthRequest(), TypeHealthResp)
}

// Shutdown shuts down the daemon.
func (c *IPCClient) Shutdown() error {
	return c.send(NewShutdownRequest())
}

// Config sends configuration (hidden files, port maps).
func (c *IPCClient) Config(hidden []string, ports []PortMap) error {
	return c.send(NewConfigRequest(hidden, ports))
}

// Subscribe subscribes to events for a container.
func (c *IPCClient) Subscribe(id string, fn EventSubscriber) {
	c.events.Subscribe(id, fn)
}

// Unsubscribe unsubscribes from events for a container.
func (c *IPCClient) Unsubscribe(id string) {
	c.events.Unsubscribe(id)
}

// Close closes the connection.
func (c *IPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.connected = false
		return c.conn.Close()
	}
	return nil
}

// IsConnected returns true if connected to daemon.
func (c *IPCClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// SocketPath returns the socket path.
func (c *IPCClient) SocketPath() string {
	return c.socketPath
}

func (c *IPCClient) send(req *Request) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected {
		return fmt.Errorf("not connected to doki-proot daemon")
	}
	data, err := req.Marshal()
	if err != nil {
		return err
	}
	_, err = c.conn.Write(data)
	return err
}

func (c *IPCClient) sendAndWait(req *Request, expectedType MessageType) (*Response, error) {
	if err := c.send(req); err != nil {
		return nil, err
	}
	// Read response with timeout.
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{})
	if !c.scanner.Scan() {
		return nil, fmt.Errorf("read response: %w", c.scanner.Err())
	}
	resp, err := UnmarshalResponse(c.scanner.Bytes())
	if err != nil {
		return nil, err
	}
	if resp.Type == TypeError {
		return nil, fmt.Errorf("daemon error: %s (code %d)", resp.Data, resp.Code)
	}
	return resp, nil
}

func (c *IPCClient) readLoop() {
	for c.scanner.Scan() {
		data := c.scanner.Bytes()
		resp, err := UnmarshalResponse(data)
		if err != nil {
			c.log.Warn("parse response", "err", err, "data", string(data))
			continue
		}
		switch resp.Type {
		case TypeStdout:
			c.events.Publish(ContainerEvent{
				Type: EventStdout, ID: resp.ID, Data: resp.Data,
			})
		case TypeStderr:
			c.events.Publish(ContainerEvent{
				Type: EventStderr, ID: resp.ID, Data: resp.Data,
			})
		case TypeExit:
			c.events.Publish(ContainerEvent{
				Type: EventExit, ID: resp.ID, ExitCode: resp.Code,
			})
		case TypeReady:
			c.log.Info("daemon ready", "socket", resp.Socket, "pid", resp.PID)
		case TypeLog:
			c.log.Debug("daemon log", "msg", resp.Msg)
		default:
			c.log.Debug("message", "type", string(resp.Type), "id", resp.ID)
		}
	}
	if err := c.scanner.Err(); err != nil {
		c.log.Warn("read loop error", "err", err)
	}
	c.log.Info("read loop ended")
}

// FindProotBinary is defined in manager.go.
