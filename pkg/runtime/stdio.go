package runtime

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/OpceanAI/Doki/internal/pty"
)

// errNotInteractive is returned when an attach/resize is requested for a
// container that was not started with a live stdio broker.
var errNotInteractive = errors.New("container is not interactive")

// stdioBroker brokers a container's live stdio between the child process and
// zero or more attach clients, while always mirroring output to the log file so
// `doki logs` keeps working.
//
// The container process is a direct child of the daemon, so its stdio only
// exists while this daemon instance is alive; like ContainerState.Cmd, the
// broker is never persisted (it hangs off the in-memory state).
//
// Two shapes:
//   - TTY: a single pseudo-terminal. The child's stdin/stdout/stderr are the
//     slave; the daemon holds the master (ptmx) and both reads output from and
//     writes input to it.
//   - non-TTY interactive: three pipes — stdin (daemon writes, child reads),
//     stdout and stderr (child writes, daemon reads).
//
// A slow or dead attach client must never stall the log or the other clients,
// so each client sink is fed through a bounded channel and dropped if it can
// not keep up (this is also what closes HIGH-11: no unbounded follower loop).
type stdioBroker struct {
	tty bool

	ptmx *os.File // TTY master; nil for non-tty

	stdinW  *os.File // non-tty: write end of child stdin; nil for tty
	stdoutR *os.File // non-tty: read end of child stdout; nil for tty
	stderrR *os.File // non-tty: read end of child stderr; nil for tty

	logFile *os.File

	// childFDs are the child-side ends the parent must close after cmd.Start
	// so EOF propagates when the child exits (the slave pts, or the child ends
	// of the pipes).
	childFDs []*os.File

	mu     sync.Mutex
	sinks  map[int]*ioSink
	nextID int
	closed bool

	wg   sync.WaitGroup
	done chan struct{}
}

const (
	streamStdout byte = 1
	streamStderr byte = 2
)

type ioChunk struct {
	stream byte
	data   []byte
}

// ioSink is one attached client. The pump goroutines drop a chunk into queue;
// the sink's own goroutine drains it to the client's pipes. If queue fills
// (client not reading), the pump drops the sink rather than block.
type ioSink struct {
	id      int
	queue   chan ioChunk
	stdoutW *io.PipeWriter
	stderrW *io.PipeWriter
	once    sync.Once
}

func (s *ioSink) close() {
	s.once.Do(func() {
		close(s.queue)
	})
}

// newTTYBroker allocates a pty and returns the broker plus the slave the caller
// must wire to the child and close in the parent after start.
func newTTYBroker(logFile *os.File) (*stdioBroker, *os.File, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, nil, err
	}
	b := &stdioBroker{
		tty:     true,
		ptmx:    master,
		logFile: logFile,
		sinks:   make(map[int]*ioSink),
		done:    make(chan struct{}),
	}
	return b, slave, nil
}

// newPipeBroker builds the three pipes for a non-tty interactive container and
// returns the broker plus the child-side ends the caller wires to the child and
// closes in the parent after start.
func newPipeBroker(logFile *os.File) (b *stdioBroker, childStdin, childStdout, childStderr *os.File, err error) {
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		_ = stdinR.Close()
		_ = stdinW.Close()
		return nil, nil, nil, nil, err
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdinR.Close()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		return nil, nil, nil, nil, err
	}
	b = &stdioBroker{
		tty:     false,
		stdinW:  stdinW,
		stdoutR: stdoutR,
		stderrR: stderrR,
		logFile: logFile,
		sinks:   make(map[int]*ioSink),
		done:    make(chan struct{}),
	}
	return b, stdinR, stdoutW, stderrW, nil
}

// afterStart closes the child-side fds the parent no longer needs (so EOF
// propagates on child exit) and launches the pumps. Call once, right after
// cmd.Start succeeds.
func (b *stdioBroker) afterStart() {
	for _, f := range b.childFDs {
		_ = f.Close()
	}
	b.childFDs = nil
	b.start()
}

// start launches the pump goroutines that copy child output to the log and to
// attached clients. Call once, after the child process has started.
func (b *stdioBroker) start() {
	if b.tty {
		b.wg.Add(1)
		go b.pump(b.ptmx, streamStdout)
	} else {
		b.wg.Add(2)
		go b.pump(b.stdoutR, streamStdout)
		go b.pump(b.stderrR, streamStderr)
	}
	// Close the broker once every pump has drained (i.e. the child exited and
	// all its output fds hit EOF).
	go func() {
		b.wg.Wait()
		b.Close()
	}()
}

func (b *stdioBroker) pump(src *os.File, stream byte) {
	defer b.wg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if b.logFile != nil {
				_, _ = b.logFile.Write(chunk)
			}
			b.fan(stream, chunk)
		}
		if err != nil {
			return
		}
	}
}

// fan delivers a chunk to every attached client without blocking on any of
// them: a client whose queue is full is disconnected.
func (b *stdioBroker) fan(stream byte, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, sink := range b.sinks {
		// Copy: the caller reuses buf on the next read.
		cp := make([]byte, len(data))
		copy(cp, data)
		select {
		case sink.queue <- ioChunk{stream: stream, data: cp}:
		default:
			sink.close()
			delete(b.sinks, id)
		}
	}
}

// AttachSession is the live-stdio handle the API layer streams to a client.
type AttachSession struct {
	Tty    bool
	Stdin  io.WriteCloser // nil if the container is not interactive
	Stdout io.ReadCloser
	Stderr io.ReadCloser // nil in TTY mode (output is combined)
	detach func()
}

// Detach releases the session's resources. Safe to call more than once.
func (a *AttachSession) Detach() {
	if a.detach != nil {
		a.detach()
	}
}

// attach registers a new client and returns its streams. It does not replay
// history — the caller replays the log first, then streams live output.
func (b *stdioBroker) attach() *AttachSession {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		// Container already exited: hand back closed, empty streams so the
		// caller sees a clean EOF instead of hanging.
		pr, pw := io.Pipe()
		_ = pw.Close()
		return &AttachSession{Tty: b.tty, Stdout: pr, detach: func() {}}
	}
	id := b.nextID
	b.nextID++
	stdoutR, stdoutW := io.Pipe()
	sink := &ioSink{
		id:      id,
		queue:   make(chan ioChunk, 256),
		stdoutW: stdoutW,
	}
	sess := &AttachSession{Tty: b.tty, Stdout: stdoutR}
	if b.tty {
		// One combined stream over the pty; stdin goes straight to the master.
		// Close must NOT close the master: on a real terminal EOF is Ctrl-D,
		// and closing the master would SIGHUP the shell before it ever reads
		// the buffered input. The shell exits via its own `exit`.
		sess.Stdin = &sharedWriteCloser{w: b.ptmx, keepOpen: true}
	} else {
		stderrR, stderrW := io.Pipe()
		sink.stderrW = stderrW
		sess.Stderr = stderrR
		// Closing stdin must reach the child so `cat` and friends see EOF, but
		// only the client that owns this stdin may close it.
		sess.Stdin = &sharedWriteCloser{w: b.stdinW}
	}
	b.sinks[id] = sink
	b.mu.Unlock()

	go sink.run()

	sess.detach = func() {
		b.mu.Lock()
		if s, ok := b.sinks[id]; ok {
			delete(b.sinks, id)
			s.close()
		}
		b.mu.Unlock()
		_ = stdoutR.Close()
	}
	return sess
}

func (s *ioSink) run() {
	for chunk := range s.queue {
		var err error
		switch chunk.stream {
		case streamStderr:
			if s.stderrW != nil {
				_, err = s.stderrW.Write(chunk.data)
			}
		default:
			_, err = s.stdoutW.Write(chunk.data)
		}
		if err != nil {
			break
		}
	}
	_ = s.stdoutW.Close()
	if s.stderrW != nil {
		_ = s.stderrW.Close()
	}
	// Drain any remaining queued chunks so the closing goroutine's send does
	// not block on an abandoned buffer.
	for range s.queue {
	}
}

// resize applies a new window size to the pty. No-op for non-tty containers.
func (b *stdioBroker) resize(rows, cols uint16) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || !b.tty || b.ptmx == nil {
		return nil
	}
	return pty.Setsize(b.ptmx, rows, cols)
}

// Close tears down the broker: it disconnects every client, closes the daemon's
// stdio fds, and closes the log file. Safe to call more than once.
func (b *stdioBroker) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	for id, sink := range b.sinks {
		sink.close()
		delete(b.sinks, id)
	}
	b.mu.Unlock()

	if b.ptmx != nil {
		_ = b.ptmx.Close()
	}
	if b.stdinW != nil {
		_ = b.stdinW.Close()
	}
	if b.stdoutR != nil {
		_ = b.stdoutR.Close()
	}
	if b.stderrR != nil {
		_ = b.stderrR.Close()
	}
	close(b.done)
	if b.logFile != nil {
		_ = b.logFile.Close()
	}
}

// sharedWriteCloser writes to a shared fd (the pty master or the stdin pipe)
// that outlives any single attach session. For a non-TTY stdin pipe, Close
// forwards to the underlying fd so the child sees stdin EOF (e.g. `cat`
// terminates). For a pty master, keepOpen is set so Close does NOT tear down
// the terminal — EOF on a real terminal is Ctrl-D, not a closed fd, and closing
// the master would SIGHUP the shell.
type sharedWriteCloser struct {
	w        io.Writer
	keepOpen bool
	mu       sync.Mutex
	closed   bool
}

func (s *sharedWriteCloser) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	return s.w.Write(p)
}

func (s *sharedWriteCloser) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.keepOpen {
		return nil
	}
	if c, ok := s.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// ── Runtime-level broker registry and helpers ──────────────────────

// registerBroker stores a live broker for a container and arranges for it to be
// removed once it closes.
func (rt *Runtime) registerBroker(id string, b *stdioBroker) {
	if b == nil {
		return
	}
	rt.ioMu.Lock()
	// Replace any stale broker (e.g. from a previous run of a restarted
	// container) so we never leak the old one.
	if old := rt.brokers[id]; old != nil {
		old.Close()
	}
	rt.brokers[id] = b
	rt.ioMu.Unlock()

	go func() {
		<-b.done
		rt.ioMu.Lock()
		if rt.brokers[id] == b {
			delete(rt.brokers, id)
		}
		rt.ioMu.Unlock()
	}()
}

func (rt *Runtime) broker(id string) *stdioBroker {
	rt.ioMu.Lock()
	defer rt.ioMu.Unlock()
	return rt.brokers[id]
}

// IsInteractive reports whether a container was started with a live stdio
// broker (i.e. `run -it` or `run -i`), so the API layer knows whether to attach
// to the process or fall back to following the log file.
func (rt *Runtime) IsInteractive(id string) bool {
	return rt.broker(id) != nil
}

// AttachStreams returns a live-stdio session for an interactive container, or
// an error if the container has no broker (not started interactively, or
// already exited). The caller must Detach the session when done.
func (rt *Runtime) AttachStreams(id string) (*AttachSession, error) {
	b := rt.broker(id)
	if b == nil {
		return nil, errNotInteractive
	}
	return b.attach(), nil
}

// ResizeTTY applies a new window size to an interactive container's pty.
func (rt *Runtime) ResizeTTY(id string, rows, cols uint16) error {
	b := rt.broker(id)
	if b == nil {
		return errNotInteractive
	}
	return b.resize(rows, cols)
}

// isInteractive reports whether a container's config asks for a live stdio
// path (a TTY, or stdin left open). The default, non-interactive container is
// untouched by any of this.
func isInteractive(cfg *Config) bool {
	return cfg.Tty || cfg.Interactive
}

// setupStdio wires a command's stdio for an interactive container and returns
// the broker, or (nil, nil) when the container is not interactive so the caller
// keeps its existing log-file wiring unchanged.
//
// On success the caller must, after cmd.Start:
//   - call broker.afterStart() to close the child-side fds and start the pumps
//   - call rt.registerBroker(id, broker)
//
// and must NOT close logFile itself (the broker owns it now).
func (rt *Runtime) setupStdio(cmd *exec.Cmd, cfg *Config, logFile *os.File) (*stdioBroker, error) {
	if !isInteractive(cfg) {
		return nil, nil
	}

	if cfg.Tty {
		broker, slave, err := newTTYBroker(logFile)
		if err != nil {
			return nil, err
		}
		cmd.Stdin = slave
		cmd.Stdout = slave
		cmd.Stderr = slave
		broker.childFDs = []*os.File{slave}
		if cmd.SysProcAttr == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
		}
		// Making the pts the child's controlling terminal (Setsid+Setctty) gives
		// full job control, but it relies on a real kernel tty subsystem. Under
		// proot on Android/Termux the TIOCSCTTY ioctl is rejected and the exec
		// fails outright, so we only claim the controlling terminal in
		// namespaces mode (real Linux). The pty's line discipline — echo,
		// canonical input, resize — works regardless; only job-control signals
		// need the controlling terminal.
		if rt.mode == ModeNamespaces {
			cmd.SysProcAttr.Setsid = true
			cmd.SysProcAttr.Setctty = true
			cmd.SysProcAttr.Ctty = 0 // fd 0 (stdin) is the slave
		}
		return broker, nil
	}

	// Non-TTY but stdin is open: three pipes.
	broker, childStdin, childStdout, childStderr, err := newPipeBroker(logFile)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = childStdin
	cmd.Stdout = childStdout
	cmd.Stderr = childStderr
	broker.childFDs = []*os.File{childStdin, childStdout, childStderr}
	return broker, nil
}
