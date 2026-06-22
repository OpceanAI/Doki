package podman

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// validateManifestName rejects names that could escape the manifest
// store directory via path traversal.
func validateManifestName(name string) error {
	if name == "" {
		return fmt.Errorf("manifest name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("manifest name %q contains path separator", name)
	}
	if name == "." || name == ".." || strings.Contains(name, "..") {
		return fmt.Errorf("manifest name %q contains path traversal sequence", name)
	}
	if !common.ValidContainerName(name) {
		return fmt.Errorf("manifest name %q contains invalid characters", name)
	}
	return nil
}

type ManifestManager struct {
	mu        sync.RWMutex
	manifests map[string]*ManifestList
	store     string
}

func NewManifestManager(root string) (*ManifestManager, error) {
	mm := &ManifestManager{
		manifests: make(map[string]*ManifestList),
		store:     filepath.Join(root, "manifests"),
	}
	if err := os.MkdirAll(mm.store, 0750); err != nil {
		return nil, fmt.Errorf("podman: create manifest store %s: %w", mm.store, err)
	}
	mm.loadManifests()
	return mm, nil
}

func (mm *ManifestManager) Create(name string, images []string) (*ManifestList, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if err := validateManifestName(name); err != nil {
		return nil, err
	}
	if _, exists := mm.manifests[name]; exists {
		return nil, fmt.Errorf("manifest list %q already exists", name)
	}

	ml := &ManifestList{
		Name:      name,
		Images:    make([]ManifestEntry, 0),
		MediaType: "application/vnd.oci.image.index.v1+json",
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
	if err := mm.saveManifest(ml); err != nil {
		delete(mm.manifests, name)
		return nil, err
	}
	return ml, nil
}

func (mm *ManifestManager) Inspect(name string) (*ManifestList, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	ml, ok := mm.manifests[name]
	if !ok {
		return nil, fmt.Errorf("manifest list %s not found", name)
	}
	// Return a deep copy to avoid data race.
	cp := *ml
	return &cp, nil
}

func (mm *ManifestManager) Add(name, image string, platform Platform) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	ml, ok := mm.manifests[name]
	if !ok {
		return fmt.Errorf("manifest list %s not found", name)
	}

	// Check for duplicate image reference before adding.
	for _, entry := range ml.Images {
		if entry.Image == image {
			return fmt.Errorf("image %s already in manifest list %s", image, name)
		}
	}

	ml.Images = append(ml.Images, ManifestEntry{
		Image:    image,
		Digest:   "sha256:pending",
		Platform: platform,
	})
	ml.Modified = time.Now()
	return mm.saveManifest(ml)
}

func (mm *ManifestManager) Remove(name, image string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	ml, ok := mm.manifests[name]
	if !ok {
		return fmt.Errorf("manifest list %s not found", name)
	}

	filtered := make([]ManifestEntry, 0, len(ml.Images))
	for _, entry := range ml.Images {
		if entry.Image != image {
			filtered = append(filtered, entry)
		}
	}
	ml.Images = filtered
	ml.Modified = time.Now()
	return mm.saveManifest(ml)
}

func (mm *ManifestManager) Delete(name string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if err := validateManifestName(name); err != nil {
		return fmt.Errorf("delete manifest: %w", err)
	}
	if _, ok := mm.manifests[name]; !ok {
		return fmt.Errorf("manifest list %s not found", name)
	}

	delete(mm.manifests, name)
	path := filepath.Join(mm.store, name+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove manifest file: %w", err)
	}
	return nil
}

func (mm *ManifestManager) List() []*ManifestList {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	result := make([]*ManifestList, 0, len(mm.manifests))
	for _, ml := range mm.manifests {
		cp := *ml
		result = append(result, &cp)
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
			return mm.saveManifest(ml)
		}
	}
	return fmt.Errorf("image %s not found in manifest list %s", image, name)
}

func (mm *ManifestManager) saveManifest(ml *ManifestList) error {
	if err := validateManifestName(ml.Name); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	data, err := json.MarshalIndent(ml, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest %s: %w", ml.Name, err)
	}
	path := filepath.Join(mm.store, ml.Name+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write manifest %s: %w", ml.Name, err)
	}
	return nil
}

func (mm *ManifestManager) loadManifests() {
	entries, err := os.ReadDir(mm.store)
	if err != nil {
		log.Printf("podman: read manifest store %s: %v", mm.store, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(mm.store, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("podman: read manifest file %s: %v", path, err)
			continue
		}
		var ml ManifestList
		if err := json.Unmarshal(data, &ml); err != nil {
			log.Printf("podman: parse manifest file %s: %v", path, err)
			continue
		}
		mm.manifests[ml.Name] = &ml
	}
}
