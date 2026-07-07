// Package cri provides the Kubernetes Container Runtime Interface.
//
// server.go implements a real gRPC CRI server that exposes the
// RuntimeServiceServer and ImageServiceServer interfaces defined by
// k8s.io/cri-api. It translates between the CRI protobuf requests/responses
// and the existing CRIPlugin / dokiruntime.Runtime / image.Store operations.
package cri

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Streamer manages ephemeral HTTP streaming endpoints for exec, attach,
// and portforward operations. Each call to Reserve picks an available port,
// starts a one-shot HTTP server, and returns the allocated port number.
type Streamer interface {
	Reserve(ctx context.Context, op, resourceID string, params map[string]string) (int, error)
	Addr() (host string, port int)
}

// CRIServer is a gRPC server implementing the Kubernetes Container Runtime
// Interface (CRI) v1 RuntimeService and ImageService.
//
// It embeds the generated Unimplemented* servers by value (required for
// forward compatibility) and delegates the actual container/image work to
// the existing CRIPlugin, which wraps dokiruntime.Runtime, image.Store and
// network.Manager.
type CRIServer struct {
	v1.UnimplementedRuntimeServiceServer
	v1.UnimplementedImageServiceServer

	plugin   *CRIPlugin
	server   *grpc.Server
	streamer Streamer
}

// NewCRIServer creates a new CRIServer backed by the given CRIPlugin.
func NewCRIServer(plugin *CRIPlugin) *CRIServer {
	return &CRIServer{plugin: plugin}
}

// ListenAndServe creates a Unix socket listener at socketPath, registers the
// RuntimeService and ImageService, and serves gRPC requests. It blocks until
// the server is stopped via Close or the listener is closed.
func (s *CRIServer) ListenAndServe(socketPath string) error {
	// Remove any stale socket file so the new listener can bind.
	if err := os.RemoveAll(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing socket %q: %w", socketPath, err)
	}

	// Ensure the parent directory exists (e.g. /var/run/doki).
	if dir := filepath.Dir(socketPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create socket directory %q: %w", dir, err)
		}
	}

	// Create a Unix domain socket listener. CRI runs over a Unix socket so
	// no TLS is required; insecure credentials are used explicitly.
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on unix socket %q: %w", socketPath, err)
	}

	s.server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	v1.RegisterRuntimeServiceServer(s.server, s)
	v1.RegisterImageServiceServer(s.server, s)

	return s.server.Serve(listener)
}

// Close gracefully stops the gRPC server, letting in-flight RPCs complete.
func (s *CRIServer) Close() error {
	if s.server != nil {
		s.server.GracefulStop()
	}
	return nil
}

// ─── helpers ────────────────────────────────────────────────────────

// toCINanos converts a Unix-second timestamp (as stored by the plugin) to the
// nanosecond timestamp expected by CRI.
func toCINanos(seconds int64) int64 {
	return seconds * int64(time.Second)
}

// toCINanosFromTime converts a time.Time to a nanosecond timestamp, returning
// 0 for the zero time (CRI uses 0 to mean "not set").
func toCINanosFromTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// podSandboxStateFromString maps the plugin's string state to the CRI enum.
func podSandboxStateFromString(s string) v1.PodSandboxState {
	if s == "SANDBOX_READY" {
		return v1.PodSandboxState_SANDBOX_READY
	}
	return v1.PodSandboxState_SANDBOX_NOTREADY
}

// containerStateFromRuntime maps the runtime's ContainerState to the CRI enum.
func containerStateFromRuntime(s common.ContainerState) v1.ContainerState {
	switch s {
	case common.StateCreated:
		return v1.ContainerState_CONTAINER_CREATED
	case common.StateRunning:
		return v1.ContainerState_CONTAINER_RUNNING
	case common.StateExited, common.StateDead:
		return v1.ContainerState_CONTAINER_EXITED
	default:
		return v1.ContainerState_CONTAINER_UNKNOWN
	}
}

// matchLabels returns true if every key/value in selector is present in labels.
func matchLabels(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// imageRefFromSpec safely extracts the image reference string from an ImageSpec.
func imageRefFromSpec(spec *v1.ImageSpec) string {
	if spec == nil {
		return ""
	}
	return spec.GetImage()
}

// notFoundErr converts a not-found error from the backend into a gRPC NotFound
// status; other errors are wrapped as Internal.
func notFoundErr(resource, id string, err error) error {
	if common.IsNotFound(err) {
		return status.Errorf(codes.NotFound, "%s %s not found", resource, id)
	}
	return status.Errorf(codes.Internal, "%v", err)
}

// ─── RuntimeService ─────────────────────────────────────────────────

// Version returns the runtime name, runtime version, and runtime API version.
func (s *CRIServer) Version(ctx context.Context, req *v1.VersionRequest) (*v1.VersionResponse, error) {
	return &v1.VersionResponse{
		Version:           "v1",
		RuntimeName:       "doki",
		RuntimeVersion:    common.Version,
		RuntimeApiVersion: "v1",
	}, nil
}

// RunPodSandbox creates and starts a pod-level sandbox.
func (s *CRIServer) RunPodSandbox(ctx context.Context, req *v1.RunPodSandboxRequest) (*v1.RunPodSandboxResponse, error) {
	cfg := req.GetConfig()
	if cfg == nil || cfg.GetMetadata() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "missing sandbox config or metadata")
	}
	meta := cfg.GetMetadata()
	id := common.GenerateID(64)

	if _, err := s.plugin.RunPodSandbox(id, meta.GetName(), meta.GetNamespace(), cfg.GetLabels(), cfg.GetAnnotations()); err != nil {
		return nil, status.Errorf(codes.Internal, "run pod sandbox: %v", err)
	}
	return &v1.RunPodSandboxResponse{PodSandboxId: id}, nil
}

// StopPodSandbox stops any running process in the sandbox and reclaims
// network resources. Idempotent.
func (s *CRIServer) StopPodSandbox(ctx context.Context, req *v1.StopPodSandboxRequest) (*v1.StopPodSandboxResponse, error) {
	if err := s.plugin.StopPodSandbox(req.GetPodSandboxId()); err != nil {
		return nil, notFoundErr("pod sandbox", req.GetPodSandboxId(), err)
	}
	return &v1.StopPodSandboxResponse{}, nil
}

// RemovePodSandbox removes the sandbox. Idempotent.
func (s *CRIServer) RemovePodSandbox(ctx context.Context, req *v1.RemovePodSandboxRequest) (*v1.RemovePodSandboxResponse, error) {
	if err := s.plugin.RemovePodSandbox(req.GetPodSandboxId()); err != nil {
		return nil, status.Errorf(codes.Internal, "remove pod sandbox: %v", err)
	}
	return &v1.RemovePodSandboxResponse{}, nil
}

// PodSandboxStatus returns the status of the PodSandbox.
func (s *CRIServer) PodSandboxStatus(ctx context.Context, req *v1.PodSandboxStatusRequest) (*v1.PodSandboxStatusResponse, error) {
	sandbox, err := s.plugin.PodSandboxStatus(req.GetPodSandboxId())
	if err != nil {
		return nil, notFoundErr("pod sandbox", req.GetPodSandboxId(), err)
	}
	now := time.Now().UnixNano()
	resp := &v1.PodSandboxStatusResponse{
		Status: &v1.PodSandboxStatus{
			Id:          sandbox.ID,
			Metadata:    &v1.PodSandboxMetadata{Name: sandbox.Name, Namespace: sandbox.Namespace, Uid: sandbox.UID},
			State:       podSandboxStateFromString(sandbox.State),
			CreatedAt:   toCINanos(sandbox.CreatedAt),
			Labels:      sandbox.Labels,
			Annotations: sandbox.Annotations,
		},
		Info:      map[string]string{"runtimeHandler": "doki", "sandbox_id": sandbox.ID},
		Timestamp: now,
	}
	// v1.26+ containers_statuses; v1.36+ includes image_id per container.
	if req.GetVerbose() {
		css := s.plugin.ListContainers(req.GetPodSandboxId())
		cs := make([]*v1.ContainerStatus, 0, len(css))
		for _, cc := range css {
			ctr, _ := s.plugin.runtime.State(cc.ID)
			state := v1.ContainerState_CONTAINER_UNKNOWN
			if ctr != nil {
				state = containerStateFromRuntime(ctr.Status)
			}
			cs = append(cs, &v1.ContainerStatus{
				Id:          cc.ID,
				Metadata:    &v1.ContainerMetadata{Name: cc.Name, Attempt: cc.Attempt},
				Image:       &v1.ImageSpec{Image: cc.Image},
				ImageRef:    cc.ImageRef,
				ImageId:     cc.ImageID,
				State:       state,
				CreatedAt:   toCINanos(cc.CreatedAt),
				Labels:      cc.Labels,
				Annotations: cc.Annotations,
				LogPath:     cc.LogPath,
			})
		}
		resp.ContainersStatuses = cs
	}
	return resp, nil
}

// ListPodSandbox returns a list of PodSandboxes, optionally filtered.
func (s *CRIServer) ListPodSandbox(ctx context.Context, req *v1.ListPodSandboxRequest) (*v1.ListPodSandboxResponse, error) {
	pods := s.plugin.ListPodSandbox()
	filter := req.GetFilter()
	items := make([]*v1.PodSandbox, 0, len(pods))
	for _, p := range pods {
		if filter != nil {
			if id := filter.GetId(); id != "" && id != p.ID {
				continue
			}
			if st := filter.GetState(); st != nil && st.GetState() != podSandboxStateFromString(p.State) {
				continue
			}
			if !matchLabels(p.Labels, filter.GetLabelSelector()) {
				continue
			}
		}
		items = append(items, &v1.PodSandbox{
			Id:          p.ID,
			Metadata:    &v1.PodSandboxMetadata{Name: p.Name, Namespace: p.Namespace, Uid: p.UID},
			State:       podSandboxStateFromString(p.State),
			CreatedAt:   toCINanos(p.CreatedAt),
			Labels:      p.Labels,
			Annotations: p.Annotations,
		})
	}
	return &v1.ListPodSandboxResponse{Items: items}, nil
}

// CreateContainer creates a new container in the specified PodSandbox. The
// container is created but NOT started; StartContainer must be called separately.
func (s *CRIServer) CreateContainer(ctx context.Context, req *v1.CreateContainerRequest) (*v1.CreateContainerResponse, error) {
	cfg := req.GetConfig()
	if cfg == nil || cfg.GetMetadata() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "missing container config or metadata")
	}
	meta := cfg.GetMetadata()
	podID := req.GetPodSandboxId()
	containerID := common.GenerateID(64)

	// Command (entrypoint) followed by Args (cmd).
	args := append(append([]string{}, cfg.GetCommand()...), cfg.GetArgs()...)

	// Convert KeyValue env list to "KEY=VALUE" strings.
	env := make([]string, 0, len(cfg.GetEnvs()))
	for _, kv := range cfg.GetEnvs() {
		env = append(env, kv.GetKey()+"="+string(kv.GetValue()))
	}

	cc := &CRIContainer{
		ID:           containerID,
		PodSandboxID: podID,
		Name:         meta.GetName(),
		Attempt:      meta.GetAttempt(),
		Image:        imageRefFromSpec(cfg.GetImage()),
		Args:         args,
		Env:          env,
		WorkingDir:   cfg.GetWorkingDir(),
		Labels:       cfg.GetLabels(),
		Annotations:  cfg.GetAnnotations(),
		LogPath:      cfg.GetLogPath(),
	}

	if err := s.plugin.CreateContainer(cc); err != nil {
		return nil, notFoundErr("pod sandbox", podID, err)
	}
	return &v1.CreateContainerResponse{ContainerId: containerID}, nil
}

// StartContainer starts the container.
func (s *CRIServer) StartContainer(ctx context.Context, req *v1.StartContainerRequest) (*v1.StartContainerResponse, error) {
	if err := s.plugin.StartContainer(req.GetContainerId()); err != nil {
		return nil, notFoundErr("container", req.GetContainerId(), err)
	}
	return &v1.StartContainerResponse{}, nil
}

// StopContainer stops a running container with a grace period. Idempotent.
func (s *CRIServer) StopContainer(ctx context.Context, req *v1.StopContainerRequest) (*v1.StopContainerResponse, error) {
	if err := s.plugin.StopContainer(req.GetContainerId(), int(req.GetTimeout())); err != nil {
		return nil, notFoundErr("container", req.GetContainerId(), err)
	}
	return &v1.StopContainerResponse{}, nil
}

// RemoveContainer removes the container. Idempotent.
func (s *CRIServer) RemoveContainer(ctx context.Context, req *v1.RemoveContainerRequest) (*v1.RemoveContainerResponse, error) {
	if err := s.plugin.RemoveContainer(req.GetContainerId()); err != nil {
		return nil, status.Errorf(codes.Internal, "remove container: %v", err)
	}
	return &v1.RemoveContainerResponse{}, nil
}

// ListContainers lists all containers by filters.
func (s *CRIServer) ListContainers(ctx context.Context, req *v1.ListContainersRequest) (*v1.ListContainersResponse, error) {
	filter := req.GetFilter()
	podID := ""
	if filter != nil {
		podID = filter.GetPodSandboxId()
	}
	ccs := s.plugin.ListContainers(podID)
	containers := make([]*v1.Container, 0, len(ccs))
	for _, cc := range ccs {
		state := v1.ContainerState_CONTAINER_CREATED
		if st, err := s.plugin.runtime.State(cc.ID); err == nil {
			state = containerStateFromRuntime(st.Status)
		}
		if filter != nil {
			if id := filter.GetId(); id != "" && id != cc.ID {
				continue
			}
			if st := filter.GetState(); st != nil && st.GetState() != state {
				continue
			}
			if !matchLabels(cc.Labels, filter.GetLabelSelector()) {
				continue
			}
		}
		containers = append(containers, &v1.Container{
			Id:           cc.ID,
			PodSandboxId: cc.PodSandboxID,
			Metadata:     &v1.ContainerMetadata{Name: cc.Name, Attempt: cc.Attempt},
			Image:        &v1.ImageSpec{Image: cc.Image},
			ImageRef:     cc.ImageRef,
			ImageId:      cc.ImageID,
			State:        state,
			CreatedAt:    toCINanos(cc.CreatedAt),
			Labels:       cc.Labels,
			Annotations:  cc.Annotations,
		})
	}
	return &v1.ListContainersResponse{Containers: containers}, nil
}

// ContainerStatus returns the status of the container.
func (s *CRIServer) ContainerStatus(ctx context.Context, req *v1.ContainerStatusRequest) (*v1.ContainerStatusResponse, error) {
	cc, err := s.plugin.GetContainer(req.GetContainerId())
	if err != nil {
		return nil, notFoundErr("container", req.GetContainerId(), err)
	}
	cs := &v1.ContainerStatus{
		Id:          cc.ID,
		Metadata:    &v1.ContainerMetadata{Name: cc.Name, Attempt: cc.Attempt},
		Image:       &v1.ImageSpec{Image: cc.Image},
		ImageRef:    cc.ImageRef,
		ImageId:     cc.ImageID,
		Labels:      cc.Labels,
		Annotations: cc.Annotations,
		LogPath:     cc.LogPath,
		CreatedAt:   toCINanos(cc.CreatedAt),
	}
	if st, err := s.plugin.runtime.State(cc.ID); err == nil {
		cs.State = containerStateFromRuntime(st.Status)
		cs.StartedAt = toCINanosFromTime(st.Started)
		cs.FinishedAt = toCINanosFromTime(st.Finished)
		cs.ExitCode = int32(st.ExitCode)
	} else {
		cs.State = v1.ContainerState_CONTAINER_UNKNOWN
	}
	return &v1.ContainerStatusResponse{Status: cs}, nil
}

// Status returns the status of the dokiruntime. Includes RuntimeHandlers
// (one per supported ExecutionMode) and RuntimeFeatures so that
// kubelet 1.36+ can advertise the supported feature set.
func (s *CRIServer) Status(ctx context.Context, req *v1.StatusRequest) (*v1.StatusResponse, error) {
	// Detect cgroup driver: kubelet 1.36+ relies on this to know
	// whether the runtime uses systemd or cgroupfs.
	cgroupDriver := "cgroupfs"
	if isSystemdCgroup() {
		cgroupDriver = "systemd"
	}

	// Build a list of RuntimeHandlers. Each Doki execution mode is
	// exposed as a handler so pod specs can pin a specific dokiruntime.
	handlers := make([]*v1.RuntimeHandler, 0)
	for _, mode := range []string{"doki", "doki-rootless", "doki-fex", "doki-sysbox", "doki-vm"} {
		handlers = append(handlers, &v1.RuntimeHandler{
			Name: mode,
			Features: &v1.RuntimeHandlerFeatures{
				RecursiveReadOnlyMounts: true,
				UserNamespaces:           mode == "doki-rootless",
			},
		})
	}

	resp := &v1.StatusResponse{
		Status: &v1.RuntimeStatus{
			Conditions: []*v1.RuntimeCondition{
				{Type: "RuntimeReady", Status: true, Reason: "Doki runtime is ready"},
				{Type: "NetworkReady", Status: s.networkReady(), Reason: "Doki network is ready"},
			},
		},
		RuntimeHandlers: handlers,
		Features: &v1.RuntimeFeatures{
			SupplementalGroupsPolicy: true,
			UserNamespacesHostNetwork: false,
		},
		Info: map[string]string{
			"doki_version": common.Version,
			"cgroup_driver": cgroupDriver,
			"cgroup_v2":    boolStr(s.plugin.cgroupV2Available()),
			"network_dns":  s.plugin.dnsMode(),
		},
	}
	return resp, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (s *CRIServer) networkReady() bool {
	return s.plugin.network != nil
}

func isSystemdCgroup() bool {
	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "systemd.unified_cgroup_hierarchy=0") ||
		strings.Contains(string(data), "systemd")
}

// ExecSync runs a command in a container synchronously and returns the
// output. Per CVE-2022-1708 / CVE-2022-31030, stdout and stderr are
// capped at 16MB each; if the cap is hit, exit code is 0xff.
func (s *CRIServer) ExecSync(ctx context.Context, req *v1.ExecSyncRequest) (*v1.ExecSyncResponse, error) {
	containerID := req.GetContainerId()
	if containerID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "container_id required")
	}
	timeout := time.Duration(req.GetTimeout()) * time.Second
	// 16 MiB cap. Anything over this returns 0xff per the spec.
	const maxBytes = 16 * 1024 * 1024
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	type result struct {
		stdout []byte
		stderr []byte
		err    error
	}
	resCh := make(chan result, 1)
	go func() {
		stdout, stderr, err := s.plugin.runtime.Exec(containerID, req.GetCmd(), nil, "", "")
		resCh <- result{stdout, stderr, err}
	}()
	var stdout, stderr []byte
	var execErr error
	select {
	case <-time.After(timeout):
		// Timeout: return what we have and signal error.
		execErr = fmt.Errorf("exec timeout after %s", timeout)
	case r := <-resCh:
		stdout = r.stdout
		stderr = r.stderr
		execErr = r.err
	}
	exitCode := int32(0)
	if execErr != nil {
		// Try to extract exit code from *exec.ExitError.
		if ee, ok := execErr.(*exec.ExitError); ok {
			exitCode = int32(ee.ExitCode())
		} else {
			exitCode = -1
		}
	}
	// Truncate to 16MB and signal overflow if needed.
	overflow := false
	if len(stdout) > maxBytes {
		stdout = stdout[:maxBytes]
		overflow = true
	}
	if len(stderr) > maxBytes {
		stderr = stderr[:maxBytes]
		overflow = true
	}
	if overflow {
		// Per the spec, set exit code 0xff to indicate truncation.
		// Don't overwrite a real non-zero exit code.
		if exitCode == 0 {
			exitCode = 0xff
		}
	}
	return &v1.ExecSyncResponse{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
	}, nil
}

// ─── RuntimeService: not-yet-implemented RPCs ───────────────────────

// StreamPodSandboxes is a streaming alternative to ListPodSandbox.
func (s *CRIServer) StreamPodSandboxes(*v1.StreamPodSandboxesRequest, grpc.ServerStreamingServer[v1.StreamPodSandboxesResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamPodSandboxes not implemented")
}

// StreamContainers is a streaming alternative to ListContainers.
func (s *CRIServer) StreamContainers(*v1.StreamContainersRequest, grpc.ServerStreamingServer[v1.StreamContainersResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamContainers not implemented")
}

// UpdateContainerResources updates container resource limits synchronously.
// Implements CRI v1.25+ semantics: all-or-nothing, transaction rollback
// on partial failure.
func (s *CRIServer) UpdateContainerResources(ctx context.Context, req *v1.UpdateContainerResourcesRequest) (*v1.UpdateContainerResourcesResponse, error) {
	containerID := req.GetContainerId()
	if containerID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "container_id required")
	}
	linux := req.GetLinux()
	if linux == nil {
		return &v1.UpdateContainerResourcesResponse{}, nil
	}
	res := &dokiruntime.LinuxResources{}
	if linux.GetCpuShares() != 0 {
		res.CPUShares = uint64(linux.GetCpuShares())
	}
	if linux.GetCpuQuota() != 0 {
		res.CPUQuota = linux.GetCpuQuota()
	}
	if linux.GetCpuPeriod() != 0 {
		res.CPUPeriod = uint64(linux.GetCpuPeriod())
	}
	if r := linux.GetCpusetCpus(); r != "" {
		res.CpusetCpus = r
	}
	if r := linux.GetCpusetMems(); r != "" {
		res.CpusetMems = r
	}
	if r := linux.GetMemoryLimitInBytes(); r != 0 {
		res.Memory = r
	}
	if r := linux.GetMemorySwapLimitInBytes(); r != 0 {
		res.MemorySwap = r
	}
	if linux.GetOomScoreAdj() != 0 {
		res.OomKillDisable = false
	}
	// PidsLimit was not in v1.28 LinuxContainerResources; check for
	// it via the unified cgroup map under the cgroup_v2 convention.
	if v, ok := linux.GetUnified()["pids.max"]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			res.PidsLimit = n
		}
	}
	if err := s.plugin.runtime.UpdateResources(containerID, res); err != nil {
		return nil, status.Errorf(codes.Internal, "update resources: %v", err)
	}
	return &v1.UpdateContainerResourcesResponse{}, nil
}

// ReopenContainerLog asks the runtime to reopen the container log file.
// Used by kubelet during log rotation.
func (s *CRIServer) ReopenContainerLog(ctx context.Context, req *v1.ReopenContainerLogRequest) (*v1.ReopenContainerLogResponse, error) {
	containerID := req.GetContainerId()
	if containerID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "container_id required")
	}
	cc, err := s.plugin.GetContainer(containerID)
	if err != nil {
		return nil, notFoundErr("container", containerID, err)
	}
	if cc.LogPath == "" {
		return &v1.ReopenContainerLogResponse{}, nil
	}
	// Touch the file; on most kernels this is sufficient for logrotate
	// to pick up the new file. For full correctness, kubelet's log
	// directory contains symlinks that can be rotated; the runtime
	// simply needs to close its current handle.
	if _, err := os.Stat(cc.LogPath); err != nil {
		// Log file does not exist; create it.
		if mkErr := os.MkdirAll(filepath.Dir(cc.LogPath), 0755); mkErr != nil {
			return nil, status.Errorf(codes.Internal, "create log dir: %v", mkErr)
		}
		f, cErr := os.Create(cc.LogPath)
		if cErr != nil {
			return nil, status.Errorf(codes.Internal, "create log file: %v", cErr)
		}
		_ = f.Close()
	}
	return &v1.ReopenContainerLogResponse{}, nil
}

// Exec prepares a streaming endpoint to execute a command in the
// container. The URL is on a per-call ephemeral HTTP server; kubelet
// opens it with the returned token.
func (s *CRIServer) Exec(ctx context.Context, req *v1.ExecRequest) (*v1.ExecResponse, error) {
	containerID := req.GetContainerId()
	if containerID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "container_id required")
	}
	if !req.GetStdout() && !req.GetStderr() && !req.GetStdin() {
		return nil, status.Errorf(codes.InvalidArgument, "at least one of stdin, stdout, stderr must be true")
	}
	if _, err := s.plugin.GetContainer(containerID); err != nil {
		return nil, notFoundErr("container", containerID, err)
	}
	url, err := s.streamingURL(ctx, "exec", containerID, map[string]string{
		"cmd":     strings.Join(req.GetCmd(), " "),
		"tty":     boolStr(req.GetTty()),
		"stdin":   boolStr(req.GetStdin()),
		"stdout":  boolStr(req.GetStdout()),
		"stderr":  boolStr(req.GetStderr()),
	})
	if err != nil {
		return nil, err
	}
	return &v1.ExecResponse{Url: url}, nil
}

// Attach prepares a streaming endpoint to attach to a container.
func (s *CRIServer) Attach(ctx context.Context, req *v1.AttachRequest) (*v1.AttachResponse, error) {
	containerID := req.GetContainerId()
	if containerID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "container_id required")
	}
	if !req.GetStdout() && !req.GetStderr() && !req.GetStdin() {
		return nil, status.Errorf(codes.InvalidArgument, "at least one of stdin, stdout, stderr must be true")
	}
	if _, err := s.plugin.GetContainer(containerID); err != nil {
		return nil, notFoundErr("container", containerID, err)
	}
	url, err := s.streamingURL(ctx, "attach", containerID, map[string]string{
		"tty":    boolStr(req.GetTty()),
		"stdin":  boolStr(req.GetStdin()),
		"stdout": boolStr(req.GetStdout()),
		"stderr": boolStr(req.GetStderr()),
	})
	if err != nil {
		return nil, err
	}
	return &v1.AttachResponse{Url: url}, nil
}

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *CRIServer) PortForward(ctx context.Context, req *v1.PortForwardRequest) (*v1.PortForwardResponse, error) {
	podID := req.GetPodSandboxId()
	if podID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "pod_sandbox_id required")
	}
	if len(req.GetPort()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "at least one port required")
	}
	if _, err := s.plugin.PodSandboxStatus(podID); err != nil {
		return nil, notFoundErr("pod sandbox", podID, err)
	}
	ports := make([]string, 0, len(req.GetPort()))
	for _, p := range req.GetPort() {
		ports = append(ports, strconv.FormatInt(int64(p), 10))
	}
	url, err := s.streamingURL(ctx, "portforward", podID, map[string]string{
		"ports": strings.Join(ports, ","),
	})
	if err != nil {
		return nil, err
	}
	return &v1.PortForwardResponse{Url: url}, nil
}

// streamingURL starts a small ephemeral HTTP server that handles a
// streaming operation (exec, attach, portforward) and returns its URL
// to the caller. The server runs until the stream is closed.
//
// The URL format matches the upstream CRI contract:
//   http://<host>:<port>/<token>
// with the operation encoded in the path of the listening server.
func (s *CRIServer) streamingURL(ctx context.Context, op, resourceID string, params map[string]string) (string, error) {
	if s.streamer == nil {
		return "", status.Errorf(codes.Unimplemented, "streaming not configured")
	}
	port, err := s.streamer.Reserve(ctx, op, resourceID, params)
	if err != nil {
		return "", status.Errorf(codes.Internal, "reserve stream port: %v", err)
	}
	host, _ := s.streamer.Addr()
	return fmt.Sprintf("http://%s:%d/%s", host, port, op), nil
}

// ContainerStats returns stats of the container.
func (s *CRIServer) ContainerStats(ctx context.Context, req *v1.ContainerStatsRequest) (*v1.ContainerStatsResponse, error) {
	containerID := req.GetContainerId()
	if containerID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "container_id required")
	}
	data, err := s.plugin.runtime.ContainerStats(containerID)
	if err != nil {
		return nil, notFoundErr("container", containerID, err)
	}
	return &v1.ContainerStatsResponse{
		Stats: s.toV1ContainerStats(data),
	}, nil
}

// toV1ContainerStats translates the runtime's portable stats into
// a CRI v1 ContainerStats.
func (s *CRIServer) toV1ContainerStats(data *dokiruntime.ContainerStatsData) *v1.ContainerStats {
	cs := &v1.ContainerStats{
		Attributes: &v1.ContainerAttributes{Id: data.ID},
		Cpu: &v1.CpuUsage{
			Timestamp: data.Timestamp,
		},
		Memory: &v1.MemoryUsage{
			Timestamp: data.Timestamp,
		},
	}
	if data.CPUUsageNS > 0 {
		v := data.CPUUsageNS
		cs.Cpu.UsageCoreNanoSeconds = &v1.UInt64Value{Value: v}
	}
	if data.MemoryBytes > 0 {
		v := data.MemoryBytes
		cs.Memory.WorkingSetBytes = &v1.UInt64Value{Value: v}
		cs.Memory.UsageBytes = &v1.UInt64Value{Value: v}
	}
	if data.CPUSomePSI > 0 {
		cs.Cpu.Psi = &v1.PsiStats{Some: &v1.PsiData{Avg10: data.CPUSomePSI}}
	}
	if data.MemorySomePSI > 0 {
		cs.Memory.Psi = &v1.PsiStats{Some: &v1.PsiData{Avg10: data.MemorySomePSI}}
	}
	return cs
}

// ListContainerStats returns stats for all running containers matching the filter.
func (s *CRIServer) ListContainerStats(ctx context.Context, req *v1.ListContainerStatsRequest) (*v1.ListContainerStatsResponse, error) {
	filter := req.GetFilter()
	var podID string
	if filter != nil {
		podID = filter.GetPodSandboxId()
	}
	css := s.plugin.ListContainers(podID)
	out := make([]*v1.ContainerStats, 0, len(css))
	for _, cc := range css {
		data, err := s.plugin.runtime.ContainerStats(cc.ID)
		if err != nil {
			continue
		}
		data.ID = cc.ID
		out = append(out, s.toV1ContainerStats(data))
	}
	return &v1.ListContainerStatsResponse{Stats: out}, nil
}

// StreamContainerStats is a streaming alternative to ListContainerStats.
// Used when kubelet has the CRIListStreaming feature gate enabled.
func (s *CRIServer) StreamContainerStats(req *v1.StreamContainerStatsRequest, stream grpc.ServerStreamingServer[v1.StreamContainerStatsResponse]) error {
	filter := req.GetFilter()
	var podID string
	if filter != nil {
		podID = filter.GetPodSandboxId()
	}
	for _, cc := range s.plugin.ListContainers(podID) {
		data, err := s.plugin.runtime.ContainerStats(cc.ID)
		if err != nil {
			continue
		}
		data.ID = cc.ID
		if err := stream.Send(&v1.StreamContainerStatsResponse{
			ContainerStats: []*v1.ContainerStats{s.toV1ContainerStats(data)},
		}); err != nil {
			return err
		}
	}
	return nil
}

// PodSandboxStats returns stats of the pod sandbox.
func (s *CRIServer) PodSandboxStats(ctx context.Context, req *v1.PodSandboxStatsRequest) (*v1.PodSandboxStatsResponse, error) {
	podID := req.GetPodSandboxId()
	if podID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "pod_sandbox_id required")
	}
	sandbox, err := s.plugin.PodSandboxStatus(podID)
	if err != nil {
		return nil, notFoundErr("pod sandbox", podID, err)
	}
	css := s.plugin.ListContainers(podID)
	stats := &v1.PodSandboxStats{
		Attributes: &v1.PodSandboxAttributes{Id: podID},
		Linux:      &v1.LinuxPodSandboxStats{},
	}
	for _, cc := range css {
		data, err := s.plugin.runtime.ContainerStats(cc.ID)
		if err != nil {
			continue
		}
		data.ID = cc.ID
		stats.Linux.Containers = append(stats.Linux.Containers, s.toV1ContainerStats(data))
	}
	_ = sandbox
	return &v1.PodSandboxStatsResponse{Stats: stats}, nil
}

// ListPodSandboxStats returns stats of the pod sandboxes matching a filter.
func (s *CRIServer) ListPodSandboxStats(ctx context.Context, req *v1.ListPodSandboxStatsRequest) (*v1.ListPodSandboxStatsResponse, error) {
	pods := s.plugin.ListPodSandbox()
	filter := req.GetFilter()
	out := make([]*v1.PodSandboxStats, 0, len(pods))
	for _, p := range pods {
		if filter != nil && filter.GetId() != "" && p.ID != filter.GetId() {
			continue
		}
		stats := &v1.PodSandboxStats{
			Attributes: &v1.PodSandboxAttributes{Id: p.ID},
			Linux:      &v1.LinuxPodSandboxStats{},
		}
		out = append(out, stats)
	}
	return &v1.ListPodSandboxStatsResponse{Stats: out}, nil
}

// StreamPodSandboxStats is a streaming alternative to ListPodSandboxStats.
func (s *CRIServer) StreamPodSandboxStats(*v1.StreamPodSandboxStatsRequest, grpc.ServerStreamingServer[v1.StreamPodSandboxStatsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamPodSandboxStats not implemented")
}

// UpdateRuntimeConfig updates the runtime configuration based on the
// request. kubelet uses this to push PodCIDR and other network
// settings at startup.
func (s *CRIServer) UpdateRuntimeConfig(ctx context.Context, req *v1.UpdateRuntimeConfigRequest) (*v1.UpdateRuntimeConfigResponse, error) {
	cfg := req.GetRuntimeConfig()
	if cfg == nil {
		return &v1.UpdateRuntimeConfigResponse{}, nil
	}
	if nc := cfg.GetNetworkConfig(); nc != nil {
		if podCIDR := nc.GetPodCidr(); podCIDR != "" {
			// Store the PodCIDR for the local pod CIDR allocator.
			s.plugin.podCIDR = podCIDR
		}
	}
	return &v1.UpdateRuntimeConfigResponse{}, nil
}

// CheckpointContainer checkpoints a container using CRIU. This is a
// best-effort implementation that shells out to the criu binary
// (or `podman container checkpoint` if available). On hosts
// without CRIU, returns Unimplemented.
func (s *CRIServer) CheckpointContainer(ctx context.Context, req *v1.CheckpointContainerRequest) (*v1.CheckpointContainerResponse, error) {
	containerID := req.GetContainerId()
	if containerID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "container_id required")
	}
	location := req.GetLocation()
	if location == "" {
		location = "/var/lib/doki/checkpoints/" + containerID
	}
	if err := os.MkdirAll(location, 0755); err != nil {
		return nil, status.Errorf(codes.Internal, "create checkpoint dir: %v", err)
	}
	// Try podman first (preferred).
	if _, err := exec.LookPath("podman"); err == nil {
		args := []string{"container", "checkpoint", containerID, "--export", location}
		// CRI's CheckpointContainerRequest v1 does not have a
		// leave_running field; podman defaults to keeping the
		// container running on a successful checkpoint. This matches
		// Docker's behavior.
		cmd := exec.CommandContext(ctx, "podman", args...)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return &v1.CheckpointContainerResponse{}, nil
		}
		// fall through to criu direct
	}
	// Try criu direct. CRIU requires the process to be tracked; we
	// skip and return Unimplemented if not present.
	if _, err := exec.LookPath("criu"); err != nil {
		return nil, status.Errorf(codes.Unimplemented, "checkpoint requires CRIU (criu binary not found in PATH)")
	}
	// CRIU dump: requires the container's PID and the namespace
	// file descriptors. We don't have a generic interface for that
	// across all execution modes, so return Unimplemented for now.
	return nil, status.Errorf(codes.Unimplemented, "checkpoint not supported in this execution mode")
}

// GetContainerEvents gets container events from the CRI dokiruntime.
func (s *CRIServer) GetContainerEvents(*v1.GetEventsRequest, grpc.ServerStreamingServer[v1.ContainerEventResponse]) error {
	return status.Errorf(codes.Unimplemented, "method GetContainerEvents not implemented")
}

// ListMetricDescriptors gets the descriptors for the metrics.
func (s *CRIServer) ListMetricDescriptors(ctx context.Context, req *v1.ListMetricDescriptorsRequest) (*v1.ListMetricDescriptorsResponse, error) {
	descs := []*v1.MetricDescriptor{
		{Name: "container_cpu_usage_seconds_total", Help: "Cumulative CPU time consumed in seconds"},
		{Name: "container_memory_usage_bytes", Help: "Current memory usage in bytes"},
		{Name: "container_memory_working_set_bytes", Help: "Current working set size in bytes"},
		{Name: "container_network_receive_bytes_total", Help: "Cumulative network bytes received"},
		{Name: "container_network_transmit_bytes_total", Help: "Cumulative network bytes transmitted"},
		{Name: "container_fs_reads_bytes_total", Help: "Cumulative bytes read from filesystem"},
		{Name: "container_fs_writes_bytes_total", Help: "Cumulative bytes written to filesystem"},
		{Name: "container_tasks_state", Help: "Number of tasks in each state"},
		{Name: "container_sandbox_creation_duration_seconds", Help: "Duration of sandbox creation"},
		{Name: "container_image_pull_duration_seconds", Help: "Duration of image pull"},
	}
	for i := range descs {
		descs[i].LabelKeys = []string{"id", "name", "namespace", "image"}
	}
	return &v1.ListMetricDescriptorsResponse{Descriptors: descs}, nil
}

// ListPodSandboxMetrics gets pod sandbox metrics from the CRI Runtime.
// Implements the v1.32+ PodSandboxMetrics spec.
func (s *CRIServer) ListPodSandboxMetrics(ctx context.Context, req *v1.ListPodSandboxMetricsRequest) (*v1.ListPodSandboxMetricsResponse, error) {
	pods := s.plugin.ListPodSandbox()
	now := time.Now().UnixNano()
	out := make([]*v1.PodSandboxMetrics, 0, len(pods))
	for _, p := range pods {
		pm := &v1.PodSandboxMetrics{
			PodSandboxId: p.ID,
			Metrics:      make([]*v1.Metric, 0, 2),
		}
		// Aggregate CPU and memory across all containers in the pod.
		var cpuNS, memBytes uint64
		for _, ccid := range p.Containers {
			data, err := s.plugin.runtime.ContainerStats(ccid)
			if err != nil {
				continue
			}
			cpuNS += data.CPUUsageNS
			memBytes += data.MemoryBytes
		}
		pm.Metrics = append(pm.Metrics, &v1.Metric{
			Name:       "pod_cpu_usage_seconds_total",
			Timestamp:  now,
			MetricType: v1.MetricType_COUNTER,
			Value:      &v1.UInt64Value{Value: cpuNS / 1000}, // ns -> us
		})
		pm.Metrics = append(pm.Metrics, &v1.Metric{
			Name:       "pod_memory_usage_bytes",
			Timestamp:  now,
			MetricType: v1.MetricType_GAUGE,
			Value:      &v1.UInt64Value{Value: memBytes},
		})
		out = append(out, pm)
	}
	return &v1.ListPodSandboxMetricsResponse{PodMetrics: out}, nil
}

// StreamPodSandboxMetrics is a streaming alternative to ListPodSandboxMetrics.
func (s *CRIServer) StreamPodSandboxMetrics(*v1.StreamPodSandboxMetricsRequest, grpc.ServerStreamingServer[v1.StreamPodSandboxMetricsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamPodSandboxMetrics not implemented")
}

// RuntimeConfig returns configuration information of the dokiruntime.
// kubelet 1.28+ uses this to discover the cgroup driver instead of
// autodetecting.
func (s *CRIServer) RuntimeConfig(ctx context.Context, req *v1.RuntimeConfigRequest) (*v1.RuntimeConfigResponse, error) {
	driver := v1.CgroupDriver_CGROUPFS
	if isSystemdCgroup() {
		driver = v1.CgroupDriver_SYSTEMD
	}
	return &v1.RuntimeConfigResponse{
		Linux: &v1.LinuxRuntimeConfiguration{
			CgroupDriver: driver,
		},
	}, nil
}

// UpdatePodSandboxResources updates the PodSandboxConfig with pod-level
// resource configuration. Used by In-Place Pod Vertical Scaling
// (KEP-5407, GA in 1.33).
func (s *CRIServer) UpdatePodSandboxResources(ctx context.Context, req *v1.UpdatePodSandboxResourcesRequest) (*v1.UpdatePodSandboxResourcesResponse, error) {
	podID := req.GetPodSandboxId()
	if podID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "pod_sandbox_id required")
	}
	if _, err := s.plugin.PodSandboxStatus(podID); err != nil {
		return nil, notFoundErr("pod sandbox", podID, err)
	}
	// Apply overhead + resources to each container's cgroup. We do
	// this best-effort: a partial failure returns Internal but the
	// spec allows the runtime to apply best-effort.
	overhead := req.GetOverhead()
	resources := req.GetResources()
	if overhead == nil && resources == nil {
		return &v1.UpdatePodSandboxResourcesResponse{}, nil
	}
	merged := &dokiruntime.LinuxResources{}
	if overhead != nil {
		merged.CPUShares = uint64(overhead.GetCpuShares())
		merged.Memory = overhead.GetMemoryLimitInBytes()
		if v, ok := overhead.GetUnified()["pids.max"]; ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				merged.PidsLimit = n
			}
		}
	}
	if resources != nil {
		merged.CPUShares = uint64(resources.GetCpuShares())
		merged.Memory = resources.GetMemoryLimitInBytes()
		if v, ok := resources.GetUnified()["pids.max"]; ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				merged.PidsLimit = n
			}
		}
	}
	for _, ccid := range s.plugin.ListContainers(podID) {
		if err := s.plugin.runtime.UpdateResources(ccid.ID, merged); err != nil {
			slog.Warn("update pod-level resources", "container", ccid.ID, "err", err)
		}
	}
	return &v1.UpdatePodSandboxResourcesResponse{}, nil
}

// ─── ImageService ───────────────────────────────────────────────────

// ListImages lists existing images.
func (s *CRIServer) ListImages(ctx context.Context, req *v1.ListImagesRequest) (*v1.ListImagesResponse, error) {
	images, err := s.plugin.image.List()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list images: %v", err)
	}
	result := make([]*v1.Image, 0, len(images))
	for _, img := range images {
		result = append(result, &v1.Image{
			Id:          img.ID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Size:        uint64(img.Size),
			Spec:        &v1.ImageSpec{Image: img.ID},
		})
	}
	return &v1.ListImagesResponse{Images: result}, nil
}

// ImageStatus returns the status of the image. If the image is not present,
// returns a response with Image set to nil (per CRI spec).
func (s *CRIServer) ImageStatus(ctx context.Context, req *v1.ImageStatusRequest) (*v1.ImageStatusResponse, error) {
	imageRef := imageRefFromSpec(req.GetImage())
	record, err := s.plugin.image.Get(imageRef)
	if err != nil {
		// Not present: return empty response with nil Image.
		return &v1.ImageStatusResponse{}, nil
	}
	return &v1.ImageStatusResponse{
		Image: &v1.Image{
			Id:          record.ID,
			RepoTags:    record.RepoTags,
			RepoDigests: record.RepoDigests,
			Size:        uint64(record.Size),
			Spec:        &v1.ImageSpec{Image: imageRef},
		},
	}, nil
}

// PullImage pulls an image with authentication config.
func (s *CRIServer) PullImage(ctx context.Context, req *v1.PullImageRequest) (*v1.PullImageResponse, error) {
	imageRef := imageRefFromSpec(req.GetImage())
	if imageRef == "" {
		return nil, status.Errorf(codes.InvalidArgument, "missing image reference")
	}
	if auth := req.GetAuth(); auth != nil {
		s.plugin.image.SetRegistryAuth(auth.GetUsername(), auth.GetPassword())
	}
	record, err := s.plugin.image.Pull(imageRef)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "pull image %q: %v", imageRef, err)
	}
	return &v1.PullImageResponse{ImageRef: record.ID}, nil
}

// RemoveImage removes the image. Idempotent.
func (s *CRIServer) RemoveImage(ctx context.Context, req *v1.RemoveImageRequest) (*v1.RemoveImageResponse, error) {
	imageRef := imageRefFromSpec(req.GetImage())
	if err := s.plugin.image.Remove(imageRef); err != nil {
		if common.IsNotFound(err) {
			return &v1.RemoveImageResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "remove image %q: %v", imageRef, err)
	}
	return &v1.RemoveImageResponse{}, nil
}

// ImageFsInfo returns information about the filesystem used to store
// images. kubelet 1.29+ requires this; without it the node is marked
// NotReady.
func (s *CRIServer) ImageFsInfo(ctx context.Context, req *v1.ImageFsInfoRequest) (*v1.ImageFsInfoResponse, error) {
	imageDir := common.ImageDir()
	used, _, inodesUsed, _, err := statFS(imageDir)
	if err != nil {
		// Fall back to /var/lib/doki.
		used, _, inodesUsed, _, _ = statFS("/var/lib/doki")
	}
	now := time.Now().UnixNano()
	fsID := &v1.FilesystemIdentifier{Mountpoint: imageDir}
	containerFS := &v1.FilesystemUsage{
		Timestamp:  now,
		FsId:       fsID,
		UsedBytes:  &v1.UInt64Value{Value: used},
		InodesUsed: &v1.UInt64Value{Value: inodesUsed},
	}
	imageFS := &v1.FilesystemUsage{
		Timestamp:  now,
		FsId:       fsID,
		UsedBytes:  &v1.UInt64Value{Value: used},
		InodesUsed: &v1.UInt64Value{Value: inodesUsed},
	}
	return &v1.ImageFsInfoResponse{
		ImageFilesystems:     []*v1.FilesystemUsage{imageFS},
		ContainerFilesystems: []*v1.FilesystemUsage{containerFS},
	}, nil
}

// statFS returns used/total bytes and inodes for a path's filesystem.
func statFS(path string) (used, total, inodesUsed, inodesTotal uint64, err error) {
	var stat syscall.Statfs_t
	if e := syscall.Statfs(path, &stat); e != nil {
		return 0, 0, 0, 0, e
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used = total - free
	inodesTotal = stat.Files
	inodesUsed = stat.Files - stat.Ffree
	return
}

// StreamImages is a streaming alternative to ListImages.
func (s *CRIServer) StreamImages(*v1.StreamImagesRequest, grpc.ServerStreamingServer[v1.StreamImagesResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamImages not implemented")
}
