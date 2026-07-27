package kubelet

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	k8s "github.com/OpceanAI/Doki/pkg/k8s-types"
)

// Restart policies. PodSpec.RestartPolicy is a bare string; these are the
// values Kubernetes uses. The package has no typed constants for them.
const (
	restartAlways    = "Always"
	restartOnFailure = "OnFailure"
	restartNever     = "Never"
)

// podRestartPolicy returns the pod's effective restart policy, defaulting to
// Always (the Kubernetes default for a Pod).
func podRestartPolicy(pod *k8s.Pod) string {
	switch pod.Spec.RestartPolicy {
	case restartAlways, restartOnFailure, restartNever:
		return pod.Spec.RestartPolicy
	default:
		return restartAlways
	}
}

// shouldRestart reports whether an exited container should be restarted under
// the given policy and exit code (K14).
func shouldRestart(policy string, exitCode int32) bool {
	switch policy {
	case restartAlways:
		return true
	case restartOnFailure:
		return exitCode != 0
	default: // Never
		return false
	}
}

// restartBackoffElapsed enforces an exponential backoff (capped at 5m) between
// restarts of the same container so a crash loop does not spin the reconcile
// loop. The delay grows with the restart count.
func (k *Kubelet) restartBackoffElapsed(podKey, container string, restartCount int32) bool {
	key := podKey + "/" + container
	k.restartMu.Lock()
	defer k.restartMu.Unlock()
	last, ok := k.lastRestart[key]
	if !ok {
		return true
	}
	backoff := time.Duration(1<<min32(restartCount, 8)) * time.Second // 1s..256s
	if backoff > 5*time.Minute {
		backoff = 5 * time.Minute
	}
	return time.Since(last) >= backoff
}

// markRestart records the time of a container restart for backoff accounting.
func (k *Kubelet) markRestart(podKey, container string) {
	k.restartMu.Lock()
	k.lastRestart[podKey+"/"+container] = time.Now()
	k.restartMu.Unlock()
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

// containerReady evaluates a container's readiness (K13). With no readiness
// probe, a running container is ready (Kubernetes default). With a probe, it is
// ready only once the probe passes — but not before InitialDelaySeconds has
// elapsed since the container started.
func (k *Kubelet) containerReady(ctx context.Context, pod *k8s.Pod, c k8s.Container, containerID string, startedAtNanos int64) bool {
	probe := c.ReadinessProbe
	if probe == nil {
		return true
	}
	if probe.InitialDelaySeconds > 0 && startedAtNanos > 0 {
		started := time.Unix(0, startedAtNanos)
		if time.Since(started) < time.Duration(probe.InitialDelaySeconds)*time.Second {
			return false
		}
	}

	timeout := time.Duration(probe.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Second
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch {
	case probe.Exec != nil:
		return k.execProbe(pctx, containerID, probe.Exec.Command, timeout)
	case probe.TCPSocket != nil:
		return tcpProbe(k.probeHost(pod, probe.TCPSocket.Host), resolveProbePort(c, probe.TCPSocket.Port), timeout)
	case probe.HTTPGet != nil:
		return httpProbe(pctx, k.probeScheme(probe.HTTPGet.Scheme), k.probeHost(pod, probe.HTTPGet.Host),
			resolveProbePort(c, probe.HTTPGet.Port), probe.HTTPGet.Path, timeout)
	case probe.GRPC != nil:
		// gRPC health probing is out of scope; treat as ready so it does not
		// block traffic (do not silently report a false negative).
		return true
	default:
		return true
	}
}

// execProbe runs an exec readiness probe inside the container via CRI ExecSync.
// A zero exit code means ready.
func (k *Kubelet) execProbe(ctx context.Context, containerID string, cmd []string, timeout time.Duration) bool {
	if len(cmd) == 0 {
		return false
	}
	resp, err := k.criClient.ExecSync(ctx, &v1.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     int64(timeout / time.Second),
	})
	if err != nil {
		return false
	}
	return resp.GetExitCode() == 0
}

// probeHost returns the host to probe: an explicit host if set, otherwise the
// pod IP.
func (k *Kubelet) probeHost(pod *k8s.Pod, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if pod.Status.PodIP != "" {
		return pod.Status.PodIP
	}
	return "127.0.0.1"
}

func (k *Kubelet) probeScheme(scheme string) string {
	if scheme == "HTTPS" {
		return "https"
	}
	return "http"
}

// resolveProbePort resolves a probe port that may be an integer or a named
// container port.
func resolveProbePort(c k8s.Container, port k8s.IntOrString) int {
	if port.IntVal > 0 {
		return int(port.IntVal)
	}
	if port.StrVal != "" {
		if n, err := strconv.Atoi(port.StrVal); err == nil {
			return n
		}
		for _, cp := range c.Ports {
			if cp.Name == port.StrVal {
				return int(cp.ContainerPort)
			}
		}
	}
	return 0
}

// tcpProbe succeeds if a TCP connection to host:port can be established.
func tcpProbe(host string, port int, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// httpProbe succeeds on any 2xx/3xx response from the probe URL.
func httpProbe(ctx context.Context, scheme, host string, port int, path string, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	if path == "" {
		path = "/"
	}
	url := fmt.Sprintf("%s://%s%s", scheme, net.JoinHostPort(host, strconv.Itoa(port)), path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}
