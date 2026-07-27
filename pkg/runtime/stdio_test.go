package runtime

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

// A non-tty broker must deliver the child's stdout to an attached client and,
// separately, mirror it to the log file, so `doki logs` keeps working while
// someone is attached.
func TestPipeBrokerFansOutAndLogs(t *testing.T) {
	logf, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	// Hold the child ends ourselves (no real child process in the test); this
	// stands in for what the container process would write.
	broker, childStdin, childStdout, childStderr, err := newPipeBroker(logf)
	if err != nil {
		t.Fatal(err)
	}
	broker.start()

	sess := broker.attach()

	if _, err := childStdout.WriteString("hello world\n"); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len("hello world\n"))
	if err := readFull(sess.Stdout, got, 2*time.Second); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(got) != "hello world\n" {
		t.Errorf("client got %q", got)
	}

	sess.Detach()
	_ = childStdin.Close()
	_ = childStdout.Close()
	_ = childStderr.Close()
	broker.Close()

	if data, _ := os.ReadFile(logf.Name()); !bytes.Contains(data, []byte("hello world")) {
		t.Errorf("log file did not capture output: %q", data)
	}
}

// Closing an attach session's stdin must reach the child so filters terminate,
// but must not close the shared pipe out from under a second client.
func TestSharedWriteCloser(t *testing.T) {
	r, w, _ := os.Pipe()
	defer func() { _ = r.Close() }()
	swc := &sharedWriteCloser{w: w}
	if _, err := swc.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := swc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Second close is harmless.
	if err := swc.Close(); err != nil {
		t.Fatalf("double close: %v", err)
	}
	// Writing after close fails rather than panics.
	if _, err := swc.Write([]byte("y")); err == nil {
		t.Error("write after close should fail")
	}
}

// A slow client (one that never reads) must be dropped rather than stall the
// pump — otherwise one dead attach freezes the log and every other client.
func TestSlowClientIsDropped(t *testing.T) {
	logf, _ := os.CreateTemp(t.TempDir(), "log")
	broker, _, childStdout, childStderr, err := newPipeBroker(logf)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the child-side stdout open here so we can drive it directly.
	broker.childFDs = nil
	broker.start()
	defer broker.Close()

	// Attach a client but never read from it.
	sess := broker.attach()
	_ = sess

	// Flood well past the sink's queue capacity. If the pump blocked on the
	// slow client this would deadlock; the test's own timeout would catch it,
	// but the assertion is simply that writes keep succeeding.
	done := make(chan struct{})
	go func() {
		defer close(done)
		payload := bytes.Repeat([]byte("A"), 4096)
		for i := 0; i < 1000; i++ {
			if _, err := childStdout.Write(payload); err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pump stalled on a slow client")
	}
	_ = childStderr.Close()
	_ = childStdout.Close()
}

func readFull(r io.Reader, buf []byte, timeout time.Duration) error {
	type res struct {
		n   int
		err error
	}
	ch := make(chan res, 1)
	go func() {
		n, err := io.ReadFull(r, buf)
		ch <- res{n, err}
	}()
	select {
	case got := <-ch:
		return got.err
	case <-time.After(timeout):
		return io.ErrNoProgress
	}
}
