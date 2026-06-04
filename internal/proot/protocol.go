package proot

import "encoding/json"

// MessageType is the type of IPC message.
type MessageType string

const (
	// Requests (Doki → doki-proot)
	TypeExec       MessageType = "exec"
	TypeConfig     MessageType = "config"
	TypeSignal     MessageType = "signal"
	TypeHealth     MessageType = "health"
	TypeShutdown   MessageType = "shutdown"
	TypeSubscribe  MessageType = "subscribe"
	TypeUnsubscribe MessageType = "unsubscribe"

	// Responses (doki-proot → Doki)
	TypeStdout     MessageType = "stdout"
	TypeStderr     MessageType = "stderr"
	TypeExit       MessageType = "exit"
	TypeReady      MessageType = "ready"
	TypeError      MessageType = "error"
	TypeExecError  MessageType = "exec_error"
	TypeConfigAck  MessageType = "config_ack"
	TypeSignalAck  MessageType = "signal_ack"
	TypeHealthResp MessageType = "health"
	TypeShutdownAck MessageType = "shutdown_ack"
	TypeSubscribeAck MessageType = "subscribe_ack"
	TypeUnsubscribeAck MessageType = "unsubscribe_ack"
	TypeLog        MessageType = "log"
)

// Request is a message sent to the doki-proot daemon.
type Request struct {
	Type    MessageType `json:"type"`
	ID      string      `json:"id,omitempty"`
	Cmd     []string    `json:"cmd,omitempty"`
	Env     []string    `json:"env,omitempty"`
	Cwd     string      `json:"cwd,omitempty"`
	Sig     string      `json:"sig,omitempty"`
	Hidden  []string    `json:"hidden_files,omitempty"`
	PortMap []PortMap   `json:"port_map,omitempty"`
}

// PortMap is a guest→host port mapping.
type PortMap struct {
	GuestBind int    `json:"guest_bind"`
	HostBind  int    `json:"host_bind"`
	Proto     string `json:"proto"`
}

// Response is a message received from the doki-proot daemon.
type Response struct {
	Type     MessageType `json:"type"`
	ID       string      `json:"id,omitempty"`
	Data     string      `json:"data,omitempty"`
	Code     int         `json:"code,omitempty"`
	Status   string      `json:"status,omitempty"`
	PID      int         `json:"pid,omitempty"`
	Socket   string      `json:"socket,omitempty"`
	Msg      string      `json:"msg,omitempty"`
}

// Marshal serializes the request to newline-delimited JSON.
func (r *Request) Marshal() ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// UnmarshalResponse deserializes a response from JSON bytes.
func UnmarshalResponse(data []byte) (*Response, error) {
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// NewExecRequest creates an exec request.
func NewExecRequest(id string, cmd []string, env []string, cwd string) *Request {
	return &Request{
		Type: TypeExec,
		ID:   id,
		Cmd:  cmd,
		Env:  env,
		Cwd:  cwd,
	}
}

// NewSignalRequest creates a signal request.
func NewSignalRequest(id string, sig string) *Request {
	return &Request{
		Type: TypeSignal,
		ID:   id,
		Sig:  sig,
	}
}

// NewConfigRequest creates a config request.
func NewConfigRequest(hidden []string, ports []PortMap) *Request {
	return &Request{
		Type:   TypeConfig,
		Hidden: hidden,
		PortMap: ports,
	}
}

// NewHealthRequest creates a health check request.
func NewHealthRequest() *Request {
	return &Request{Type: TypeHealth}
}

// NewShutdownRequest creates a shutdown request.
func NewShutdownRequest() *Request {
	return &Request{Type: TypeShutdown}
}

// NewSubscribeRequest creates a subscribe request.
func NewSubscribeRequest(id string) *Request {
	return &Request{
		Type: TypeSubscribe,
		ID:   id,
	}
}

// NewUnsubscribeRequest creates an unsubscribe request.
func NewUnsubscribeRequest(id string) *Request {
	return &Request{
		Type: TypeUnsubscribe,
		ID:   id,
	}
}
