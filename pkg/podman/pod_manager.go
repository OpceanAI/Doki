package podman

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

type PodManager struct {
	mu     sync.RWMutex
	pods   map[string]*Pod
	store  string
}

func NewPodManager(root string) *PodManager {
	pm := &PodManager{
		pods:  make(map[string]*Pod),
		store: filepath.Join(root, "pods"),
	}
	_ = os.MkdirAll(pm.store, 0755)
	pm.loadPods()
	return pm
}

func (pm *PodManager) CreatePod(cfg *PodCreateConfig) (*Pod, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, existing := range pm.pods {
		if existing.Name == cfg.Name {
			return nil, fmt.Errorf("pod name %q already in use", cfg.Name)
		}
	}

	infraImage := cfg.InfraImage
	if infraImage == "" {
		infraImage = "registry.k8s.io/pause:3.9"
	}

	exitPolicy := cfg.ExitPolicy
	if exitPolicy == "" {
		exitPolicy = "continue"
	}

	sharedNS := cfg.ShareNamespaces
	if len(sharedNS) == 0 {
		sharedNS = []string{"ipc", "net", "uts"}
	}

	pod := &Pod{
		ID:         generateID(),
		Name:       cfg.Name,
		Created:    time.Now(),
		Hostname:   cfg.Hostname,
		Labels:     cfg.Labels,
		State:      "Created",
		Containers: []PodContainer{},
		InfraID:    generateID(),
		SharedNS:   sharedNS,
		ExitPolicy: exitPolicy,
		CPUShares:  cfg.CPUShares,
		MemoryLimit: cfg.MemoryLimit,
		Restart:    cfg.Restart,
	}

	pod.Containers = append(pod.Containers, PodContainer{
		ID:    pod.InfraID,
		Name:  cfg.InfraName,
		State: "running",
	})

	pm.pods[pod.ID] = pod
	pm.savePod(pod)

	return pod, nil
}

func (pm *PodManager) GetPod(nameOrID string) (*Pod, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pod, ok := pm.pods[nameOrID]; ok {
		return pod, nil
	}

	for _, pod := range pm.pods {
		if pod.Name == nameOrID {
			return pod, nil
		}
	}

	return nil, fmt.Errorf("pod %s not found", nameOrID)
}

func (pm *PodManager) ListPods() []*Pod {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pods := make([]*Pod, 0, len(pm.pods))
	for _, pod := range pm.pods {
		pods = append(pods, pod)
	}
	return pods
}

func (pm *PodManager) StartPod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Running"
	pm.savePod(pod)
	return nil
}

func (pm *PodManager) StopPod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Stopped"
	pm.savePod(pod)
	return nil
}

func (pm *PodManager) RestartPod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Running"
	pm.savePod(pod)
	return nil
}

func (pm *PodManager) KillPod(nameOrID string, signal string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Exited"
	pm.savePod(pod)
	return nil
}

func (pm *PodManager) PausePod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Paused"
	pm.savePod(pod)
	return nil
}

func (pm *PodManager) UnpausePod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Running"
	pm.savePod(pod)
	return nil
}

func (pm *PodManager) RemovePod(nameOrID string, force bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	if pod.State == "Running" && !force {
		return fmt.Errorf("pod %s is running, use force to remove", nameOrID)
	}

	delete(pm.pods, pod.ID)
	pm.removePodFile(pod.ID)
	return nil
}

func (pm *PodManager) RenamePod(nameOrID, newName string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	for _, existing := range pm.pods {
		if existing.Name == newName && existing.ID != pod.ID {
			return fmt.Errorf("pod name %q already in use", newName)
		}
	}

	pod.Name = newName
	pm.savePod(pod)
	return nil
}

func (pm *PodManager) PrunePods() []string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var removed []string
	for id, pod := range pm.pods {
		if pod.State == "Exited" || pod.State == "Stopped" {
			delete(pm.pods, id)
			pm.removePodFile(id)
			removed = append(removed, id)
		}
	}
	return removed
}

func (pm *PodManager) Exists(nameOrID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, err := pm.findPodLocked(nameOrID)
	return err == nil
}

func (pm *PodManager) AddContainerToPod(podID string, containerID, containerName string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(podID)
	if err != nil {
		return err
	}

	pod.Containers = append(pod.Containers, PodContainer{
		ID:    containerID,
		Name:  containerName,
		State: "created",
	})

	pm.savePod(pod)
	return nil
}

func (pm *PodManager) findPodLocked(nameOrID string) (*Pod, error) {
	if pod, ok := pm.pods[nameOrID]; ok {
		return pod, nil
	}
	for _, pod := range pm.pods {
		if pod.Name == nameOrID {
			return pod, nil
		}
	}
	return nil, fmt.Errorf("pod %s not found", nameOrID)
}

func (pm *PodManager) savePod(pod *Pod) {
	data, _ := json.MarshalIndent(pod, "", "  ")
	_ = os.WriteFile(filepath.Join(pm.store, pod.ID+".json"), data, 0644)
}

func (pm *PodManager) removePodFile(id string) {
	_ = os.Remove(filepath.Join(pm.store, id+".json"))
}

func (pm *PodManager) loadPods() {
	entries, err := os.ReadDir(pm.store)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(pm.store, entry.Name()))
		if err != nil {
			continue
		}
		var pod Pod
		if err := json.Unmarshal(data, &pod); err != nil {
			continue
		}
		pm.pods[pod.ID] = &pod
	}
}

func generateID() string {
	return common.GenerateID(12)
}
