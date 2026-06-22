// Package cri provides the Kubernetes Container Runtime Interface.
//
// server.go implements a real gRPC CRI server that exposes the
// RuntimeServiceServer and ImageServiceServer interfaces defined by
// k8s.io/cri-api. It translates between the CRI protobuf requests/responses
// and the existing CRIPlugin / runtime.Runtime / image.Store operations.
package cri

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// CRIServer is a gRPC server implementing the Kubernetes Container Runtime
// Interface (CRI) v1 RuntimeService and ImageService.
//
// It embeds the generated Unimplemented* servers by value (required for
// forward compatibility) and delegates the actual container/image work to
// the existing CRIPlugin, which wraps runtime.Runtime, image.Store and
// network.Manager.
type CRIServer struct {
	v1.UnimplementedRuntimeServiceServer
	v1.UnimplementedImageServiceServer

	plugin *CRIPlugin
	server *grpc.Server
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
	return &v1.PodSandboxStatusResponse{
		Status: &v1.PodSandboxStatus{
			Id:          sandbox.ID,
			Metadata:    &v1.PodSandboxMetadata{Name: sandbox.Name, Namespace: sandbox.Namespace, Uid: sandbox.UID},
			State:       podSandboxStateFromString(sandbox.State),
			CreatedAt:   toCINanos(sandbox.CreatedAt),
			Labels:      sandbox.Labels,
			Annotations: sandbox.Annotations,
		},
	}, nil
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

// Status returns the status of the runtime. Both RuntimeReady and NetworkReady
// are reported as true.
func (s *CRIServer) Status(ctx context.Context, req *v1.StatusRequest) (*v1.StatusResponse, error) {
	return &v1.StatusResponse{
		Status: &v1.RuntimeStatus{
			Conditions: []*v1.RuntimeCondition{
				{
					Type:   "RuntimeReady",
					Status: true,
					Reason: "Doki runtime is ready",
				},
				{
					Type:   "NetworkReady",
					Status: true,
					Reason: "Doki network is ready",
				},
			},
		},
	}, nil
}

// ExecSync runs a command in a container synchronously and returns the output.
func (s *CRIServer) ExecSync(ctx context.Context, req *v1.ExecSyncRequest) (*v1.ExecSyncResponse, error) {
	stdout, stderr, err := s.plugin.runtime.Exec(req.GetContainerId(), req.GetCmd(), nil, "", "")
	resp := &v1.ExecSyncResponse{
		Stdout: stdout,
		Stderr: stderr,
	}
	if err != nil {
		resp.ExitCode = -1
	}
	return resp, nil
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
func (s *CRIServer) UpdateContainerResources(ctx context.Context, req *v1.UpdateContainerResourcesRequest) (*v1.UpdateContainerResourcesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateContainerResources not implemented")
}

// ReopenContainerLog asks the runtime to reopen the container log file.
func (s *CRIServer) ReopenContainerLog(ctx context.Context, req *v1.ReopenContainerLogRequest) (*v1.ReopenContainerLogResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReopenContainerLog not implemented")
}

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *CRIServer) Exec(ctx context.Context, req *v1.ExecRequest) (*v1.ExecResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Exec not implemented")
}

// Attach prepares a streaming endpoint to attach to a running container.
func (s *CRIServer) Attach(ctx context.Context, req *v1.AttachRequest) (*v1.AttachResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Attach not implemented")
}

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *CRIServer) PortForward(ctx context.Context, req *v1.PortForwardRequest) (*v1.PortForwardResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PortForward not implemented")
}

// ContainerStats returns stats of the container.
func (s *CRIServer) ContainerStats(ctx context.Context, req *v1.ContainerStatsRequest) (*v1.ContainerStatsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ContainerStats not implemented")
}

// ListContainerStats returns stats of all running containers.
func (s *CRIServer) ListContainerStats(ctx context.Context, req *v1.ListContainerStatsRequest) (*v1.ListContainerStatsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListContainerStats not implemented")
}

// StreamContainerStats is a streaming alternative to ListContainerStats.
func (s *CRIServer) StreamContainerStats(*v1.StreamContainerStatsRequest, grpc.ServerStreamingServer[v1.StreamContainerStatsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamContainerStats not implemented")
}

// PodSandboxStats returns stats of the pod sandbox.
func (s *CRIServer) PodSandboxStats(ctx context.Context, req *v1.PodSandboxStatsRequest) (*v1.PodSandboxStatsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PodSandboxStats not implemented")
}

// ListPodSandboxStats returns stats of the pod sandboxes matching a filter.
func (s *CRIServer) ListPodSandboxStats(ctx context.Context, req *v1.ListPodSandboxStatsRequest) (*v1.ListPodSandboxStatsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListPodSandboxStats not implemented")
}

// StreamPodSandboxStats is a streaming alternative to ListPodSandboxStats.
func (s *CRIServer) StreamPodSandboxStats(*v1.StreamPodSandboxStatsRequest, grpc.ServerStreamingServer[v1.StreamPodSandboxStatsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamPodSandboxStats not implemented")
}

// UpdateRuntimeConfig updates the runtime configuration based on the request.
func (s *CRIServer) UpdateRuntimeConfig(ctx context.Context, req *v1.UpdateRuntimeConfigRequest) (*v1.UpdateRuntimeConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateRuntimeConfig not implemented")
}

// CheckpointContainer checkpoints a container.
func (s *CRIServer) CheckpointContainer(ctx context.Context, req *v1.CheckpointContainerRequest) (*v1.CheckpointContainerResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CheckpointContainer not implemented")
}

// GetContainerEvents gets container events from the CRI runtime.
func (s *CRIServer) GetContainerEvents(*v1.GetEventsRequest, grpc.ServerStreamingServer[v1.ContainerEventResponse]) error {
	return status.Errorf(codes.Unimplemented, "method GetContainerEvents not implemented")
}

// ListMetricDescriptors gets the descriptors for the metrics.
func (s *CRIServer) ListMetricDescriptors(ctx context.Context, req *v1.ListMetricDescriptorsRequest) (*v1.ListMetricDescriptorsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListMetricDescriptors not implemented")
}

// ListPodSandboxMetrics gets pod sandbox metrics from the CRI Runtime.
func (s *CRIServer) ListPodSandboxMetrics(ctx context.Context, req *v1.ListPodSandboxMetricsRequest) (*v1.ListPodSandboxMetricsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListPodSandboxMetrics not implemented")
}

// StreamPodSandboxMetrics is a streaming alternative to ListPodSandboxMetrics.
func (s *CRIServer) StreamPodSandboxMetrics(*v1.StreamPodSandboxMetricsRequest, grpc.ServerStreamingServer[v1.StreamPodSandboxMetricsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamPodSandboxMetrics not implemented")
}

// RuntimeConfig returns configuration information of the runtime.
func (s *CRIServer) RuntimeConfig(ctx context.Context, req *v1.RuntimeConfigRequest) (*v1.RuntimeConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RuntimeConfig not implemented")
}

// UpdatePodSandboxResources updates the PodSandboxConfig with pod-level
// resource configuration.
func (s *CRIServer) UpdatePodSandboxResources(ctx context.Context, req *v1.UpdatePodSandboxResourcesRequest) (*v1.UpdatePodSandboxResourcesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdatePodSandboxResources not implemented")
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

// ImageFsInfo returns information about the filesystem used to store images.
func (s *CRIServer) ImageFsInfo(ctx context.Context, req *v1.ImageFsInfoRequest) (*v1.ImageFsInfoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ImageFsInfo not implemented")
}

// StreamImages is a streaming alternative to ListImages.
func (s *CRIServer) StreamImages(*v1.StreamImagesRequest, grpc.ServerStreamingServer[v1.StreamImagesResponse]) error {
	return status.Errorf(codes.Unimplemented, "method StreamImages not implemented")
}
