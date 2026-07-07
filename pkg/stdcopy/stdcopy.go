// Package stdcopy implements the Docker multiplexed-stream protocol used
// by the attach, exec and logs endpoints.
//
// The wire format (when Tty=false on the server) is:
//
//	+----------+---------+---------+---------+---------+
//	|  1 byte  | 3 bytes |      4 bytes (BE)         |
//	|  STREAM  | padding | FRAME SIZE (uint32, big)  |
//	+----------+---------+---------+---------+---------+
//	| FRAME PAYLOAD (FRAMEsIZE bytes)                 |
//	+--------------------------------------------------+
//
// STREAM values: 0 = stdin, 1 = stdout, 2 = stderr.
//
// When Tty=true, the server sends raw bytes without framing (the PTY
// already merges stdout+stderr).
//
// This file is the Go counterpart of github.com/docker/cli/pkg/stdcopy
// and matches Docker Engine v25+ framing exactly.
package stdcopy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// StreamType identifies the channel in a multiplexed frame.
type StreamType byte

const (
	StreamStdin  StreamType = 0
	StreamStdout StreamType = 1
	StreamStderr StreamType = 2
)

// ErrInvalidHeader is returned when the 8-byte header is malformed.
var ErrInvalidHeader = errors.New("stdcopy: invalid frame header")

// frameHeaderSize is the fixed size of each frame header.
const frameHeaderSize = 8

// WriteFrame writes a single frame to w: 8-byte header + payload.
func WriteFrame(w io.Writer, t StreamType, payload []byte) (int, error) {
	if len(payload) > 0x7FFFFFFF {
		return 0, fmt.Errorf("stdcopy: frame too large (%d bytes)", len(payload))
	}
	header := make([]byte, frameHeaderSize)
	header[0] = byte(t)
	// bytes 1-3 are padding
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return 0, err
	}
	if len(payload) == 0 {
		return 0, nil
	}
	return w.Write(payload)
}

// ReadFrame reads exactly one frame from r.
func ReadFrame(r io.Reader) (StreamType, []byte, error) {
	hdr := make([]byte, frameHeaderSize)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return 0, nil, err
	}
	t := StreamType(hdr[0])
	if t > 2 {
		return 0, nil, fmt.Errorf("%w: unknown stream type %d", ErrInvalidHeader, t)
	}
	size := binary.BigEndian.Uint32(hdr[4:8])
	if size > 0x7FFFFFFF {
		return 0, nil, fmt.Errorf("%w: size too large %d", ErrInvalidHeader, size)
	}
	if size == 0 {
		return t, nil, nil
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return t, payload, nil
}

// StdCopy is the equivalent of github.com/docker/cli/pkg/stdcopy.StdCopy:
// demultiplexes r into stdout and stderr writers. A non-blocking read
// of stdout is forwarded to dstOut; stderr to dstErr. Stdin frames on
// the read side are also forwarded to dstOut (legacy Docker behavior).
//
// Loop terminates cleanly on EOF. Any frame >2 (protocol extension) is
// silently skipped.
func StdCopy(dstOut, dstErr io.Writer, src io.Reader) (written int64, err error) {
	for {
		t, payload, err := ReadFrame(src)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return written, nil
			}
			return written, err
		}
		var dst io.Writer
		switch t {
		case StreamStdout, StreamStdin:
			dst = dstOut
		case StreamStderr:
			dst = dstErr
		default:
			// unknown stream type; skip
			continue
		}
		if len(payload) == 0 {
			continue
		}
		n, werr := dst.Write(payload)
		written += int64(n)
		if werr != nil {
			return written, werr
		}
	}
}

// StdCopySingleReader is like StdCopy but for the case when the source
// is an unframed raw stream (Tty=true). It just copies src to dst.
func StdCopySingleReader(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}
