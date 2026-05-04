package image

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/registry"
)

// Store manages OCI images on disk.
type Store struct {
	mu       sync.RWMutex
	root     string
	registry *registry.Client
}

// Config represents an OCI image configuration.
type Config struct {
	Created      string            `json:"created,omitempty"`
	Author       string            `json:"author,omitempty"`
	Architecture string            `json:"architecture"`
	OS           string            `json:"os"`
	Config       ImageConfig       `json:"config"`
	RootFS       RootFS            `json:"rootfs"`
	History      []History         `json:"history,omitempty"`
}

// ImageConfig is the runtime configuration for a container.
type ImageConfig struct {
	User         string              `json:"User,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	Env          []string            `json:"Env,omitempty"`
	Entrypoint   []string            `json:"Entrypoint,omitempty"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Volumes      map[string]struct{} `json:"Volumes,omitempty"`
	WorkingDir   string              `json:"WorkingDir,omitempty"`
	Labels       map[string]string   `json:"Labels,omitempty"`
	StopSignal   string              `json:"StopSignal,omitempty"`
	Shell        []string            `json:"Shell,omitempty"`
}

// RootFS describes the image's root filesystem.
type RootFS struct {
	Type    string   `json:"type"`
	DiffIDs []string `json:"diff_ids"`
}

// History describes the history of an image layer.
type History struct {
	Created    string `json:"created,omitempty"`
	CreatedBy  string `json:"created_by,omitempty"`
	Comment    string `json:"comment,omitempty"`
	EmptyLayer bool   `json:"empty_layer,omitempty"`
}

// ImageRecord stores image metadata on disk.
type ImageRecord struct {
	ID          string            `json:"id"`
	RepoTags    []string          `json:"repo_tags"`
	RepoDigests []string          `json:"repo_digests"`
	Parent      string            `json:"parent,omitempty"`
	Config      *Config           `json:"config"`
	Manifest    *registry.ManifestV2 `json:"manifest,omitempty"`
	Size        int64             `json:"size"`
	Created     int64             `json:"created"`
	Architecture string           `json:"architecture"`
	OS          string            `json:"os"`
	Layers      []string          `json:"layers"`
}

// NewStore creates a new image store.
func NewStore(root string) (*Store, error) {
	common.EnsureDir(root)
	common.EnsureDir(filepath.Join(root, "blobs"))
	common.EnsureDir(filepath.Join(root, "manifests"))
	common.EnsureDir(filepath.Join(root, "layers"))

	return &Store{
		root:     root,
		registry: registry.NewClient(false),
	}, nil
}

// Pull downloads an image from a registry.
func (s *Store) Pull(imageRef string) (*ImageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ref, err := registry.ParseImageRef(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse image ref: %w", err)
	}

	// Get manifest (with multi-arch resolution).
	manifest, _, err := s.registry.ResolveManifest(ref.Registry, ref.Name, ref.Tag)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	// Get config.
	configData, err := s.registry.GetConfig(ref.Registry, ref.Name, manifest)
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Download layers.
	var layers []string
	for i, layer := range manifest.Layers {
		layerPath := s.layerPath(layer.Digest)
		if !common.PathExists(layerPath) {
			if err := s.downloadLayer(ref.Registry, ref.Name, layer, layerPath); err != nil {
				return nil, fmt.Errorf("download layer %d: %w", i, err)
			}
		}
		layers = append(layers, layer.Digest)
	}

	// Create image ID from config digest.
	imageID := manifest.Config.Digest

	// Save image record (initial size = 0, will be fixed below).
	record := &ImageRecord{
		ID:           imageID,
		RepoTags:     []string{imageRef},
		RepoDigests:  []string{},
		Config:       &config,
		Manifest:     manifest,
		Size:         0,
		Created:      common.NowTimestamp(),
		Architecture: config.Architecture,
		OS:           config.OS,
		Layers:       layers,
	}

	// Compute actual size from downloaded files.
	record.Size = realSize(s, record)

	if err := s.SaveRecord(record); err != nil {
		return nil, err
	}

	return record, nil
}

func (s *Store) downloadLayer(registryHost, name string, layer registry.ManifestBlob, targetPath string) error {
	common.EnsureDir(filepath.Dir(targetPath))

	f, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return s.registry.DownloadBlob(registryHost, name, layer.Digest, f)
}

func (s *Store) layerPath(digest string) string {
	return filepath.Join(s.root, "layers", digest)
}

func (s *Store) manifestPath(id string) string {
	return filepath.Join(s.root, "manifests", id)
}

func (s *Store) recordPath(id string) string {
	return filepath.Join(s.root, "manifests", id+".json")
}

func realSize(store *Store, record *ImageRecord) int64 {
	var size int64
	for _, digest := range record.Layers {
		path := store.layerPath(digest)
		if info, err := os.Stat(path); err == nil {
			size += info.Size()
		}
	}
	// Also count manifest size from disk.
	if len(record.Layers) == 0 && record.Manifest != nil {
		for _, layer := range record.Manifest.Layers {
			path := store.layerPath(layer.Digest)
			if info, err := os.Stat(path); err == nil {
				size += info.Size()
			}
		}
	}
	return size
}

func (s *Store) SaveRecord(record *ImageRecord) error {
	common.EnsureDir(s.manifestPath(record.ID))

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.recordPath(record.ID), data, 0644)
}

// Get returns an image record by ID or tag.
func (s *Store) Get(idOrTag string) (*ImageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try by ID first.
	if common.PathExists(s.recordPath(idOrTag)) {
		return s.loadRecord(idOrTag)
	}

	// Search by tag.
	records, err := s.listRecords()
	if err != nil {
		return nil, err
	}

	for _, record := range records {
		for _, tag := range record.RepoTags {
			if tag == idOrTag {
				rec := record // copy the loop variable
				return &rec, nil
			}
		}
	}

	return nil, common.NewErrNotFound("image", idOrTag)
}

func (s *Store) loadRecord(id string) (*ImageRecord, error) {
	data, err := os.ReadFile(s.recordPath(id))
	if err != nil {
		return nil, err
	}

	var record ImageRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}

	// Fix size if it was stored as 0.
	if record.Size == 0 {
		record.Size = realSize(s, &record)
	}

	return &record, nil
}

// List returns all locally stored images.
func (s *Store) List() ([]common.ImageInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records, err := s.listRecords()
	if err != nil {
		return nil, err
	}

	images := make([]common.ImageInfo, 0, len(records))
	for _, record := range records {
		images = append(images, common.ImageInfo{
			ID:           record.ID[:12],
			RepoTags:     record.RepoTags,
			RepoDigests:  record.RepoDigests,
			Created:      record.Created,
			Size:         record.Size,
			VirtualSize:  record.Size,
			Architecture: record.Architecture,
			Os:           record.OS,
			Labels:       recordLabels(record.Config),
		})
	}

	return images, nil
}

func recordLabels(cfg *Config) map[string]string {
	if cfg == nil {
		return nil
	}
	return cfg.Config.Labels
}

func (s *Store) listRecords() ([]ImageRecord, error) {
	var records []ImageRecord

	entries, err := os.ReadDir(filepath.Join(s.root, "manifests"))
	if err != nil {
		return records, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		record, err := s.loadRecord(entry.Name())
		if err != nil {
			continue
		}
		records = append(records, *record)
	}

	return records, nil
}

// Tag adds a tag to an existing image.
func (s *Store) Tag(source, target string) error {
	s.mu.Lock()

	// Use listRecords directly to avoid nested locking.
	records, err := s.listRecords()
	if err != nil {
		s.mu.Unlock()
		return err
	}

	var record *ImageRecord
	for _, r := range records {
		for _, tag := range r.RepoTags {
			if tag == source {
				rec := r
				record = &rec
				break
			}
		}
		if record != nil {
			break
		}
	}
	if record == nil {
		// Try by ID.
		r, err := s.loadRecord(source)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		record = r
	}

	for _, tag := range record.RepoTags {
		if tag == target {
			s.mu.Unlock()
			return nil
		}
	}

	record.RepoTags = append(record.RepoTags, target)
	err = s.SaveRecord(record)
	s.mu.Unlock()
	return err
}

// Remove removes an image.
func (s *Store) Remove(idOrTag string) error {
	s.mu.Lock()

	// Use listRecords directly to avoid deadlock with Get.
	records, err := s.listRecords()
	if err != nil {
		s.mu.Unlock()
		return err
	}

	var record *ImageRecord
	for _, r := range records {
		for _, tag := range r.RepoTags {
			if tag == idOrTag {
				rec := r
				record = &rec
				break
			}
		}
		if record != nil {
			break
		}
	}
	if record == nil {
		r, err := s.loadRecord(idOrTag)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		record = r
	}

	os.RemoveAll(s.manifestPath(record.ID))
	s.mu.Unlock()
	return nil
}

// Prune removes unused images.
func (s *Store) Prune() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.listRecords()
	if err != nil {
		return nil, err
	}

	var removed []string
	for _, record := range records {
		os.RemoveAll(s.manifestPath(record.ID))
		removed = append(removed, record.ID[:12])
	}

	return removed, nil
}

// Exists checks if an image exists locally.
func (s *Store) Exists(idOrTag string) bool {
	_, err := s.Get(idOrTag)
	return err == nil
}

// Inspect returns the full image config.
func (s *Store) Inspect(idOrTag string) (*Config, error) {
	record, err := s.Get(idOrTag)
	if err != nil {
		return nil, err
	}
	return record.Config, nil
}

// Search searches Docker Hub for images.
func (s *Store) Search(term string, limit int) ([]SearchResult, error) {
	client := s.registry

	url := fmt.Sprintf("https://hub.docker.com/v2/search/repositories/?query=%s&page_size=%d", term, limit)
	resp, err := client.DoRequest(nil, "GET", url, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []SearchResult `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// SearchResult represents a Docker Hub search result.
type SearchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
	IsOfficial  bool   `json:"is_official"`
	IsAutomated bool   `json:"is_automated"`
}

// Export exports an image to a tarball.
func (s *Store) Export(idOrTag string, writer io.Writer) error {
	record, err := s.Get(idOrTag)
	if err != nil {
		return err
	}

	// Write manifest.
	manifestData, _ := json.Marshal(record.Manifest)
	writer.Write(manifestData)

	// Write config.
	configData, _ := json.Marshal(record.Config)
	writer.Write(configData)

	// Write layers.
	for _, digest := range record.Layers {
		layerPath := s.layerPath(digest)
		f, err := os.Open(layerPath)
		if err != nil {
			return err
		}
		io.Copy(writer, f)
		f.Close()
	}

	return nil
}

// Import imports an image from a tarball.
func (s *Store) Import(reader io.Reader) (*ImageRecord, error) {
	// Stream layout: manifest JSON + config JSON + layer blobs.
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var manifest registry.ManifestV2
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	imageID := manifest.Config.Digest

	record := &ImageRecord{
		ID:       imageID,
		RepoTags: []string{"imported:latest"},
		Manifest: &manifest,
		Created:  common.NowTimestamp(),
	}

	if err := s.SaveRecord(record); err != nil {
		return nil, err
	}

	return record, nil
}

// GetLayerPath returns the path to a layer blob on disk.
func (s *Store) GetLayerPath(digest string) string {
	return s.layerPath(digest)
}

// History returns the image build history.
func (s *Store) History(idOrTag string) ([]History, error) {
	record, err := s.Get(idOrTag)
	if err != nil {
		return nil, err
	}

	if record.Config == nil {
		return nil, nil
	}

	return record.Config.History, nil
}

// GetLayerPaths returns paths to all layer tarballs for an image.
func (s *Store) GetLayerPaths(idOrTag string) ([]string, error) {
	record, err := s.Get(idOrTag)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, digest := range record.Layers {
		paths = append(paths, s.layerPath(digest))
	}
	return paths, nil
}

// Config returns the OCI image configuration.
func (s *Store) Config(idOrTag string) (*Config, error) {
	record, err := s.Get(idOrTag)
	if err != nil {
		return nil, err
	}
	return record.Config, nil
}
