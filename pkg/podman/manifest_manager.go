package podman

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ManifestManager struct {
	mu        sync.RWMutex
	manifests map[string]*ManifestList
	store     string
}

func NewManifestManager(root string) *ManifestManager {
	mm := &ManifestManager{
		manifests: make(map[string]*ManifestList),
		store:     filepath.Join(root, "manifests"),
	}
	_ = os.MkdirAll(mm.store, 0755)
	mm.loadManifests()
	return mm
}

func (mm *ManifestManager) Create(name string, images []string) (*ManifestList, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.manifests[name]; exists {
		return nil, fmt.Errorf("manifest list %q already exists", name)
	}

	ml := &ManifestList{
		Name:      name,
		Images:    make([]ManifestEntry, 0),
		MediaType: "application/vnd.docker.distribution.manifest.list.v2+json",
		Created:   time.Now(),
		Modified:  time.Now(),
	}

	for _, img := range images {
		ml.Images = append(ml.Images, ManifestEntry{
			Image:  img,
			Digest: "sha256:pending",
		})
	}

	mm.manifests[name] = ml
	mm.saveManifest(ml)
	return ml, nil
}

func (mm *ManifestManager) Inspect(name string) (*ManifestList, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	ml, ok := mm.manifests[name]
	if !ok {
		return nil, fmt.Errorf("manifest list %s not found", name)
	}
	return ml, nil
}

func (mm *ManifestManager) Add(name, image string, platform Platform) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	ml, ok := mm.manifests[name]
	if !ok {
		return fmt.Errorf("manifest list %s not found", name)
	}

	ml.Images = append(ml.Images, ManifestEntry{
		Image:    image,
		Digest:   "sha256:pending",
		Platform: platform,
	})
	ml.Modified = time.Now()
	mm.saveManifest(ml)
	return nil
}

func (mm *ManifestManager) Remove(name, image string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	ml, ok := mm.manifests[name]
	if !ok {
		return fmt.Errorf("manifest list %s not found", name)
	}

	filtered := make([]ManifestEntry, 0)
	for _, entry := range ml.Images {
		if entry.Image != image {
			filtered = append(filtered, entry)
		}
	}
	ml.Images = filtered
	ml.Modified = time.Now()
	mm.saveManifest(ml)
	return nil
}

func (mm *ManifestManager) Delete(name string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, ok := mm.manifests[name]; !ok {
		return fmt.Errorf("manifest list %s not found", name)
	}

	delete(mm.manifests, name)
	_ = os.Remove(filepath.Join(mm.store, name+".json"))
	return nil
}

func (mm *ManifestManager) List() []*ManifestList {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	result := make([]*ManifestList, 0, len(mm.manifests))
	for _, ml := range mm.manifests {
		result = append(result, ml)
	}
	return result
}

func (mm *ManifestManager) Exists(name string) bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	_, ok := mm.manifests[name]
	return ok
}

func (mm *ManifestManager) Annotate(name, image string, platform Platform) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	ml, ok := mm.manifests[name]
	if !ok {
		return fmt.Errorf("manifest list %s not found", name)
	}

	for i, entry := range ml.Images {
		if entry.Image == image {
			ml.Images[i].Platform = platform
			ml.Modified = time.Now()
			mm.saveManifest(ml)
			return nil
		}
	}
	return fmt.Errorf("image %s not found in manifest list %s", image, name)
}

func (mm *ManifestManager) saveManifest(ml *ManifestList) {
	data, _ := json.MarshalIndent(ml, "", "  ")
	_ = os.WriteFile(filepath.Join(mm.store, ml.Name+".json"), data, 0644)
}

func (mm *ManifestManager) loadManifests() {
	entries, err := os.ReadDir(mm.store)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(mm.store, entry.Name()))
		if err != nil {
			continue
		}
		var ml ManifestList
		if err := json.Unmarshal(data, &ml); err != nil {
			continue
		}
		mm.manifests[ml.Name] = &ml
	}
}
