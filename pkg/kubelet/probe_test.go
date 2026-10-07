package kubelet

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	k8s "github.com/OpceanAI/Doki/pkg/k8s-types"
)

func TestPodRestartPolicyDefault(t *testing.T) {
	if got := podRestartPolicy(&k8s.Pod{}); got != restartAlways {
		t.Errorf("default policy = %q, want Always", got)
	}
	p := &k8s.Pod{}
	p.Spec.RestartPolicy = "OnFailure"
	if got := podRestartPolicy(p); got != restartOnFailure {
		t.Errorf("policy = %q, want OnFailure", got)
	}
	p.Spec.RestartPolicy = "bogus"
	if got := podRestartPolicy(p); got != restartAlways {
		t.Errorf("bogus policy = %q, want Always fallback", got)
	}
}

func TestShouldRestart(t *testing.T) {
	cases := []struct {
		policy string
		exit   int32
		want   bool
	}{
		{restartAlways, 0, true},
		{restartAlways, 1, true},
		{restartOnFailure, 0, false},
		{restartOnFailure, 7, true},
		{restartNever, 0, false},
		{restartNever, 1, false},
	}
	for _, c := range cases {
		if got := shouldRestart(c.policy, c.exit); got != c.want {
			t.Errorf("shouldRestart(%s, %d) = %v, want %v", c.policy, c.exit, got, c.want)
		}
	}
}

// Backoff grows with the restart count, so a crash loop does not restart every
// reconcile.
func TestRestartBackoff(t *testing.T) {
	k := &Kubelet{lastRestart: map[string]time.Time{}}
	// First restart is always allowed.
	if !k.restartBackoffElapsed("ns/pod", "c", 0) {
		t.Fatal("first restart should be allowed")
	}
	k.markRestart("ns/pod", "c")
	// Immediately after, with a higher count, backoff must block.
	if k.restartBackoffElapsed("ns/pod", "c", 3) {
		t.Error("restart during backoff window should be blocked")
	}
	// A different container is unaffected.
	if !k.restartBackoffElapsed("ns/pod", "other", 0) {
		t.Error("unrelated container should not be blocked")
	}
}

func TestResolveProbePort(t *testing.T) {
	c := k8s.Container{Ports: []k8s.ContainerPort{{Name: "http", ContainerPort: 8080}}}
	if got := resolveProbePort(c, k8s.IntOrString{IntVal: 9090}); got != 9090 {
		t.Errorf("int port = %d, want 9090", got)
	}
	if got := resolveProbePort(c, k8s.IntOrString{StrVal: "http"}); got != 8080 {
		t.Errorf("named port = %d, want 8080", got)
	}
	if got := resolveProbePort(c, k8s.IntOrString{StrVal: "5000"}); got != 5000 {
		t.Errorf("numeric string port = %d, want 5000", got)
	}
}

// A TCP probe succeeds against a live listener and fails against a dead port.
func TestTCPProbe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	_, _ = fmtSscan(portStr, &port)

	if !tcpProbe("127.0.0.1", port, time.Second) {
		t.Error("probe against live listener should succeed")
	}
	_ = ln.Close()
	if tcpProbe("127.0.0.1", port, 200*time.Millisecond) {
		t.Error("probe against closed port should fail")
	}
}

// An HTTP probe succeeds on 2xx and fails on 5xx.
func TestHTTPProbe(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	host, port := hostPort(t, okSrv.URL)
	if !httpProbe(context.Background(), "http", host, port, "/healthz", time.Second) {
		t.Error("2xx should be ready")
	}
	host, port = hostPort(t, failSrv.URL)
	if httpProbe(context.Background(), "http", host, port, "/", time.Second) {
		t.Error("5xx should be not ready")
	}
}

func fmtSscan(s string, p *int) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	*p = n
	return 1, nil
}

func hostPort(t *testing.T, u string) (string, int) {
	t.Helper()
	// u is like http://127.0.0.1:PORT
	hp := u[len("http://"):]
	host, portStr, err := net.SplitHostPort(hp)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	_, _ = fmtSscan(portStr, &port)
	return host, port
}

// Probe engine tests (liveness / startup / readiness)
// fakeCRI is a minimal RuntimeServiceClient for probe tests. It embeds the
// interface so only the methods under test need real implementations.
type fakeCRI struct {
	v1.RuntimeServiceClient

	mu       sync.Mutex
	ok       map[string]bool // exec command -> exit 0
	execRuns [][]string
	stopped  []string
	removed  []string
}

func newFakeCRI() *fakeCRI {
	return &fakeCRI{ok: map[string]bool{}}
}

func (f *fakeCRI) ExecSync(_ context.Context, req *v1.ExecSyncRequest, _ ...grpc.CallOption) (*v1.ExecSyncResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execRuns = append(f.execRuns, req.GetCmd())
	if f.ok[strings.Join(req.GetCmd(), " ")] {
		return &v1.ExecSyncResponse{ExitCode: 0}, nil
	}
	return &v1.ExecSyncResponse{ExitCode: 1}, nil
}

func (f *fakeCRI) StopContainer(_ context.Context, req *v1.StopContainerRequest, _ ...grpc.CallOption) (*v1.StopContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, req.GetContainerId())
	return &v1.StopContainerResponse{}, nil
}

func (f *fakeCRI) RemoveContainer(_ context.Context, req *v1.RemoveContainerRequest, _ ...grpc.CallOption) (*v1.RemoveContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, req.GetContainerId())
	return &v1.RemoveContainerResponse{}, nil
}

func (f *fakeCRI) runs() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.execRuns))
	copy(out, f.execRuns)
	return out
}

func (f *fakeCRI) kills() (stopped, removed []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.stopped...), append([]string{}, f.removed...)
}

func newProbeKubelet(cri v1.RuntimeServiceClient) *Kubelet {
	return &Kubelet{
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		probes:       make(map[string]*probeTracker),
		restartBumps: make(map[string]int32),
		lastRestart:  make(map[string]time.Time),
		criClient:    cri,
	}
}

func probePod() *k8s.Pod {
	return &k8s.Pod{
		ObjectMeta: k8s.ObjectMeta{Name: "p", Namespace: "ns", UID: "uid-1"},
	}
}

// A failed liveness probe kills the container: it must be stopped and removed
// through CRI so the reconcile loop restarts it (K15).
func TestLivenessFailureKillsContainer(t *testing.T) {
	fake := newFakeCRI()
	k := newProbeKubelet(fake)
	pod := probePod()
	c := k8s.Container{
		Name: "app",
		LivenessProbe: &k8s.Probe{
			ProbeHandler:     k8s.ProbeHandler{Exec: &k8s.ExecAction{Command: []string{"health"}}},
			TimeoutSeconds:   1,
			FailureThreshold: 1,
		},
	}

	ready := k.containerHealth(context.Background(), pod, c, "cid-live-1", time.Now().UnixNano())
	if ready {
		t.Error("container with failing liveness probe must not be ready")
	}
	stopped, removed := fake.kills()
	if len(stopped) != 1 || stopped[0] != "cid-live-1" {
		t.Errorf("stopped containers = %v, want [cid-live-1]", stopped)
	}
	if len(removed) != 1 || removed[0] != "cid-live-1" {
		t.Errorf("removed containers = %v, want [cid-live-1]", removed)
	}

	// A passing liveness probe must not kill anything.
	fake.ok["health"] = true
	ready = k.containerHealth(context.Background(), pod, c, "cid-live-2", time.Now().UnixNano())
	if !ready {
		t.Error("container with passing liveness probe should be ready (no readiness probe)")
	}
	if stopped, removed := fake.kills(); len(stopped) != 1 || len(removed) != 1 {
		t.Errorf("healthy probe killed containers: stopped=%v removed=%v", stopped, removed)
	}
}

// A startup probe gates both readiness and liveness: until it succeeds the
// container is not ready and liveness must not kill it; once it succeeds the
// readiness probe decides Ready (K16).
func TestStartupProbeGatesReadiness(t *testing.T) {
	fake := newFakeCRI()
	fake.ok["ready"] = true
	fake.ok["live"] = true
	k := newProbeKubelet(fake)
	pod := probePod()
	c := k8s.Container{
		Name: "app",
		StartupProbe: &k8s.Probe{
			ProbeHandler:     k8s.ProbeHandler{Exec: &k8s.ExecAction{Command: []string{"startup"}}},
			TimeoutSeconds:   1,
			FailureThreshold: 100,
			SuccessThreshold: 1,
		},
		LivenessProbe: &k8s.Probe{
			ProbeHandler:     k8s.ProbeHandler{Exec: &k8s.ExecAction{Command: []string{"live"}}},
			TimeoutSeconds:   1,
			FailureThreshold: 1,
		},
		ReadinessProbe: &k8s.Probe{
			ProbeHandler:     k8s.ProbeHandler{Exec: &k8s.ExecAction{Command: []string{"ready"}}},
			TimeoutSeconds:   1,
			SuccessThreshold: 1,
		},
	}

	// 1) Startup still failing: not ready, and only the startup probe ran.
	ready := k.containerHealth(context.Background(), pod, c, "cid-start-1", time.Now().UnixNano())
	if ready {
		t.Error("container must not be ready while the startup probe has not succeeded")
	}
	runs := fake.runs()
	if len(runs) != 1 || strings.Join(runs[0], " ") != "startup" {
		t.Fatalf("probe runs = %v, want only the startup probe", runs)
	}
	if stopped, removed := fake.kills(); len(stopped) != 0 || len(removed) != 0 {
		t.Fatalf("startup failure must not kill the container: stopped=%v removed=%v", stopped, removed)
	}

	// 2) Startup now succeeds: liveness and readiness run and the container
	// becomes ready.
	fake.ok["startup"] = true
	ready = k.containerHealth(context.Background(), pod, c, "cid-start-1", time.Now().UnixNano())
	if !ready {
		t.Error("container should be ready once startup, liveness and readiness probes pass")
	}
	runs = fake.runs()
	if len(runs) != 4 { // startup, startup (pass), live, ready
		t.Errorf("probe runs = %v, want 4 attempts", runs)
	}
	last := map[string]bool{}
	for _, r := range runs {
		last[strings.Join(r, " ")] = true
	}
	if !last["live"] || !last["ready"] {
		t.Errorf("liveness/readiness must run after startup succeeds, runs=%v", runs)
	}
}

// Startup probe failures beyond failureThreshold restart the container just
// like liveness failures.
func TestStartupProbeFailureThresholdKillsContainer(t *testing.T) {
	fake := newFakeCRI()
	k := newProbeKubelet(fake)
	pod := probePod()
	c := k8s.Container{
		Name: "app",
		StartupProbe: &k8s.Probe{
			ProbeHandler:     k8s.ProbeHandler{Exec: &k8s.ExecAction{Command: []string{"startup"}}},
			TimeoutSeconds:   1,
			FailureThreshold: 2,
		},
	}

	if ready := k.containerHealth(context.Background(), pod, c, "cid-sf-1", time.Now().UnixNano()); ready {
		t.Fatal("first startup failure must not make the container ready")
	}
	if stopped, _ := fake.kills(); len(stopped) != 0 {
		t.Fatalf("one startup failure must not kill yet: %v", stopped)
	}
	if ready := k.containerHealth(context.Background(), pod, c, "cid-sf-1", time.Now().UnixNano()); ready {
		t.Fatal("second startup failure must not make the container ready")
	}
	if stopped, removed := fake.kills(); len(stopped) != 1 || len(removed) != 1 {
		t.Fatalf("failureThreshold startup failures must kill the container: stopped=%v removed=%v", stopped, removed)
	}
}
