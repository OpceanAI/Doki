package runtime

import (
	"io"
	"os/exec"
	"testing"
	"time"
)

// TestExecStdinPipeDoesNotBlockWait pins the fix for the exec hang: a child that
// does not read stdin must not keep cmd.Wait() blocked when stdin is fed from a
// pipe whose writer is never closed (the non-interactive exec case). The bug was
// assigning a non-*os.File io.Reader to cmd.Stdin, which makes os/exec spawn an
// internal copy goroutine that Wait() blocks on forever. Using StdinPipe (a real
// fd) means Wait() only depends on the process exiting.
func TestExecStdinPipeDoesNotBlockWait(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("`true` not available")
	}

	// A pipe whose write end is intentionally never closed, mirroring a client
	// that attached stdin but sends no data and never half-closes.
	stdinR, stdinW := io.Pipe()
	defer func() { _ = stdinW.Close() }()

	cmd := exec.Command("true") // exits immediately, ignores stdin
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	go func() {
		_, _ = io.Copy(stdinPipe, stdinR)
		_ = stdinPipe.Close()
	}()

	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cmd.Wait() blocked despite process exit — exec stdin regression")
	}
	// Unblock the copier goroutine.
	_ = stdinR.Close()
}
