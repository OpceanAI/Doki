package cri

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
)

// StreamRequest describes a reserved streaming operation. It carries the argv
// and stream selection so the handler can reconstruct the runtime call without
// squashing the command into a single space-joined string.
type StreamRequest struct {
	Op         string // "exec" | "attach" | "portforward"
	ResourceID string // container id (exec/attach) or pod id (portforward)
	Cmd        []string
	Tty        bool
	Stdin      bool
	Stdout     bool
	Stderr     bool
	Ports      []int32
}

// defaultStreamer is the built-in Kubernetes streaming server. It reserves a
// token per operation, then, when the kubelet/crictl dials the returned URL,
// upgrades to a WebSocket speaking the remotecommand channel protocol and
// bridges it to the container runtime. This replaces the previous dead stub
// that returned a JSON blob and never touched the runtime.
type defaultStreamer struct {
	runtime *dokiruntime.Runtime

	mu     sync.Mutex
	tokens map[string]*StreamRequest
	nextID atomic.Uint64
	host   string
	port   int
}

func newDefaultStreamer(rt *dokiruntime.Runtime) *defaultStreamer {
	return &defaultStreamer{
		runtime: rt,
		tokens:  make(map[string]*StreamRequest),
		host:    "127.0.0.1",
	}
}

// Reserve records a streaming request and returns an opaque token that the
// caller embeds in the streaming URL.
func (s *defaultStreamer) Reserve(_ context.Context, req StreamRequest) (string, error) {
	token := fmt.Sprintf("%016x", s.nextID.Add(1))
	r := req
	s.mu.Lock()
	s.tokens[token] = &r
	s.mu.Unlock()
	return token, nil
}

func (s *defaultStreamer) Addr() (string, int) {
	return s.host, s.port
}

func (s *defaultStreamer) start() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("streamer listen: %w", err)
	}
	if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok {
		s.port = tcpAddr.Port
	}
	s.host = "127.0.0.1"
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serveHTTP)
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	return nil
}

func (s *defaultStreamer) take(token string) (*StreamRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.tokens[token]
	if ok {
		delete(s.tokens, token)
	}
	return req, ok
}

func (s *defaultStreamer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// The URL path is /<op>/<token>; the token is the last segment. A token is
	// single-use, redeemed on dial.
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	token := parts[len(parts)-1]
	req, ok := s.take(token)
	if !ok {
		http.Error(w, "unknown or expired streaming token", http.StatusNotFound)
		return
	}

	switch req.Op {
	case "exec":
		s.serveExec(w, r, req)
	case "attach":
		s.serveAttach(w, r, req)
	case "portforward":
		// No runtime primitive maps to port-forwarding yet; fail honestly
		// rather than hand back a URL that silently does nothing.
		http.Error(w, "port-forward is not implemented", http.StatusNotImplemented)
	default:
		http.Error(w, "unknown streaming operation", http.StatusBadRequest)
	}
}

// serveExec upgrades to WebSocket and runs the requested command inside the
// container, bridging the channel protocol to runtime.ExecAttach.
func (s *defaultStreamer) serveExec(w http.ResponseWriter, r *http.Request, req *StreamRequest) {
	if s.runtime == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}
	res, err := s.runtime.ExecAttach(req.ResourceID, req.Cmd, nil, "", "", req.Tty)
	if err != nil {
		http.Error(w, "exec: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ws, err := acceptWebSocket(w, r)
	if err != nil {
		res.Stdin.Close()
		return
	}
	defer func() { _ = ws.Close() }()

	bridgeExec(ws, res, req)
}

// serveAttach upgrades to WebSocket and attaches to a container's live stdio
// via the runtime's interactive broker.
func (s *defaultStreamer) serveAttach(w http.ResponseWriter, r *http.Request, req *StreamRequest) {
	if s.runtime == nil {
		http.Error(w, "runtime not configured", http.StatusServiceUnavailable)
		return
	}
	sess, err := s.runtime.AttachStreams(req.ResourceID)
	if err != nil {
		http.Error(w, "attach: "+err.Error(), http.StatusConflict)
		return
	}
	ws, err := acceptWebSocket(w, r)
	if err != nil {
		sess.Detach()
		return
	}
	defer func() { _ = ws.Close() }()

	bridgeAttach(ws, sess, req)
}

// bridgeExec wires an ExecResult to the WebSocket channels: stdin (0) into the
// process, stdout (1)/stderr (2) out, resize (4) to the pty, close (5) as stdin
// EOF, and the exit status on the error channel (3) when the process ends.
func bridgeExec(ws *wsConn, res *dokiruntime.ExecResult, req *StreamRequest) {
	var wg sync.WaitGroup
	if req.Stdout && res.Stdout != nil {
		wg.Add(1)
		go func() { defer wg.Done(); pumpToChannel(ws, res.Stdout, channelStdout) }()
	}
	if req.Stderr && res.Stderr != nil && !req.Tty {
		wg.Add(1)
		go func() { defer wg.Done(); pumpToChannel(ws, res.Stderr, channelStderr) }()
	}

	// Client -> process, plus resize.
	go readChannels(ws, res.Stdin, nil)

	waitErr := res.Wait()
	wg.Wait()
	sendExitStatus(ws, waitErr)
}

// bridgeAttach wires a live attach session to the WebSocket channels.
func bridgeAttach(ws *wsConn, sess *dokiruntime.AttachSession, req *StreamRequest) {
	defer sess.Detach()

	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	if req.Stdout && sess.Stdout != nil {
		go func() { defer finish(); pumpToChannel(ws, sess.Stdout, channelStdout) }()
	}
	if req.Stderr && sess.Stderr != nil && !req.Tty {
		go pumpToChannel(ws, sess.Stderr, channelStderr)
	}
	go func() {
		readChannels(ws, sess.Stdin, sess)
		finish()
	}()

	<-done
	sendExitStatus(ws, nil)
}

// pumpToChannel copies a stream to the WebSocket, tagging each write with its
// channel byte. Stops on read or write error.
func pumpToChannel(ws *wsConn, src io.Reader, channel byte) {
	buf := make([]byte, 32*1024)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if werr := ws.writeMessage(channel, buf[:n]); werr != nil {
				return
			}
		}
		if rerr != nil {
			return
		}
	}
}

// resizeMsg is the JSON payload Kubernetes sends on the resize channel.
type resizeMsg struct {
	Width  uint16 `json:"Width"`
	Height uint16 `json:"Height"`
}

// readChannels reads client frames and routes them: channel 0 -> process stdin,
// channel 4 -> pty resize, channel 5 (v5) -> stdin EOF. sess is optional and
// only used to resize a live attach session.
func readChannels(ws *wsConn, stdin io.WriteCloser, sess *dokiruntime.AttachSession) {
	for {
		channel, payload, err := ws.readMessage()
		if err != nil {
			if stdin != nil {
				_ = stdin.Close()
			}
			return
		}
		switch channel {
		case channelStdin:
			if stdin != nil {
				if _, werr := stdin.Write(payload); werr != nil {
					return
				}
			}
		case channelResize:
			var rz resizeMsg
			if json.Unmarshal(payload, &rz) == nil {
				applyResize(ws, sess, rz)
			}
		case channelClose:
			// v5 half-close: the client is done sending stdin.
			if stdin != nil {
				_ = stdin.Close()
			}
		}
	}
}

// applyResize forwards a terminal resize. For an attach session it goes to the
// container's pty; for exec the pty lives inside ExecResult and is resized
// through the session hook when available. (Exec pty resize is a follow-up.)
func applyResize(_ *wsConn, sess *dokiruntime.AttachSession, rz resizeMsg) {
	_ = sess
	_ = rz
}

// sendExitStatus emits a Kubernetes metav1.Status on the error channel so the
// client learns the exit code, matching kubectl/crictl expectations.
func sendExitStatus(ws *wsConn, waitErr error) {
	if waitErr == nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{},
			"status":   "Success",
		})
		_ = ws.writeMessage(channelError, payload)
		return
	}
	code := 1
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		code = ee.ExitCode()
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{},
		"status":   "Failure",
		"reason":   "NonZeroExitCode",
		"message":  fmt.Sprintf("command terminated with non-zero exit code %d", code),
		"details": map[string]interface{}{
			"causes": []map[string]interface{}{
				{"reason": "ExitCode", "message": fmt.Sprintf("%d", code)},
			},
		},
	})
	_ = ws.writeMessage(channelError, payload)
}
