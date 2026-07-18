// Package cri provides the Kubernetes Container Runtime Interface.
package cri

import (
	"fmt"
	"os"
	"sync"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
	"github.com/OpceanAI/Doki/pkg/runtime"
)

// CRIPlugin implements the Kubernetes Container Runtime Interface.
// This provides a K8s-compatible container runtime via the CRI gRPC protocol.
type CRIPlugin struct {
	mu           sync.RWMutex
	runtime      *runtime.Runtime
	image        *image.Store
	network      *network.Manager
	podSandboxes map[string]*PodSandbox
	containers   map[string]*CRIContainer
	podCIDR      string
}

// CRIContainer holds CRI-specific container metadata that is not stored in
// the lower-level runtime.ContainerState (e.g. pod sandbox association and
// CRI ContainerMetadata such as name/attempt).
type CRIContainer struct {
	ID           string
	PodSandboxID string
	Name         string
	Attempt      uint32
	Image        string
	ImageRef     string
	ImageID      string
	Args         []string
	Env          []string
	WorkingDir   string
	Labels       map[string]string
	Annotations  map[string]string
	Mounts       []common.Mount
	CreatedAt    int64
	LogPath      string
}

// PodSandbox represents a Kubernetes Pod.
type PodSandbox struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	UID          string            `json:"uid"`
	CreatedAt    int64             `json:"created_at"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	Containers   []string          `json:"containers"`
	Hostname     string            `json:"hostname"`
	LogDirectory string            `json:"log_directory"`
	State        string            `json:"state"`
}

// NewCRIPlugin creates a new CRI plugin.
func NewCRIPlugin(rt *runtime.Runtime, img *image.Store, net *network.Manager) *CRIPlugin {
	return &CRIPlugin{
		runtime:      rt,
		image:        img,
		network:      net,
		podSandboxes: make(map[string]*PodSandbox),
		containers:   make(map[string]*CRIContainer),
	}
}

// RunPodSandbox creates and starts a pod sandbox.
func (c *CRIPlugin) RunPodSandbox(id, name, namespace string, labels, annotations map[string]string) (*PodSandbox, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a pause container that acts as the pod infra container.
	pauseContainerID := common.GenerateID(64)

	// Create pod network namespace.
	networkCfg := &network.NetworkConfig{
		Name:   "pod-" + id[:12],
		Driver: "bridge",
	}
	nw, err := c.network.CreateNetwork(networkCfg)
	if err != nil {
		return nil, fmt.Errorf("create pod network: %w", err)
	}

	// Create the infra container (pause container).
	cfg := &runtime.Config{
		ID:          pauseContainerID,
		Args:        []string{"/bin/sh", "-c", "while true; do sleep 3600; done"},
		Env:         []string{},
		Labels:      labels,
		Annotations: annotations,
		NetworkMode: common.NetworkBridge,
	}

	if _, err := c.runtime.Create(cfg); err != nil {
		return nil, fmt.Errorf("create pause container: %w", err)
	}

	if err := c.runtime.Start(pauseContainerID); err != nil {
		return nil, fmt.Errorf("start pause container: %w", err)
	}

	sandbox := &PodSandbox{
		ID:           id,
		Name:         name,
		Namespace:    namespace,
		UID:          id,
		CreatedAt:    common.NowTimestamp(),
		Labels:       labels,
		Annotations:  annotations,
		Containers:   []string{pauseContainerID},
		Hostname:     name,
		LogDirectory: "/var/log/pods/" + namespace + "_" + name + "_" + id,
		State:        "SANDBOX_READY",
	}

	c.podSandboxes[id] = sandbox

	_ = nw

	return sandbox, nil
}

// StopPodSandbox stops a pod sandbox.
func (c *CRIPlugin) StopPodSandbox(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sandbox, ok := c.podSandboxes[id]
	if !ok {
		return common.NewErrNotFound("pod", id)
	}

	for _, containerID := range sandbox.Containers {
		_ = c.runtime.Stop(containerID, 10)
		_ = c.runtime.Delete(containerID, true)
		delete(c.containers, containerID)
	}
	sandbox.Containers = nil

	sandbox.State = "SANDBOX_NOTREADY"
	// Cleanup network.
	_ = c.network.Disconnect("pod-"+id[:12], "", 0)

	return nil
}

// RemovePodSandbox removes a pod sandbox.
func (c *CRIPlugin) RemovePodSandbox(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sandbox, ok := c.podSandboxes[id]
	if !ok {
		return nil
	}

	for _, containerID := range sandbox.Containers {
		_ = c.runtime.Stop(containerID, 10)
		_ = c.runtime.Delete(containerID, true)
		delete(c.containers, containerID)
	}
	sandbox.Containers = nil

	delete(c.podSandboxes, id)
	return nil
}

// PodSandboxStatus returns the status of a pod sandbox.
func (c *CRIPlugin) PodSandboxStatus(id string) (*PodSandbox, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sandbox, ok := c.podSandboxes[id]
	if !ok {
		return nil, common.NewErrNotFound("pod", id)
	}

	return sandbox, nil
}

// ListPodSandbox lists all pod sandboxes.
func (c *CRIPlugin) ListPodSandbox() []*PodSandbox {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pods := make([]*PodSandbox, 0, len(c.podSandboxes))
	for _, pod := range c.podSandboxes {
		pods = append(pods, pod)
	}

	return pods
}

// CreateContainer creates a container within a pod sandbox but does NOT start
// it. The CRI gRPC API separates CreateContainer and StartContainer, so the
// caller is responsible for invoking StartContainer afterwards.
func (c *CRIPlugin) CreateContainer(cc *CRIContainer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sandbox, ok := c.podSandboxes[cc.PodSandboxID]
	if !ok {
		return common.NewErrNotFound("pod", cc.PodSandboxID)
	}

	// Pull image if needed.
	if cc.Image != "" && !c.image.Exists(cc.Image) {
		_, _ = c.image.Pull(cc.Image)
	}

	// Resolve image ID/ref for status reporting.
	if record, err := c.image.Get(cc.Image); err == nil {
		cc.ImageID = record.ID
		cc.ImageRef = record.ID
	}

	cfg := &runtime.Config{
		ID:          cc.ID,
		Args:        cc.Args,
		Env:         cc.Env,
		Cwd:         cc.WorkingDir,
		Labels:      cc.Labels,
		Annotations: cc.Annotations,
		Mounts:      cc.Mounts,
		ImageRef:    cc.Image,
		NetworkMode: common.NetworkBridge,
	}

	// Resolve the image's layer tarballs so Create extracts a real rootfs.
	// Without this the container's rootfs is empty and nothing runs (the
	// kubelet's exec liveness/readiness probes and the entrypoint both fail).
	if layers, err := c.image.GetLayerPaths(cc.Image); err == nil {
		cfg.ImageLayers = layers
	}

	if _, err := c.runtime.Create(cfg); err != nil {
		return err
	}

	cc.CreatedAt = common.NowTimestamp()
	c.containers[cc.ID] = cc
	sandbox.Containers = append(sandbox.Containers, cc.ID)
	return nil
}

// StartContainer starts a previously created container.
func (c *CRIPlugin) StartContainer(containerID string) error {
	c.mu.Lock()
	cc, ok := c.containers[containerID]
	c.mu.Unlock()
	if !ok {
		return common.NewErrNotFound("container", containerID)
	}
	_ = cc
	return c.runtime.Start(containerID)
}

// StopContainer stops a running container with a grace period (seconds).
func (c *CRIPlugin) StopContainer(containerID string, timeout int) error {
	c.mu.RLock()
	_, ok := c.containers[containerID]
	c.mu.RUnlock()
	if !ok {
		return common.NewErrNotFound("container", containerID)
	}
	return c.runtime.Stop(containerID, timeout)
}

// RemoveContainer removes a container. If it is still running it is forcibly
// removed. This call is idempotent.
func (c *CRIPlugin) RemoveContainer(containerID string) error {
	c.mu.Lock()
	cc, ok := c.containers[containerID]
	if ok {
		// Remove from the owning sandbox's container list.
		if sandbox, sOK := c.podSandboxes[cc.PodSandboxID]; sOK {
			sandbox.Containers = removeString(sandbox.Containers, containerID)
		}
		delete(c.containers, containerID)
	}
	c.mu.Unlock()

	// Idempotent: if the container was never tracked or already removed,
	// runtime.Delete with force=true still succeeds.
	return c.runtime.Delete(containerID, true)
}

// GetContainer returns the CRI metadata for a container.
func (c *CRIPlugin) GetContainer(containerID string) (*CRIContainer, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cc, ok := c.containers[containerID]
	if !ok {
		return nil, common.NewErrNotFound("container", containerID)
	}
	return cc, nil
}

// ListContainers returns the CRI metadata for all containers, optionally
// filtered by pod sandbox ID. An empty podID returns every container.
func (c *CRIPlugin) ListContainers(podID string) []*CRIContainer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]*CRIContainer, 0, len(c.containers))
	for _, cc := range c.containers {
		if podID != "" && cc.PodSandboxID != podID {
			continue
		}
		result = append(result, cc)
	}
	return result
}

func removeString(slice []string, s string) []string {
	out := make([]string, 0, len(slice))
	for _, v := range slice {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

// PullImage pulls an image.
func (c *CRIPlugin) PullImage(imageName string) error {
	_, err := c.image.Pull(imageName)
	return err
}

// ListImages lists all available images.
func (c *CRIPlugin) ListImages() ([]common.ImageInfo, error) {
	return c.image.List()
}

// ImageStatus returns the status of an image.
func (c *CRIPlugin) ImageStatus(imageName string) (*common.ImageInfo, error) {
	record, err := c.image.Get(imageName)
	if err != nil {
		return nil, err
	}

	return &common.ImageInfo{
		ID:       record.ID[:12],
		RepoTags: record.RepoTags,
		Size:     record.Size,
	}, nil
}

// RemoveImage removes an image.
func (c *CRIPlugin) RemoveImage(imageName string) error {
	return c.image.Remove(imageName)
}

// IsCRIReady checks if CRI plugin is ready.
func (c *CRIPlugin) IsCRIReady() bool {
	return true
}

// Version returns CRI version info.
func (c *CRIPlugin) Version() map[string]string {
	return map[string]string{
		"runtime_name":    "doki",
		"runtime_version": common.Version,
	}
}

// cgroupV2Available returns true when /sys/fs/cgroup/cgroup.controllers
// (a cgroup v2-only file) exists. Used by Status to advertise support.
func (c *CRIPlugin) cgroupV2Available() bool {
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	return err == nil
}

// dnsMode reports which DNS backend is in use. The CRI Status.Info
// field exposes this to the kubelet for diagnostics.
func (c *CRIPlugin) dnsMode() string {
	if c.network == nil {
		return "none"
	}
	return "internal"
}

// UpdateResources updates cgroup limits for a running container.
// Implements CRI UpdateContainerResources semantics: best-effort
// atomic with rollback on any controller failure.
func (c *CRIPlugin) UpdateResources(containerID string, resources *runtime.LinuxResources) error {
	if c.runtime == nil {
		return common.NewErrNotFound("container", containerID)
	}
	return c.runtime.UpdateResources(containerID, resources)
}
