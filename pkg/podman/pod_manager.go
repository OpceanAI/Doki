package podman

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

type PodManager struct {
	mu    sync.RWMutex
	pods  map[string]*Pod
	store string
}

func NewPodManager(root string) (*PodManager, error) {
	pm := &PodManager{
		pods:  make(map[string]*Pod),
		store: filepath.Join(root, "pods"),
	}
	if err := os.MkdirAll(pm.store, 0750); err != nil {
		return nil, fmt.Errorf("podman: create pod store %s: %w", pm.store, err)
	}
	pm.loadPods()
	return pm, nil
}

func (pm *PodManager) CreatePod(cfg *PodCreateConfig) (*Pod, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if cfg == nil {
		return nil, fmt.Errorf("pod config is required")
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("pod name is required")
	}
	if !common.ValidContainerName(cfg.Name) {
		return nil, fmt.Errorf("pod name %q contains invalid characters", cfg.Name)
	}

	for _, existing := range pm.pods {
		if existing.Name == cfg.Name {
			return nil, fmt.Errorf("pod name %q already in use", cfg.Name)
		}
	}

	infraImage := cfg.InfraImage
	if infraImage == "" {
		infraImage = "registry.k8s.io/pause:3.9"
	}
	_ = infraImage // retained for future infra container creation

	exitPolicy := cfg.ExitPolicy
	if exitPolicy == "" {
		exitPolicy = "continue"
	}

	sharedNS := cfg.ShareNamespaces
	if len(sharedNS) == 0 {
		sharedNS = []string{"ipc", "net", "uts"}
	}

	pod := &Pod{
		ID:          generateID(),
		Name:        cfg.Name,
		Created:     time.Now(),
		Hostname:    cfg.Hostname,
		Labels:      cfg.Labels,
		State:       "Created",
		Containers:  []PodContainer{},
		InfraID:     generateID(),
		SharedNS:    sharedNS,
		ExitPolicy:  exitPolicy,
		CPUShares:   cfg.CPUShares,
		MemoryLimit: cfg.MemoryLimit,
		Restart:     cfg.Restart,
	}

	// Only create an infra container if NoInfra is not set.
	if !cfg.NoInfra {
		infraName := cfg.InfraName
		if infraName == "" {
			infraName = cfg.Name + "-infra"
		}
		pod.Containers = append(pod.Containers, PodContainer{
			ID:    pod.InfraID,
			Name:  infraName,
			State: "running",
		})
	}

	pm.pods[pod.ID] = pod
	if err := pm.savePod(pod); err != nil {
		delete(pm.pods, pod.ID)
		return nil, err
	}

	return pod, nil
}

func (pm *PodManager) GetPod(nameOrID string) (*Pod, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var pod *Pod
	if p, ok := pm.pods[nameOrID]; ok {
		pod = p
	} else {
		for _, p := range pm.pods {
			if p.Name == nameOrID {
				pod = p
				break
			}
		}
	}
	if pod == nil {
		return nil, fmt.Errorf("pod %s not found", nameOrID)
	}
	// Return a deep copy to avoid data race.
	cp := *pod
	if pod.Containers != nil {
		cp.Containers = make([]PodContainer, len(pod.Containers))
		copy(cp.Containers, pod.Containers)
	}
	if pod.Labels != nil {
		cp.Labels = make(map[string]string, len(pod.Labels))
		for k, v := range pod.Labels {
			cp.Labels[k] = v
		}
	}
	return &cp, nil
}

func (pm *PodManager) ListPods() []*Pod {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pods := make([]*Pod, 0, len(pm.pods))
	for _, pod := range pm.pods {
		cp := *pod
		if pod.Containers != nil {
			cp.Containers = make([]PodContainer, len(pod.Containers))
			copy(cp.Containers, pod.Containers)
		}
		if pod.Labels != nil {
			cp.Labels = make(map[string]string, len(pod.Labels))
			for k, v := range pod.Labels {
				cp.Labels[k] = v
			}
		}
		pods = append(pods, &cp)
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
	return pm.savePod(pod)
}

func (pm *PodManager) StopPod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Stopped"
	return pm.savePod(pod)
}

func (pm *PodManager) RestartPod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Running"
	return pm.savePod(pod)
}

func (pm *PodManager) KillPod(nameOrID string, signal string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	// Record the signal used (for inspection). The actual signal
	// delivery to containers happens when wired to the runtime.
	if signal == "" {
		signal = "SIGTERM"
	}
	pod.State = "Exited"
	pod.ExitSignal = signal
	return pm.savePod(pod)
}

func (pm *PodManager) PausePod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Paused"
	return pm.savePod(pod)
}

func (pm *PodManager) UnpausePod(nameOrID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(nameOrID)
	if err != nil {
		return err
	}

	pod.State = "Running"
	return pm.savePod(pod)
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
	return pm.removePodFile(pod.ID)
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
	return pm.savePod(pod)
}

func (pm *PodManager) PrunePods() []string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var removed []string
	for id, pod := range pm.pods {
		// Only prune pods that are Exited or Stopped AND have no
		// running containers. A stopped pod with running app
		// containers should not be pruned (would orphan them).
		if pod.State != "Exited" && pod.State != "Stopped" {
			continue
		}
		hasRunning := false
		for _, c := range pod.Containers {
			if c.State == "running" || c.State == "Running" {
				hasRunning = true
				break
			}
		}
		if hasRunning {
			continue
		}
		delete(pm.pods, id)
		if err := pm.removePodFile(id); err != nil {
			// Log but continue pruning other pods.
			_ = err
		}
		removed = append(removed, id)
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

	return pm.savePod(pod)
}

// RemoveContainerFromPod removes a container from a pod by ID or name.
func (pm *PodManager) RemoveContainerFromPod(podID, containerIDOrName string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, err := pm.findPodLocked(podID)
	if err != nil {
		return err
	}

	filtered := make([]PodContainer, 0, len(pod.Containers))
	removed := false
	for _, c := range pod.Containers {
		if c.ID == containerIDOrName || c.Name == containerIDOrName {
			removed = true
			continue
		}
		filtered = append(filtered, c)
	}
	if !removed {
		return fmt.Errorf("container %s not found in pod %s", containerIDOrName, podID)
	}
	pod.Containers = filtered
	return pm.savePod(pod)
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

func (pm *PodManager) savePod(pod *Pod) error {
	data, err := json.MarshalIndent(pod, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pod %s: %w", pod.ID, err)
	}
	path := filepath.Join(pm.store, pod.ID+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write pod %s: %w", pod.ID, err)
	}
	return nil
}

func (pm *PodManager) removePodFile(id string) error {
	path := filepath.Join(pm.store, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (pm *PodManager) loadPods() {
	entries, err := os.ReadDir(pm.store)
	if err != nil {
		log.Printf("podman: read pod store %s: %v", pm.store, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(pm.store, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("podman: read pod file %s: %v", path, err)
			continue
		}
		var pod Pod
		if err := json.Unmarshal(data, &pod); err != nil {
			log.Printf("podman: parse pod file %s: %v", path, err)
			continue
		}
		pm.pods[pod.ID] = &pod
	}
}

func generateID() string {
	return common.GenerateID(12)
}
