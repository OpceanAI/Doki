package kubelet

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

// Probe state machine
// probeTracker holds the per-container probe state machine: consecutive
// success/failure counters and last-run timestamps for the startup, liveness
// and readiness probes, following Kubernetes semantics (periodSeconds,
// timeoutSeconds, successThreshold, failureThreshold).
type probeTracker struct {
	mu sync.Mutex

	// lastContainerID detects container recreation: a new container always
	// starts with fresh probe state.
	lastContainerID string

	startupDone      bool
	startupSuccesses int32
	startupFailures  int32
	lastStartupRun   time.Time

	livenessFailures int32
	lastLivenessRun  time.Time

	ready          bool
	readySuccesses int32
	readyFailures  int32
	lastReadyRun   time.Time
}

// reset clears all probe state for a (re)created container. Callers must hold
// tr.mu; the mutex itself is never overwritten.
func (tr *probeTracker) reset(containerID string) {
	tr.lastContainerID = containerID
	tr.startupDone = false
	tr.startupSuccesses = 0
	tr.startupFailures = 0
	tr.lastStartupRun = time.Time{}
	tr.livenessFailures = 0
	tr.lastLivenessRun = time.Time{}
	tr.ready = false
	tr.readySuccesses = 0
	tr.readyFailures = 0
	tr.lastReadyRun = time.Time{}
}

// probeThreshold returns a probe threshold with the Kubernetes default applied
// (0 means "unset"). successThreshold defaults to 1, failureThreshold to 3.
func probeThreshold(v, def int32) int32 {
	if v <= 0 {
		return def
	}
	return v
}

// probeDue reports whether a probe may run again: the initial delay has
// elapsed since the container started and periodSeconds has elapsed since the
// last attempt. A zero period means "on every pass".
func probeDue(last time.Time, p *k8s.Probe, startedAtNanos int64) bool {
	if p == nil {
		return false
	}
	if p.InitialDelaySeconds > 0 && startedAtNanos > 0 {
		if time.Since(time.Unix(0, startedAtNanos)) < time.Duration(p.InitialDelaySeconds)*time.Second {
			return false
		}
	}
	if last.IsZero() {
		return true
	}
	period := time.Duration(p.PeriodSeconds) * time.Second
	if period <= 0 {
		return true
	}
	return time.Since(last) >= period
}

// tracker returns (creating it if needed) the probe state of one container.
func (k *Kubelet) tracker(podKey, container string) *probeTracker {
	key := podKey + "/" + container
	k.probeMu.Lock()
	defer k.probeMu.Unlock()
	if k.probes == nil {
		k.probes = make(map[string]*probeTracker)
	}
	tr, ok := k.probes[key]
	if !ok {
		tr = &probeTracker{}
		k.probes[key] = tr
	}
	return tr
}

// clearProbeState drops all probe state belonging to a deleted pod.
func (k *Kubelet) clearProbeState(podKey string) {
	k.probeMu.Lock()
	defer k.probeMu.Unlock()
	prefix := podKey + "/"
	for key := range k.probes {
		if strings.HasPrefix(key, prefix) {
			delete(k.probes, key)
		}
	}
}

// bumpRestart records a restart caused by a failed liveness/startup probe so
// the next reconcile reports it in RestartCount.
func (k *Kubelet) bumpRestart(podKey, container string) {
	k.restartBumpMu.Lock()
	k.restartBumps[podKey+"/"+container]++
	k.restartBumpMu.Unlock()
}

// takeRestartBump consumes the pending probe-restart count of a container.
func (k *Kubelet) takeRestartBump(podKey, container string) int32 {
	key := podKey + "/" + container
	k.restartBumpMu.Lock()
	defer k.restartBumpMu.Unlock()
	n := k.restartBumps[key]
	delete(k.restartBumps, key)
	return n
}

// containerHealth runs one probe pass for a running container and reports
// whether it is ready:
//
//   - startupProbe gates everything: while it has not yet succeeded, liveness
//     and readiness are not evaluated and the container is not ready;
//   - livenessProbe failure (failureThreshold consecutive failures) kills and
//     removes the container so the reconcile loop restarts it;
//   - readinessProbe drives the Ready condition via successThreshold /
//     failureThreshold counters.
//
// Each probe runs at most once per periodSeconds (and not before
// initialDelaySeconds after container start).
func (k *Kubelet) containerHealth(ctx context.Context, pod *k8s.Pod, c k8s.Container, containerID string, startedAtNanos int64) bool {
	podKey := podKey(pod)
	tr := k.tracker(podKey, c.Name)

	tr.mu.Lock()
	defer tr.mu.Unlock()

	// A recreated container always starts with fresh probe state.
	if tr.lastContainerID != containerID {
		tr.reset(containerID)
	}

	// startupProbe gates liveness and readiness until it has succeeded.
	if c.StartupProbe != nil && !tr.startupDone {
		if probeDue(tr.lastStartupRun, c.StartupProbe, startedAtNanos) {
			tr.lastStartupRun = time.Now()
			if k.probeOnce(ctx, pod, c, c.StartupProbe, containerID) {
				tr.startupSuccesses++
				tr.startupFailures = 0
				if tr.startupSuccesses >= probeThreshold(c.StartupProbe.SuccessThreshold, 1) {
					tr.startupDone = true
				}
			} else {
				tr.startupFailures++
				tr.startupSuccesses = 0
				if tr.startupFailures >= probeThreshold(c.StartupProbe.FailureThreshold, 3) {
					k.killUnhealthyContainer(ctx, podKey, c.Name, containerID, "StartupProbeFailed")
					tr.startupFailures = 0
					return false
				}
			}
		}
		if !tr.startupDone {
			return false
		}
	}

	// livenessProbe: consecutive failures beyond failureThreshold restart the
	// container (Kubernetes does this regardless of restartPolicy).
	if c.LivenessProbe != nil && probeDue(tr.lastLivenessRun, c.LivenessProbe, startedAtNanos) {
		tr.lastLivenessRun = time.Now()
		if k.probeOnce(ctx, pod, c, c.LivenessProbe, containerID) {
			tr.livenessFailures = 0
		} else {
			tr.livenessFailures++
			if tr.livenessFailures >= probeThreshold(c.LivenessProbe.FailureThreshold, 3) {
				k.killUnhealthyContainer(ctx, podKey, c.Name, containerID, "LivenessProbeFailed")
				tr.livenessFailures = 0
				tr.ready = false
				return false
			}
		}
	}

	// readinessProbe: no probe means "ready once running" (Kubernetes default).
	if c.ReadinessProbe == nil {
		tr.ready = true
		return tr.ready
	}
	if probeDue(tr.lastReadyRun, c.ReadinessProbe, startedAtNanos) {
		tr.lastReadyRun = time.Now()
		if k.probeOnce(ctx, pod, c, c.ReadinessProbe, containerID) {
			tr.readySuccesses++
			tr.readyFailures = 0
			if tr.readySuccesses >= probeThreshold(c.ReadinessProbe.SuccessThreshold, 1) {
				tr.ready = true
			}
		} else {
			tr.readyFailures++
			tr.readySuccesses = 0
			if tr.readyFailures >= probeThreshold(c.ReadinessProbe.FailureThreshold, 3) {
				tr.ready = false
			}
		}
	}
	return tr.ready
}

// killUnhealthyContainer stops and removes a container whose startup or
// liveness probe failed. The reconcile loop recreates it (the same path used
// for restartPolicy handling); the restart is counted so RestartCount and the
// restart backoff reflect it.
func (k *Kubelet) killUnhealthyContainer(ctx context.Context, podKey, name, containerID, reason string) {
	if k.criClient == nil {
		return
	}
	k.logger.Warn("probe failed, restarting container",
		"pod", podKey, "container", name, "containerID", containerID, "reason", reason)
	k.bumpRestart(podKey, name)
	k.markRestart(podKey, name)
	if _, err := k.criClient.StopContainer(ctx, &v1.StopContainerRequest{ContainerId: containerID, Timeout: 10}); err != nil {
		k.logger.Warn("cri: stop unhealthy container", "pod", podKey, "container", name, "error", err)
	}
	if _, err := k.criClient.RemoveContainer(ctx, &v1.RemoveContainerRequest{ContainerId: containerID}); err != nil {
		k.logger.Warn("cri: remove unhealthy container", "pod", podKey, "container", name, "error", err)
	}
}

// probeOnce runs a single probe attempt against the container.
func (k *Kubelet) probeOnce(ctx context.Context, pod *k8s.Pod, c k8s.Container, probe *k8s.Probe, containerID string) bool {
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
		return grpcProbe(pctx, pod, probe.GRPC, timeout)
	default:
		// A probe with no handler is not a probe; treat it as passing rather
		// than blocking the container forever.
		return true
	}
}

// grpcProbe performs a real gRPC health check (grpc.health.v1.Health/Check)
// against the container. A container is considered healthy only when the
// health service answers SERVING; connection failures and unknown services are
// reported as failures instead of the previous unconditional "ready".
//
// The two health messages are encoded by hand (healthProtoCodec) so the probe
// needs no generated stubs and no vendor/ changes.
func grpcProbe(ctx context.Context, pod *k8s.Pod, action *k8s.GRPCAction, timeout time.Duration) bool {
	port := int(action.Port)
	if port <= 0 {
		return false
	}
	host := pod.Status.PodIP
	if host == "" {
		host = "127.0.0.1"
	}
	conn, err := grpc.NewClient(net.JoinHostPort(host, strconv.Itoa(port)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	service := ""
	if action.Service != nil {
		service = *action.Service
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var resp healthCheckResponse
	err = conn.Invoke(pctx, "/grpc.health.v1.Health/Check",
		&healthCheckRequest{Service: service}, &resp,
		grpc.ForceCodec(healthProtoCodec{}))
	if err != nil {
		return false
	}
	return resp.Status == healthStatusServing
}

// healthStatusServing is grpc.health.v1.HealthCheckResponse.SERVING.
const healthStatusServing = 1

// healthCheckRequest is grpc.health.v1.HealthCheckRequest: field 1 is the
// service name (length-delimited string). healthCheckResponse is
// HealthCheckResponse: field 1 is the status enum (varint).
type healthCheckRequest struct{ Service string }

func (h *healthCheckRequest) Marshal() ([]byte, error) {
	if h.Service == "" {
		return []byte{}, nil
	}
	buf := make([]byte, 0, len(h.Service)+2)
	buf = append(buf, 0x0a, byte(len(h.Service)))
	return append(buf, h.Service...), nil
}

func (h *healthCheckRequest) Unmarshal(b []byte) error {
	h.Service = string(b)
	return nil
}

type healthCheckResponse struct {
	Status int32
}

func (h *healthCheckResponse) Marshal() ([]byte, error) { return []byte{}, nil }

func (h *healthCheckResponse) Unmarshal(b []byte) error {
	for len(b) > 0 {
		key := b[0]
		b = b[1:]
		if key&0x07 != 0 { // only varint fields are understood
			return fmt.Errorf("grpc health: unexpected wire type %d", key&0x07)
		}
		var v uint64
		shift := 0
		for {
			if len(b) == 0 {
				return fmt.Errorf("grpc health: truncated varint")
			}
			c := b[0]
			b = b[1:]
			v |= uint64(c&0x7f) << shift
			if c&0x80 == 0 {
				break
			}
			shift += 7
		}
		h.Status = int32(v)
	}
	return nil
}

// healthProtoCodec marshals the hand-rolled health messages with the "proto"
// wire codec name so grpc accepts them on the standard content subtype.
type healthProtoCodec struct{}

func (healthProtoCodec) Name() string { return "proto" }

func (healthProtoCodec) Marshal(v any) ([]byte, error) {
	m, ok := v.(interface{ Marshal() ([]byte, error) })
	if !ok {
		return nil, fmt.Errorf("grpc health: unsupported message %T", v)
	}
	return m.Marshal()
}

func (healthProtoCodec) Unmarshal(data []byte, v any) error {
	m, ok := v.(interface{ Unmarshal([]byte) error })
	if !ok {
		return fmt.Errorf("grpc health: unsupported message %T", v)
	}
	return m.Unmarshal(data)
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
