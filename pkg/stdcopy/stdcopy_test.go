package stdcopy

import (
	"bytes"
	"io"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("hello world\n")
	if _, err := WriteFrame(&buf, StreamStdout, payload); err != nil {
		t.Fatal(err)
	}

	typ, got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if typ != StreamStdout {
		t.Errorf("typ=%d", typ)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("got=%q, want %q", got, payload)
	}
}

func TestStdCopy(t *testing.T) {
	var src bytes.Buffer
	_, _ = WriteFrame(&src, StreamStdout, []byte("out\n"))
	_, _ = WriteFrame(&src, StreamStderr, []byte("err\n"))
	_, _ = WriteFrame(&src, StreamStdout, []byte("more\n"))

	var stdout, stderr bytes.Buffer
	n, err := StdCopy(&stdout, &stderr, &src)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if stdout.String() != "out\nmore\n" {
		t.Errorf("stdout=%q", stdout.String())
	}
	if stderr.String() != "err\n" {
		t.Errorf("stderr=%q", stderr.String())
	}
	if n == 0 {
		t.Errorf("expected n>0")
	}
}

func TestReadFrameErrors(t *testing.T) {
	// Stream type >2 is invalid.
	var buf bytes.Buffer
	buf.Write([]byte{0xff, 0, 0, 0, 0, 0, 0, 0})
	if _, _, err := ReadFrame(&buf); err == nil {
		t.Error("expected error on invalid stream type")
	}
}

func TestEmptyFrame(t *testing.T) {
	var buf bytes.Buffer
	if _, err := WriteFrame(&buf, StreamStdout, nil); err != nil {
		t.Fatal(err)
	}
	typ, payload, err := ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if typ != StreamStdout {
		t.Errorf("typ=%d", typ)
	}
	if len(payload) != 0 {
		t.Errorf("payload=%q", payload)
	}
}
