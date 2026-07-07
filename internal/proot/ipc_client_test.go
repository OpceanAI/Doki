package proot

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestProtocolMarshal(t *testing.T) {
	req := &Request{
		Type: TypeExec,
		ID:   "test-001",
		Cmd:  []string{"/bin/echo", "hello"},
		Env:  []string{"PATH=/usr/bin"},
		Cwd:  "/app",
	}
	data, err := req.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(data) == 0 {
		t.Error("empty marshal")
	}
	if data[len(data)-1] != '\n' {
		t.Error("missing newline terminator")
	}
	// Verify JSON contains expected fields.
	s := string(data)
	for _, want := range []string{`"type":"exec"`, `"id":"test-001"`, `"cmd":`} {
		if !contains(s, want) {
			t.Errorf("marshal missing %q", want)
		}
	}
}

func TestProtocolUnmarshalResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Response
	}{
		{
			name:  "stdout",
			input: `{"type":"stdout","id":"test-001","data":"hello world","code":0}`,
			want:  Response{Type: TypeStdout, ID: "test-001", Data: "hello world", Code: 0},
		},
		{
			name:  "stderr",
			input: `{"type":"stderr","id":"test-001","data":"error msg","code":1}`,
			want:  Response{Type: TypeStderr, ID: "test-001", Data: "error msg", Code: 1},
		},
		{
			name:  "exit",
			input: `{"type":"exit","id":"test-001","code":0}`,
			want:  Response{Type: TypeExit, ID: "test-001", Code: 0},
		},
		{
			name:  "ready",
			input: `{"type":"ready","socket":"/tmp/doki-proot.sock","pid":12345}`,
			want:  Response{Type: TypeReady, Socket: "/tmp/doki-proot.sock", PID: 12345},
		},
		{
			name:  "health",
			input: `{"type":"health","status":"ok","pid":12345}`,
			want:  Response{Type: TypeHealthResp, Status: "ok", PID: 12345},
		},
		{
			name:  "signal_ack",
			input: `{"type":"signal_ack","id":"test-001","data":"ok","code":0}`,
			want:  Response{Type: TypeSignalAck, ID: "test-001", Data: "ok", Code: 0},
		},
		{
			name:  "subscribe_ack",
			input: `{"type":"subscribe_ack","id":"test-001","data":"ok","code":0}`,
			want:  Response{Type: TypeSubscribeAck, ID: "test-001", Data: "ok", Code: 0},
		},
		{
			name:  "error",
			input: `{"type":"error","data":"unknown type","code":-1}`,
			want:  Response{Type: TypeError, Data: "unknown type", Code: -1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := UnmarshalResponse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if resp.Type != tt.want.Type {
				t.Errorf("Type = %s, want %s", resp.Type, tt.want.Type)
			}
			if resp.ID != tt.want.ID {
				t.Errorf("ID = %s, want %s", resp.ID, tt.want.ID)
			}
			if resp.Data != tt.want.Data {
				t.Errorf("Data = %s, want %s", resp.Data, tt.want.Data)
			}
			if resp.Code != tt.want.Code {
				t.Errorf("Code = %d, want %d", resp.Code, tt.want.Code)
			}
			if resp.PID != tt.want.PID {
				t.Errorf("PID = %d, want %d", resp.PID, tt.want.PID)
			}
			if resp.Socket != tt.want.Socket {
				t.Errorf("Socket = %s, want %s", resp.Socket, tt.want.Socket)
			}
		})
	}
}

func TestNewRequests(t *testing.T) {
	t.Run("exec", func(t *testing.T) {
		r := NewExecRequest("id1", []string{"/bin/sh"}, []string{"PATH=/"}, "/tmp")
		if r.Type != TypeExec {
			t.Errorf("Type = %s, want exec", r.Type)
		}
		if r.ID != "id1" {
			t.Errorf("ID = %s, want id1", r.ID)
		}
	})
	t.Run("signal", func(t *testing.T) {
		r := NewSignalRequest("id1", "SIGTERM")
		if r.Type != TypeSignal || r.Sig != "SIGTERM" {
			t.Errorf("unexpected signal request: %+v", r)
		}
	})
	t.Run("config", func(t *testing.T) {
		r := NewConfigRequest([]string{"/proc/self/maps"}, []PortMap{{GuestBind: 80, HostBind: 8080, Proto: "tcp"}})
		if r.Type != TypeConfig || len(r.Hidden) != 1 || len(r.PortMap) != 1 {
			t.Errorf("unexpected config request: %+v", r)
		}
	})
	t.Run("health", func(t *testing.T) {
		r := NewHealthRequest()
		if r.Type != TypeHealth {
			t.Errorf("Type = %s, want health", r.Type)
		}
	})
	t.Run("shutdown", func(t *testing.T) {
		r := NewShutdownRequest()
		if r.Type != TypeShutdown {
			t.Errorf("Type = %s, want shutdown", r.Type)
		}
	})
	t.Run("subscribe", func(t *testing.T) {
		r := NewSubscribeRequest("id1")
		if r.Type != TypeSubscribe || r.ID != "id1" {
			t.Errorf("unexpected subscribe request: %+v", r)
		}
	})
	t.Run("unsubscribe", func(t *testing.T) {
		r := NewUnsubscribeRequest("id1")
		if r.Type != TypeUnsubscribe || r.ID != "id1" {
			t.Errorf("unexpected unsubscribe request: %+v", r)
		}
	})
}

func TestEventBusSubscribe(t *testing.T) {
	bus := NewEventBus()
	received := make(chan ContainerEvent, 1)
	bus.Subscribe("test-001", func(event ContainerEvent) {
		received <- event
	})
	bus.Publish(ContainerEvent{
		Type: EventStdout, ID: "test-001", Data: "hello",
	})
	select {
	case event := <-received:
		if event.Data != "hello" {
			t.Errorf("data = %s, want hello", event.Data)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestEventBusUnsubscribe(t *testing.T) {
	bus := NewEventBus()
	count := 0
	bus.Subscribe("test-001", func(event ContainerEvent) {
		count++
	})
	bus.Unsubscribe("test-001")
	bus.Publish(ContainerEvent{Type: EventStdout, ID: "test-001", Data: "x"})
	time.Sleep(100 * time.Millisecond)
	if count != 0 {
		t.Errorf("received %d events after unsubscribe", count)
	}
}

func TestEventBusSubscriberCount(t *testing.T) {
	bus := NewEventBus()
	if bus.SubscriberCount("test-001") != 0 {
		t.Error("expected 0 subscribers initially")
	}
	bus.Subscribe("test-001", func(event ContainerEvent) {})
	bus.Subscribe("test-001", func(event ContainerEvent) {})
	if bus.SubscriberCount("test-001") != 2 {
		t.Errorf("expected 2 subscribers, got %d", bus.SubscriberCount("test-001"))
	}
}

func TestEventBusMultipleContainers(t *testing.T) {
	bus := NewEventBus()
	events1 := make(chan ContainerEvent, 1)
	events2 := make(chan ContainerEvent, 1)
	bus.Subscribe("c1", func(e ContainerEvent) { events1 <- e })
	bus.Subscribe("c2", func(e ContainerEvent) { events2 <- e })

	bus.Publish(ContainerEvent{Type: EventStdout, ID: "c1", Data: "from-c1"})
	bus.Publish(ContainerEvent{Type: EventStdout, ID: "c2", Data: "from-c2"})

	select {
	case e := <-events1:
		if e.Data != "from-c1" {
			t.Errorf("c1 got %s", e.Data)
		}
	case <-time.After(time.Second):
		t.Error("timeout c1")
	}
	select {
	case e := <-events2:
		if e.Data != "from-c2" {
			t.Errorf("c2 got %s", e.Data)
		}
	case <-time.After(time.Second):
		t.Error("timeout c2")
	}
}

func TestIPCClientNotConnected(t *testing.T) {
	client := NewIPCClient("/nonexistent.sock")
	err := client.send(&Request{Type: TypeHealth})
	if err == nil {
		t.Error("expected error for not connected")
	}
	if client.IsConnected() {
		t.Error("should not be connected")
	}
}

func TestIPCClientConnect(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	client := NewIPCClient(sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Error("should be connected")
	}
	if client.SocketPath() != sockPath {
		t.Errorf("SocketPath = %s, want %s", client.SocketPath(), sockPath)
	}
}

func TestIPCClientConnectTimeout(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "nonexistent.sock")
	client := NewIPCClient(sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestFindProotBinary(t *testing.T) {
	bin := FindProotBinary()
	if bin == "" {
		t.Skip("proot binary not found in this environment")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
