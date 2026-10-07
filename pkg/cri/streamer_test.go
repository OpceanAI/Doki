package cri

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
)

// PortForward must report codes.Unimplemented instead of handing out a
// streaming URL that later fails at dial time with HTTP 501.
func TestPortForwardUnimplemented(t *testing.T) {
	s := NewCRIServer(nil)

	// Missing arguments still fail with InvalidArgument first.
	if _, err := s.PortForward(context.Background(), &v1.PortForwardRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty request: code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}
	if _, err := s.PortForward(context.Background(), &v1.PortForwardRequest{
		PodSandboxId: "sandbox-1",
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing ports: code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}

	// A well-formed request must fail with Unimplemented and no URL.
	resp, err := s.PortForward(context.Background(), &v1.PortForwardRequest{
		PodSandboxId: "sandbox-1",
		Port:         []int32{8080},
	})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("code = %v, want Unimplemented (err=%v)", status.Code(err), err)
	}
	if resp != nil && resp.GetUrl() != "" {
		t.Fatalf("PortForward must not return a URL, got %q", resp.GetUrl())
	}
}

// The streaming resize path must apply a real TIOCSWINSZ to the container's
// pty, not silently drop the frame.
func TestResizeTTYReal(t *testing.T) {
	rt := dokiruntime.NewRuntime(t.TempDir(), nil, dokiruntime.WithMode(dokiruntime.ModeNative))

	cfg := &dokiruntime.Config{
		ID:   "cri-tty-resize",
		Args: []string{"sh", "-c", "sleep 30"},
		Tty:  true,
	}
	if _, err := rt.Create(cfg); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.Start(cfg.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = rt.Stop(cfg.ID, 1)
		_ = rt.Delete(cfg.ID, true)
	}()

	sess, err := rt.AttachStreams(cfg.ID)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	defer sess.Detach()

	// Kubernetes sends Width=columns, Height=rows.
	applyResize(sess, resizeMsg{Width: 100, Height: 50})

	deadline := time.Now().Add(2 * time.Second)
	var rows, cols uint16
	for {
		rows, cols, err = rt.TTYSize(cfg.ID)
		if err != nil {
			t.Fatalf("tty size: %v", err)
		}
		if rows == 50 && cols == 100 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pty size = %dx%d (rows x cols), want 50x100", rows, cols)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// A second resize must also land.
	applyResize(sess, resizeMsg{Width: 80, Height: 24})
	if rows, cols, err = rt.TTYSize(cfg.ID); err != nil {
		t.Fatalf("tty size: %v", err)
	}
	if rows != 24 || cols != 80 {
		t.Fatalf("pty size = %dx%d (rows x cols), want 24x80", rows, cols)
	}
}
