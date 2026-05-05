package cri

import (
	"fmt"
	"sync"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
	"github.com/OpceanAI/Doki/pkg/runtime"
)

// CRIPlugin implements the Kubernetes Container Runtime Interface.
// This provides a K8s-compatible container runtime via the CRI gRPC protocol.
type CRIPlugin struct {
	mu       sync.RWMutex
	runtime  *runtime.Runtime
	image    *image.Store
	network  *network.Manager
	podSandboxes map[string]*PodSandbox
}

// PodSandbox represents a Kubernetes Pod.
type PodSandbox struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	UID         string            `json:"uid"`
	CreatedAt   int64             `json:"created_at"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Containers  []string          `json:"containers"`
	Hostname    string            `json:"hostname"`
	LogDirectory string           `json:"log_directory"`
	State       string            `json:"state"`
}

// NewCRIPlugin creates a new CRI plugin.
func NewCRIPlugin(rt *runtime.Runtime, img *image.Store, net *network.Manager) *CRIPlugin {
	return &CRIPlugin{
		runtime:      rt,
		image:        img,
		network:      net,
		podSandboxes: make(map[string]*PodSandbox),
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
		c.runtime.Stop(containerID, 10)
		c.runtime.Delete(containerID, true)
	}

	sandbox.State = "SANDBOX_NOTREADY"
	// Cleanup network.
	c.network.Disconnect("pod-"+id[:12], "", 0)

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
		c.runtime.Stop(containerID, 10)
		c.runtime.Delete(containerID, true)
	}

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

// CreateContainer creates a container within a pod sandbox.
func (c *CRIPlugin) CreateContainer(podID, containerID string, imageName string, args, env []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sandbox, ok := c.podSandboxes[podID]
	if !ok {
		return common.NewErrNotFound("pod", podID)
	}

	// Pull image if needed.
	if !c.image.Exists(imageName) {
		c.image.Pull(imageName)
	}

	cfg := &runtime.Config{
		ID: containerID,
		Args:       args,
		Env:        env,
	}

	if _, err := c.runtime.Create(cfg); err != nil {
		return err
	}

	if err := c.runtime.Start(containerID); err != nil {
		return err
	}

	sandbox.Containers = append(sandbox.Containers, containerID)
	return nil
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
