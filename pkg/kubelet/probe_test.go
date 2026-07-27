package kubelet

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
