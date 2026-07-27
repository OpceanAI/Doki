// Package kubelet provides the Kubernetes kubelet agent.
package kubelet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	k8s "github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// defaultCRISocket is the Unix socket the kubelet talks to when no
// explicit CRI endpoint is configured.
func defaultCRISocket() string { return common.DefaultCRISocket() }

type Kubelet struct {
	nodeName string
	store    store.Store
	mu       sync.RWMutex
	pods     map[string]*k8s.Pod
	running  map[string]bool
	logger   *slog.Logger

	// nodeIP is the primary non-loopback IPv4 of the host, detected once
	// at construction time and used for both node registration and pod
	// status reporting.
	nodeIP string

	// CRI clients. When criClient is nil the kubelet falls back to the
	// legacy fake reconciliation path so existing callers/tests that
	// construct via NewKubelet keep working without a real runtime.
	criSocket   string
	conn        *grpc.ClientConn
	criClient   v1.RuntimeServiceClient
	imageClient v1.ImageServiceClient

	// lastRestart rate-limits container restarts (K14) so a crash-looping
	// container does not get recreated on every watch-driven reconcile. Keyed
	// by "<podKey>/<container>".
	restartMu   sync.Mutex
	lastRestart map[string]time.Time
}

// NewKubelet returns a Kubelet without a CRI connection. It preserves the
// historical fake reconciliation behaviour and is kept for backward
// compatibility; new callers should prefer NewKubeletWithCRI.
func NewKubelet(nodeName string, s store.Store, logger *slog.Logger) *Kubelet {
	return &Kubelet{
		nodeName:    nodeName,
		store:       s,
		pods:        make(map[string]*k8s.Pod),
		running:     make(map[string]bool),
		logger:      logger,
		nodeIP:      detectNodeIP(),
		criSocket:   defaultCRISocket(),
		lastRestart: make(map[string]time.Time),
	}
}

// NewKubeletWithCRI returns a Kubelet that drives a real CRI runtime over
// the given Unix socket. It dials the runtime, builds the
// RuntimeService/ImageService clients and stores them on the struct so
// reconcilePod can issue real RunPodSandbox/CreateContainer/StartContainer
// calls instead of faking pod status.
func NewKubeletWithCRI(ctx context.Context, nodeName string, s store.Store, logger *slog.Logger, criSocket string) (*Kubelet, error) {
	if criSocket == "" {
		criSocket = defaultCRISocket()
	}
	conn, err := grpc.NewClient("unix://"+criSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial CRI socket %q: %w", criSocket, err)
	}
	return &Kubelet{
		nodeName:    nodeName,
		store:       s,
		pods:        make(map[string]*k8s.Pod),
		running:     make(map[string]bool),
		logger:      logger,
		nodeIP:      detectNodeIP(),
		criSocket:   criSocket,
		conn:        conn,
		criClient:   v1.NewRuntimeServiceClient(conn),
		imageClient: v1.NewImageServiceClient(conn),
		lastRestart: make(map[string]time.Time),
	}, nil
}

// Close releases the gRPC connection to the CRI runtime, if any. It is safe
// to call on a Kubelet created via NewKubelet (no CRI).
func (k *Kubelet) Close() error {
	if k.conn != nil {
		return k.conn.Close()
	}
	return nil
}

func (k *Kubelet) Run(ctx context.Context) error {
	k.registerNode()

	go k.watchPods(ctx)
	go k.statusLoop(ctx)

	k.logger.Info("kubelet started", "node", k.nodeName, "cri", k.criSocket, "nodeIP", k.nodeIP)
	<-ctx.Done()
	return nil
}

func (k *Kubelet) registerNode() {
	cpu := strconv.Itoa(runtime.NumCPU())
	mem := detectMemory()
	node := k8s.Node{
		TypeMeta: k8s.TypeMeta{Kind: "Node", APIVersion: "v1"},
		ObjectMeta: k8s.ObjectMeta{
			Name:   k.nodeName,
			Labels: map[string]string{"kubernetes.io/hostname": k.nodeName},
		},
		Spec: k8s.NodeSpec{
			PodCIDR: "10.244.0.0/24",
		},
		Status: k8s.NodeStatus{
			Capacity: k8s.ResourceList{
				"cpu":    cpu,
				"memory": mem,
				"pods":   "110",
			},
			Allocatable: k8s.ResourceList{
				"cpu":    cpu,
				"memory": mem,
				"pods":   "110",
			},
			Conditions: []k8s.NodeCondition{
				{Type: "Ready", Status: "True", LastHeartbeatTime: time.Now(), LastTransitionTime: time.Now(), Reason: "KubeletReady"},
				{Type: "MemoryPressure", Status: "False", LastHeartbeatTime: time.Now()},
				{Type: "DiskPressure", Status: "False", LastHeartbeatTime: time.Now()},
				{Type: "PIDPressure", Status: "False", LastHeartbeatTime: time.Now()},
			},
			NodeInfo: k8s.NodeSystemInfo{
				KubeletVersion:          "v" + common.DokiVersion,
				ContainerRuntimeVersion: "doki://" + common.DokiVersion,
				OperatingSystem:         runtime.GOOS,
				Architecture:            runtime.GOARCH,
			},
			Addresses: []k8s.NodeAddress{
				{Type: "InternalIP", Address: k.nodeIP},
				{Type: "Hostname", Address: k.nodeName},
			},
		},
	}

	data, err := json.Marshal(node)
	if err != nil {
		k.logger.Error("failed to marshal node", "node", k.nodeName, "error", err)
		return
	}
	key := store.KeyFor("", "nodes", "", k.nodeName)
	if err := k.store.Put(key, &store.StoredObject{Value: data}); err != nil {
		k.logger.Error("failed to register node", "node", k.nodeName, "error", err)
	}
}

func (k *Kubelet) watchPods(ctx context.Context) {
	prefix := store.KeyFor("", "pods", "", "")
	ch, err := k.store.Watch(prefix, 0)
	if err != nil {
		k.logger.Error("failed to watch pods", "error", err)
		return
	}
	defer k.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			var pod k8s.Pod
			if err := json.Unmarshal(event.Object.Value, &pod); err != nil {
				k.logger.Warn("failed to unmarshal pod from watch event", "error", err)
				continue
			}
			if pod.Spec.NodeName != "" && pod.Spec.NodeName != k.nodeName {
				continue
			}

			switch event.Type {
			case store.EventAdded, store.EventModified:
				k.reconcilePod(ctx, &pod)
			case store.EventDeleted:
				k.deletePod(&pod)
			}
		}
	}
}

func (k *Kubelet) reconcilePod(ctx context.Context, pod *k8s.Pod) {
	k.mu.Lock()
	defer k.mu.Unlock()

	podKey := podKey(pod)
	k.pods[podKey] = pod

	if k.criClient == nil {
		k.reconcilePodFake(pod, podKey)
		return
	}
	k.reconcilePodCRI(ctx, pod, podKey)
}

// reconcilePodFake reproduces the legacy stub behaviour: it stamps the pod
// as Running with a synthetic 10.244.0.x pod IP and per-container
// "sha256:stub" image IDs. Used when no CRI client is configured.
func (k *Kubelet) reconcilePodFake(pod *k8s.Pod, podKey string) {
	pod.Status.Phase = k8s.PodRunning
	pod.Status.HostIP = k.nodeIP
	pod.Status.PodIP = fmt.Sprintf("10.244.0.%d", len(k.pods)+2)
	now := time.Now()
	pod.Status.StartTime = &now

	statuses := make([]k8s.ContainerStatus, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		running := true
		statuses = append(statuses, k8s.ContainerStatus{
			Name:    c.Name,
			Ready:   true,
			Started: &running,
			State: k8s.ContainerState{
				Running: &k8s.ContainerStateRunning{StartedAt: now},
			},
			Image:        c.Image,
			ImageID:      c.Image + "@sha256:stub",
			RestartCount: 0,
		})
	}
	pod.Status.ContainerStatuses = statuses
	pod.Status.Conditions = []k8s.PodCondition{
		{Type: "Initialized", Status: "True", LastTransitionTime: now},
		{Type: "Ready", Status: "True", LastTransitionTime: now},
		{Type: "ContainersReady", Status: "True", LastTransitionTime: now},
		{Type: "PodScheduled", Status: "True", LastTransitionTime: now},
	}

	k.persistPod(pod, podKey)
	k.running[podKey] = true
	k.logger.Info("pod reconciled (fake)", "pod", podKey, "phase", pod.Status.Phase)
}

// reconcilePodCRI drives a real CRI runtime: it ensures the pod sandbox
// exists (creating it via RunPodSandbox if necessary), creates and starts
// each container, then queries PodSandboxStatus and per-container
// ContainerStatus to populate the pod status with real IPs, state and image
// digests.
func (k *Kubelet) reconcilePodCRI(ctx context.Context, pod *k8s.Pod, podKey string) {
	// Capture the previous conditions (the pod arrives from the watch with the
	// status we last wrote) so LastTransitionTime only advances when a
	// condition's Status actually flips — otherwise every reconcile rewrites the
	// timestamps, the persisted pod always differs, and the kubelet hot-loops.
	prevConditions := pod.Status.Conditions
	labels := sandboxLabels(pod)
	sandboxCfg := &v1.PodSandboxConfig{
		Metadata: &v1.PodSandboxMetadata{
			Name:      pod.Name,
			Uid:       pod.UID,
			Namespace: pod.Namespace,
		},
		Hostname:     pod.Spec.Hostname,
		Labels:       labels,
		Annotations:  pod.Annotations,
		LogDirectory: fmt.Sprintf("/var/log/pods/%s_%s_%s/", pod.Namespace, pod.Name, pod.UID),
	}

	// Look up an existing sandbox by the pod-uid label so we don't double
	// create sandboxes across kubelet restarts.
	sandboxID := ""
	listResp, err := k.criClient.ListPodSandbox(ctx, &v1.ListPodSandboxRequest{
		Filter: &v1.PodSandboxFilter{LabelSelector: labels},
	})
	if err != nil {
		k.logger.Error("cri: list pod sandbox", "pod", podKey, "error", err)
		pod.Status.Phase = k8s.PodPending
		k.persistPod(pod, podKey)
		return
	}
	if len(listResp.GetItems()) > 0 {
		sandboxID = listResp.GetItems()[0].GetId()
	} else {
		runResp, err := k.criClient.RunPodSandbox(ctx, &v1.RunPodSandboxRequest{Config: sandboxCfg})
		if err != nil {
			k.logger.Error("cri: run pod sandbox", "pod", podKey, "error", err)
			pod.Status.Phase = k8s.PodPending
			k.persistPod(pod, podKey)
			return
		}
		sandboxID = runResp.GetPodSandboxId()
	}

	// Discover containers already present in the sandbox (e.g. after a
	// kubelet restart) so we don't try to recreate them.
	existing := map[string]string{} // container name -> container ID
	listCResp, err := k.criClient.ListContainers(ctx, &v1.ListContainersRequest{
		Filter: &v1.ContainerFilter{PodSandboxId: sandboxID},
	})
	if err != nil {
		k.logger.Warn("cri: list containers", "pod", podKey, "error", err)
	} else {
		for _, c := range listCResp.GetContainers() {
			if md := c.GetMetadata(); md != nil {
				existing[md.GetName()] = c.GetId()
			}
		}
	}

	// Carry each container's restart count forward from the last-written status
	// so it survives across reconciles (K14). RestartCount was previously
	// hardcoded to 0.
	restartCounts := map[string]int32{}
	for _, cs := range pod.Status.ContainerStatuses {
		restartCounts[cs.Name] = cs.RestartCount
	}
	policy := podRestartPolicy(pod)

	mounts := k.criMounts(pod)
	for _, c := range pod.Spec.Containers {
		containerID, ok := existing[c.Name]

		// K14: an already-created container that has EXITED may need to be
		// restarted per the pod's restartPolicy. Remove the dead container and
		// fall through to recreate it, rate-limited so a crash loop does not
		// spin.
		if ok {
			if st, serr := k.criClient.ContainerStatus(ctx, &v1.ContainerStatusRequest{ContainerId: containerID}); serr == nil {
				cst := st.GetStatus()
				if cst.GetState() == v1.ContainerState_CONTAINER_EXITED &&
					shouldRestart(policy, cst.GetExitCode()) &&
					k.restartBackoffElapsed(podKey, c.Name, restartCounts[c.Name]) {
					_, _ = k.criClient.RemoveContainer(ctx, &v1.RemoveContainerRequest{ContainerId: containerID})
					delete(existing, c.Name)
					ok = false
					restartCounts[c.Name]++
					k.markRestart(podKey, c.Name)
					k.logger.Info("restarting exited container", "pod", podKey, "container", c.Name, "restartCount", restartCounts[c.Name], "policy", policy)
				}
			}
		}

		if !ok {
			createResp, err := k.criClient.CreateContainer(ctx, &v1.CreateContainerRequest{
				PodSandboxId:  sandboxID,
				SandboxConfig: sandboxCfg,
				Config:        containerConfig(pod, c, mounts),
			})
			if err != nil {
				k.logger.Error("cri: create container", "pod", podKey, "container", c.Name, "error", err)
				continue
			}
			containerID = createResp.GetContainerId()
			existing[c.Name] = containerID
		}

		// Only start containers that haven't been started yet. We fetch
		// the current state first so a restart of the kubelet doesn't
		// attempt to start an already-running container.
		if cs, err := k.criClient.ContainerStatus(ctx, &v1.ContainerStatusRequest{ContainerId: containerID}); err == nil {
			if cs.GetStatus().GetState() == v1.ContainerState_CONTAINER_CREATED {
				if _, err := k.criClient.StartContainer(ctx, &v1.StartContainerRequest{ContainerId: containerID}); err != nil {
					k.logger.Error("cri: start container", "pod", podKey, "container", c.Name, "error", err)
				}
			}
		}
	}

	// Fetch the real sandbox status for the pod IP and phase.
	sandboxStatus, err := k.criClient.PodSandboxStatus(ctx, &v1.PodSandboxStatusRequest{PodSandboxId: sandboxID})
	if err != nil {
		k.logger.Error("cri: pod sandbox status", "pod", podKey, "error", err)
		pod.Status.Phase = k8s.PodPending
		k.persistPod(pod, podKey)
		return
	}

	now := time.Now()
	pod.Status.HostIP = k.nodeIP
	if net := sandboxStatus.GetStatus().GetNetwork(); net != nil {
		pod.Status.PodIP = net.GetIp()
	}
	if createdAt := sandboxStatus.GetStatus().GetCreatedAt(); createdAt > 0 {
		t := time.Unix(0, createdAt).UTC()
		pod.Status.StartTime = &t
	} else {
		pod.Status.StartTime = &now
	}

	// Aggregate per-container real status.
	statuses := make([]k8s.ContainerStatus, 0, len(pod.Spec.Containers))
	allRunning := true
	allExited0 := true
	anyExitedNon0 := false
	allReady := true

	for _, c := range pod.Spec.Containers {
		cs := k8s.ContainerStatus{Name: c.Name, Image: c.Image, RestartCount: restartCounts[c.Name]}

		containerID, ok := existing[c.Name]
		if !ok {
			// Container create failed earlier in this reconcile pass;
			// look it up by sandbox+name in case the runtime still has it.
			containerID = k.findContainerID(ctx, sandboxID, c.Name)
		}

		if containerID == "" {
			allRunning = false
			allExited0 = false
			allReady = false
			cs.State = k8s.ContainerState{Waiting: &k8s.ContainerStateWaiting{Reason: "ContainerNotCreated"}}
			statuses = append(statuses, cs)
			continue
		}

		cs.ContainerID = containerID
		resp, err := k.criClient.ContainerStatus(ctx, &v1.ContainerStatusRequest{ContainerId: containerID})
		if err != nil {
			k.logger.Warn("cri: container status", "pod", podKey, "container", c.Name, "error", err)
			allRunning = false
			allExited0 = false
			allReady = false
			cs.State = k8s.ContainerState{Waiting: &k8s.ContainerStateWaiting{Reason: "StatusUnknown"}}
			statuses = append(statuses, cs)
			continue
		}

		st := resp.GetStatus()
		if id := st.GetImageId(); id != "" {
			cs.ImageID = id
		} else {
			cs.ImageID = st.GetImageRef()
		}

		switch st.GetState() {
		case v1.ContainerState_CONTAINER_RUNNING:
			running := true
			cs.Started = &running
			cs.State = k8s.ContainerState{Running: &k8s.ContainerStateRunning{StartedAt: time.Unix(0, st.GetStartedAt()).UTC()}}
			allExited0 = false
			// K13: a container is Ready only when its readiness probe passes.
			// Without a probe, "running" is ready, matching Kubernetes. Service
			// endpoints depend on this, so a running-but-not-ready pod must not
			// be marked ready.
			cs.Ready = k.containerReady(ctx, pod, c, containerID, st.GetStartedAt())
			if !cs.Ready {
				allReady = false
			}
		case v1.ContainerState_CONTAINER_EXITED:
			allRunning = false
			allReady = false
			cs.State = k8s.ContainerState{Terminated: &k8s.ContainerStateTerminated{
				ExitCode:    st.GetExitCode(),
				Reason:      st.GetReason(),
				Message:     st.GetMessage(),
				StartedAt:   time.Unix(0, st.GetStartedAt()).UTC(),
				FinishedAt:  time.Unix(0, st.GetFinishedAt()).UTC(),
				ContainerID: containerID,
			}}
			if st.GetExitCode() != 0 {
				anyExitedNon0 = true
				allExited0 = false
			}
		case v1.ContainerState_CONTAINER_CREATED:
			allRunning = false
			allExited0 = false
			allReady = false
			cs.State = k8s.ContainerState{Waiting: &k8s.ContainerStateWaiting{Reason: "ContainerCreated"}}
		default: // CONTAINER_UNKNOWN or unspecified
			allRunning = false
			allExited0 = false
			allReady = false
			cs.State = k8s.ContainerState{Waiting: &k8s.ContainerStateWaiting{Reason: "Unknown"}}
		}
		statuses = append(statuses, cs)
	}
	pod.Status.ContainerStatuses = statuses

	// Derive the pod phase from the aggregate sandbox + container states, taking
	// the restart policy into account (K14): a pod that will restart its
	// containers must not be reported as terminally Failed/Succeeded.
	switch {
	case sandboxStatus.GetStatus().GetState() != v1.PodSandboxState_SANDBOX_READY:
		pod.Status.Phase = k8s.PodPending
	case len(statuses) == 0:
		pod.Status.Phase = k8s.PodPending
	case anyExitedNon0 && policy == restartNever:
		pod.Status.Phase = k8s.PodFailed
	case allExited0 && policy != restartAlways:
		pod.Status.Phase = k8s.PodSucceeded
	case allRunning:
		pod.Status.Phase = k8s.PodRunning
	default:
		// Containers are being (re)started, or a crash-looping container is in
		// backoff — still an active pod, not terminal.
		pod.Status.Phase = k8s.PodPending
	}

	// Ready reflects readiness probes (K13), not merely "running". Service
	// endpoint controllers gate traffic on this condition.
	containersReady := allRunning && allReady
	pod.Status.Conditions = []k8s.PodCondition{
		{Type: "Initialized", Status: "True", LastTransitionTime: condTime(prevConditions, "Initialized", "True", now)},
		{Type: "Ready", Status: boolStatus(containersReady), LastTransitionTime: condTime(prevConditions, "Ready", boolStatus(containersReady), now)},
		{Type: "ContainersReady", Status: boolStatus(containersReady), LastTransitionTime: condTime(prevConditions, "ContainersReady", boolStatus(containersReady), now)},
		{Type: "PodScheduled", Status: "True", LastTransitionTime: condTime(prevConditions, "PodScheduled", "True", now)},
	}

	k.persistPod(pod, podKey)
	k.running[podKey] = pod.Status.Phase == k8s.PodRunning
	k.logger.Info("pod reconciled (cri)", "pod", podKey, "phase", pod.Status.Phase, "podIP", pod.Status.PodIP)
}

// findContainerID lists containers in a sandbox and returns the ID of the
// one whose metadata name matches. It is a fallback for the rare case where
// CreateContainer failed but the runtime still ended up creating the
// container.
func (k *Kubelet) findContainerID(ctx context.Context, sandboxID, name string) string {
	resp, err := k.criClient.ListContainers(ctx, &v1.ListContainersRequest{
		Filter: &v1.ContainerFilter{PodSandboxId: sandboxID},
	})
	if err != nil {
		return ""
	}
	for _, c := range resp.GetContainers() {
		if md := c.GetMetadata(); md != nil && md.GetName() == name {
			return c.GetId()
		}
	}
	return ""
}

// persistPod serialises the pod and writes it back to the store, logging
// any error instead of silently dropping it.
// condTime returns the LastTransitionTime a condition should carry: the prior
// timestamp if the condition already existed with the same status (so it stays
// stable across reconciles), otherwise now (the status just changed).
func condTime(prev []k8s.PodCondition, condType, status string, now time.Time) time.Time {
	for _, c := range prev {
		if c.Type == condType {
			if c.Status == status && !c.LastTransitionTime.IsZero() {
				return c.LastTransitionTime
			}
			break
		}
	}
	return now
}

func (k *Kubelet) persistPod(pod *k8s.Pod, podKey string) {
	data, err := json.Marshal(pod)
	if err != nil {
		k.logger.Error("failed to marshal pod", "pod", podKey, "error", err)
		return
	}
	key := store.KeyFor("", "pods", pod.Namespace, pod.Name)
	// Skip the write when nothing changed. Our own Put fires a watch event that
	// re-enters reconcilePod; without this guard the kubelet spins in a hot loop
	// re-persisting an identical pod status thousands of times a second.
	if existing, gerr := k.store.Get(key); gerr == nil && existing != nil && bytes.Equal(existing.Value, data) {
		return
	}
	if err := k.store.Put(key, &store.StoredObject{Value: data}); err != nil {
		k.logger.Error("failed to persist pod", "pod", podKey, "error", err)
	}
}

func (k *Kubelet) deletePod(pod *k8s.Pod) {
	k.mu.Lock()
	defer k.mu.Unlock()

	podKey := podKey(pod)
	delete(k.pods, podKey)
	delete(k.running, podKey)

	k.logger.Info("pod deleted", "pod", podKey)
}

func (k *Kubelet) statusLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			k.heartbeat()
		}
	}
}

func (k *Kubelet) heartbeat() {
	key := store.KeyFor("", "nodes", "", k.nodeName)
	obj, err := k.store.Get(key)
	if err != nil {
		k.logger.Warn("heartbeat: failed to read node", "node", k.nodeName, "error", err)
		return
	}

	var node k8s.Node
	if err := json.Unmarshal(obj.Value, &node); err != nil {
		k.logger.Error("heartbeat: failed to unmarshal node", "node", k.nodeName, "error", err)
		return
	}

	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == "Ready" {
			node.Status.Conditions[i].Status = "True"
			node.Status.Conditions[i].LastHeartbeatTime = time.Now()
		}
	}

	data, err := json.Marshal(node)
	if err != nil {
		k.logger.Error("heartbeat: failed to marshal node", "node", k.nodeName, "error", err)
		return
	}
	if err := k.store.Put(key, &store.StoredObject{Value: data}); err != nil {
		k.logger.Error("heartbeat: failed to persist node", "node", k.nodeName, "error", err)
	}
}

func (k *Kubelet) RunningPods() []*k8s.Pod {
	k.mu.RLock()
	defer k.mu.RUnlock()

	pods := make([]*k8s.Pod, 0)
	for _, pod := range k.pods {
		pods = append(pods, pod)
	}
	return pods
}

func podKey(pod *k8s.Pod) string {
	if pod.Namespace != "" {
		return pod.Namespace + "/" + pod.Name
	}
	return pod.Name
}

// ---- helpers ---------------------------------------------------------------

// sandboxLabels returns the set of Kubernetes-standard labels the kubelet
// stamps onto a CRI pod sandbox. The io.kubernetes.pod.uid label is what we
// use to find an existing sandbox across restarts.
func sandboxLabels(pod *k8s.Pod) map[string]string {
	return map[string]string{
		"io.kubernetes.pod.uid":       pod.UID,
		"io.kubernetes.pod.name":      pod.Name,
		"io.kubernetes.pod.namespace": pod.Namespace,
		"io.kubernetes.pod.nodeName":  pod.Spec.NodeName,
	}
}

// containerConfig builds the CRI ContainerConfig for a single pod container.
func containerConfig(pod *k8s.Pod, c k8s.Container, mounts []*v1.Mount) *v1.ContainerConfig {
	envs := make([]*v1.KeyValue, 0, len(c.Env))
	for _, e := range c.Env {
		if e.ValueFrom != nil {
			continue
		}
		envs = append(envs, &v1.KeyValue{Key: e.Name, Value: []byte(e.Value)})
	}
	return &v1.ContainerConfig{
		Metadata:   &v1.ContainerMetadata{Name: c.Name},
		Image:      &v1.ImageSpec{Image: c.Image},
		Command:    c.Command,
		Args:       c.Args,
		WorkingDir: c.WorkingDir,
		Envs:       envs,
		Mounts:     mounts,
		Labels: map[string]string{
			"io.kubernetes.pod.uid":        pod.UID,
			"io.kubernetes.pod.name":       pod.Name,
			"io.kubernetes.pod.namespace":  pod.Namespace,
			"io.kubernetes.container.name": c.Name,
		},
		Annotations: pod.Annotations,
		LogPath:     fmt.Sprintf("%s/0.log", c.Name),
	}
}

// criMounts resolves a pod's VolumeMounts into CRI Mount entries using the
// pod's declared Volumes. HostPath maps to its host path; EmptyDir to a per-pod
// directory; PVC to its bound PV; and ConfigMap/Secret are projected to files
// on disk (K12) — previously these were skipped, so a container that mounted
// its config as a volume started and then misbehaved silently.
func (k *Kubelet) criMounts(pod *k8s.Pod) []*v1.Mount {
	hostPaths := make(map[string]string, len(pod.Spec.Volumes))
	for _, vol := range pod.Spec.Volumes {
		switch {
		case vol.HostPath != nil:
			hostPaths[vol.Name] = vol.HostPath.Path
		case vol.EmptyDir != nil:
			// The source must exist before proot/bind can mount it; the old
			// code referenced this path but never created it.
			dir := filepath.Join(common.AppDataDir(), "emptydir", pod.UID, vol.Name)
			if err := os.MkdirAll(dir, 0o777); err != nil {
				k.logger.Warn("emptyDir mkdir", "pod", pod.Name, "vol", vol.Name, "err", err)
				continue
			}
			hostPaths[vol.Name] = dir
		case vol.PersistentVolumeClaim != nil:
			if p := k.resolvePVCHostPath(pod.Namespace, vol.PersistentVolumeClaim.ClaimName); p != "" {
				hostPaths[vol.Name] = p
			} else {
				k.logger.Info("PVC not bound yet; pod volume deferred", "pod", pod.Name, "claim", vol.PersistentVolumeClaim.ClaimName)
			}
		case vol.ConfigMap != nil:
			if dir := k.projectConfigMap(pod, vol); dir != "" {
				hostPaths[vol.Name] = dir
			}
		case vol.Secret != nil:
			if dir := k.projectSecret(pod, vol); dir != "" {
				hostPaths[vol.Name] = dir
			}
		}
	}

	mounts := make([]*v1.Mount, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		for _, vm := range c.VolumeMounts {
			host, ok := hostPaths[vm.Name]
			if !ok {
				continue
			}
			mounts = append(mounts, &v1.Mount{
				ContainerPath: vm.MountPath,
				HostPath:      host,
				Readonly:      vm.ReadOnly,
			})
		}
	}
	return mounts
}

// projectConfigMap materializes a ConfigMap volume as a directory of files on
// disk (one file per data key), returning the directory to bind into the
// container. Honors an explicit items[] mapping and the volume's optional flag.
func (k *Kubelet) projectConfigMap(pod *k8s.Pod, vol k8s.Volume) string {
	src := vol.ConfigMap
	obj, err := k.store.Get(store.KeyFor("", "configmaps", pod.Namespace, src.Name))
	if err != nil || obj == nil {
		if src.Optional == nil || !*src.Optional {
			k.logger.Warn("configMap volume not found", "pod", pod.Name, "configMap", src.Name)
		}
		return ""
	}
	var cm k8s.ConfigMap
	if json.Unmarshal(obj.Value, &cm) != nil {
		return ""
	}
	files := map[string][]byte{}
	for key, val := range cm.Data {
		files[key] = []byte(val)
	}
	for key, val := range cm.BinaryData {
		files[key] = val
	}
	return k.writeProjectedFiles(pod, vol.Name, files, src.Items)
}

// projectSecret materializes a Secret volume as a directory of files on disk.
// Secret Data is base64-decoded by the JSON unmarshal into []byte; StringData
// is written verbatim.
func (k *Kubelet) projectSecret(pod *k8s.Pod, vol k8s.Volume) string {
	src := vol.Secret
	obj, err := k.store.Get(store.KeyFor("", "secrets", pod.Namespace, src.SecretName))
	if err != nil || obj == nil {
		if src.Optional == nil || !*src.Optional {
			k.logger.Warn("secret volume not found", "pod", pod.Name, "secret", src.SecretName)
		}
		return ""
	}
	var sec k8s.Secret
	if json.Unmarshal(obj.Value, &sec) != nil {
		return ""
	}
	files := map[string][]byte{}
	for key, val := range sec.Data {
		files[key] = val
	}
	for key, val := range sec.StringData {
		files[key] = []byte(val)
	}
	return k.writeProjectedFiles(pod, vol.Name, files, src.Items)
}

// writeProjectedFiles writes the given key/value pairs as files under a per-pod
// volume directory and returns that directory. An items[] mapping, when
// present, restricts and renames the projected keys the way Kubernetes does.
func (k *Kubelet) writeProjectedFiles(pod *k8s.Pod, volName string, data map[string][]byte, items []k8s.KeyToPath) string {
	dir := filepath.Join(common.AppDataDir(), "projected", pod.UID, volName)
	// Rebuild from scratch each reconcile so removed keys do not linger.
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		k.logger.Warn("projected volume mkdir", "pod", pod.Name, "vol", volName, "err", err)
		return ""
	}

	write := func(relPath string, content []byte) {
		full := filepath.Join(dir, filepath.Clean("/"+relPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			k.logger.Warn("projected volume subdir", "pod", pod.Name, "path", relPath, "err", err)
			return
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			k.logger.Warn("projected volume write", "pod", pod.Name, "path", relPath, "err", err)
		}
	}

	if len(items) > 0 {
		// Explicit selection: only the named keys, at their mapped paths.
		for _, it := range items {
			if content, ok := data[it.Key]; ok {
				dest := it.Path
				if dest == "" {
					dest = it.Key
				}
				write(dest, content)
			}
		}
	} else {
		for key, content := range data {
			write(key, content)
		}
	}
	return dir
}

// resolvePVCHostPath resolves a PVC name to the host directory of its bound PV
// (created by the PVC provisioner controller). Returns "" if the claim is not
// yet bound, so the pod's volume is retried on the next reconcile.
func (k *Kubelet) resolvePVCHostPath(namespace, claimName string) string {
	if k.store == nil {
		return ""
	}
	obj, err := k.store.Get(store.KeyFor("", "persistentvolumeclaims", namespace, claimName))
	if err != nil || obj == nil {
		return ""
	}
	var pvc k8s.PersistentVolumeClaim
	if json.Unmarshal(obj.Value, &pvc) != nil || pvc.Spec.VolumeName == "" {
		return ""
	}
	pvObj, err := k.store.Get(store.KeyFor("", "persistentvolumes", "", pvc.Spec.VolumeName))
	if err != nil || pvObj == nil {
		return ""
	}
	var pv k8s.PersistentVolume
	if json.Unmarshal(pvObj.Value, &pv) != nil || pv.Spec.HostPath == nil || pv.Spec.HostPath.Path == "" {
		return ""
	}
	_ = os.MkdirAll(pv.Spec.HostPath.Path, 0o777)
	return pv.Spec.HostPath.Path
}

func boolStatus(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// detectNodeIP returns the first non-loopback IPv4 address of the host, or
// 127.0.0.1 if none can be found.
func detectNodeIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "127.0.0.1"
}

// detectMemory returns the host's total memory as a Kubernetes-style
// quantity string (e.g. "8Gi"). It reads /proc/meminfo on Linux and falls
// back to "8Gi" when the file is unavailable.
func detectMemory() string {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "8Gi"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			break
		}
		ki, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			break
		}
		bytes := ki * 1024
		gi := bytes / (1 << 30)
		if gi == 0 {
			return strconv.FormatUint(bytes/(1<<20), 10) + "Mi"
		}
		return strconv.FormatUint(gi, 10) + "Gi"
	}
	return "8Gi"
}
