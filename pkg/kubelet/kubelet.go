package kubelet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

type Kubelet struct {
	nodeName    string
	store       store.Store
	mu          sync.RWMutex
	pods        map[string]*k8s.Pod
	running     map[string]bool
	logger      *slog.Logger
}

func NewKubelet(nodeName string, s store.Store, logger *slog.Logger) *Kubelet {
	return &Kubelet{
		nodeName: nodeName,
		store:    s,
		pods:     make(map[string]*k8s.Pod),
		running:  make(map[string]bool),
		logger:   logger,
	}
}

func (k *Kubelet) Run(ctx context.Context) error {
	k.registerNode()

	go k.watchPods(ctx)
	go k.statusLoop(ctx)

	k.logger.Info("kubelet started", "node", k.nodeName)
	<-ctx.Done()
	return nil
}

func (k *Kubelet) registerNode() {
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
				"cpu":    "4",
				"memory": "8Gi",
				"pods":   "110",
			},
			Allocatable: k8s.ResourceList{
				"cpu":    "3800m",
				"memory": "7Gi",
				"pods":   "110",
			},
			Conditions: []k8s.NodeCondition{
				{Type: "Ready", Status: "True", LastHeartbeatTime: time.Now(), LastTransitionTime: time.Now(), Reason: "KubeletReady"},
				{Type: "MemoryPressure", Status: "False", LastHeartbeatTime: time.Now()},
				{Type: "DiskPressure", Status: "False", LastHeartbeatTime: time.Now()},
				{Type: "PIDPressure", Status: "False", LastHeartbeatTime: time.Now()},
			},
			NodeInfo: k8s.NodeSystemInfo{
				KubeletVersion:          "v0.10.0",
				ContainerRuntimeVersion: "doki://0.10.0",
				OperatingSystem:         "linux",
				Architecture:            "arm64",
			},
			Addresses: []k8s.NodeAddress{
				{Type: "InternalIP", Address: "127.0.0.1"},
				{Type: "Hostname", Address: k.nodeName},
			},
		},
	}

	data, _ := json.Marshal(node)
	key := store.KeyFor("", "nodes", "", k.nodeName)
	_ = k.store.Put(key, &store.StoredObject{Value: data})
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
				continue
			}
			if pod.Spec.NodeName != "" && pod.Spec.NodeName != k.nodeName {
				continue
			}

			switch event.Type {
			case store.EventAdded, store.EventModified:
				k.reconcilePod(&pod)
			case store.EventDeleted:
				k.deletePod(&pod)
			}
		}
	}
}

func (k *Kubelet) reconcilePod(pod *k8s.Pod) {
	k.mu.Lock()
	defer k.mu.Unlock()

	podKey := podKey(pod)
	k.pods[podKey] = pod

	pod.Status.Phase = k8s.PodRunning
	pod.Status.HostIP = "127.0.0.1"
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
			Image:    c.Image,
			ImageID:  c.Image + "@sha256:stub",
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

	data, _ := json.Marshal(pod)
	key := store.KeyFor("", "pods", pod.Namespace, pod.Name)
	_ = k.store.Put(key, &store.StoredObject{Value: data})
	k.running[podKey] = true

	k.logger.Info("pod reconciled", "pod", podKey, "phase", pod.Status.Phase)
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
		return
	}

	var node k8s.Node
	_ = json.Unmarshal(obj.Value, &node)

	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == "Ready" {
			node.Status.Conditions[i].Status = "True"
			node.Status.Conditions[i].LastHeartbeatTime = time.Now()
		}
	}

	data, _ := json.Marshal(node)
	_ = k.store.Put(key, &store.StoredObject{Value: data})
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
