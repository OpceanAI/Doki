package image

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/registry"
)

// Store manages OCI images on disk.
type Store struct {
	mu             sync.RWMutex
	root           string
	registry       *registry.Client
	manifestCache  map[string]*manifestCacheEntry
	cacheMu        sync.RWMutex
}

type manifestCacheEntry struct {
	manifest  *registry.ManifestV2
	mediaType string
	expiresAt time.Time
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
	HealthCheck  *HealthCheckConfig  `json:"Healthcheck,omitempty"`
}

// HealthCheckConfig describes a container's health check.
type HealthCheckConfig struct {
	Test        []string `json:"Test,omitempty"`
	Interval    int64    `json:"Interval,omitempty"`
	Timeout     int64    `json:"Timeout,omitempty"`
	Retries     int      `json:"Retries,omitempty"`
	StartPeriod int64    `json:"StartPeriod,omitempty"`
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
		root:          root,
		registry:      registry.NewClient(false),
		manifestCache: make(map[string]*manifestCacheEntry),
	}, nil
}

// Pull downloads an image from a registry.
func (s *Store) Pull(imageRef string) (*ImageRecord, error) {
	ref, err := registry.ParseImageRef(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse image ref: %w", err)
	}

	// AG4: Check manifest cache (5-minute TTL).
	cacheKey := ref.Registry + "/" + ref.Name + ":" + ref.Tag
	s.cacheMu.RLock()
	if entry, ok := s.manifestCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		manifest, mediaType := entry.manifest, entry.mediaType
		s.cacheMu.RUnlock()
		configData, err := s.registry.GetConfig(ref.Registry, ref.Name, manifest)
		if err != nil {
			return nil, fmt.Errorf("get config: %w", err)
		}
		var config Config
		if err := json.Unmarshal(configData, &config); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
		// AG2: Parallel layer downloads.
		layers, err := s.downloadLayersParallel(ref.Registry, ref.Name, manifest.Layers)
		if err != nil {
			return nil, err
		}
		return s.saveImageRecord(imageRef, ref, manifest, mediaType, &config, layers)
	}
	s.cacheMu.RUnlock()

	// Download manifest and config (no lock needed - network I/O).
	manifest, mediaType, err := s.registry.ResolveManifest(ref.Registry, ref.Name, ref.Tag)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	// AG4: Cache the manifest for 5 minutes.
	s.cacheMu.Lock()
	s.manifestCache[cacheKey] = &manifestCacheEntry{
		manifest:  manifest,
		mediaType: mediaType,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	s.cacheMu.Unlock()

	configData, err := s.registry.GetConfig(ref.Registry, ref.Name, manifest)
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// AG2: Download layers concurrently with 3 goroutines limit.
	layers, err := s.downloadLayersParallel(ref.Registry, ref.Name, manifest.Layers)
	if err != nil {
		return nil, err
	}

	return s.saveImageRecord(imageRef, ref, manifest, mediaType, &config, layers)
}

// downloadLayersParallel downloads layers concurrently with a limit of 3 concurrent downloads.
func (s *Store) downloadLayersParallel(registryHost, name string, layers []registry.ManifestBlob) ([]string, error) {
	if len(layers) == 0 {
		return nil, nil
	}

	type result struct {
		index  int
		digest string
		err    error
	}

	maxConcurrent := 3
	if len(layers) < maxConcurrent {
		maxConcurrent = len(layers)
	}

	sem := make(chan struct{}, maxConcurrent)
	results := make(chan result, len(layers))
	var wg sync.WaitGroup

	for i, layer := range layers {
		layerPath := s.layerPath(layer.Digest)
		if common.PathExists(layerPath) {
			results <- result{index: i, digest: layer.Digest, err: nil}
			continue
		}
		wg.Add(1)
		go func(idx int, l registry.ManifestBlob, lp string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err := s.downloadLayer(registryHost, name, l, lp)
			results <- result{index: idx, digest: l.Digest, err: err}
		}(i, layer, layerPath)
	}

	wg.Wait()
	close(results)

	digests := make([]string, len(layers))
	for r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("download layer %d: %w", r.index, r.err)
		}
		digests[r.index] = r.digest
	}

	return digests, nil
}

func (s *Store) saveImageRecord(imageRef string, ref *registry.ImageRef, manifest *registry.ManifestV2, mediaType string, config *Config, layers []string) (*ImageRecord, error) {
	// Create and save record (lock for state modification).
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = mediaType

	imageID := manifest.Config.Digest
	record := &ImageRecord{
		ID:           imageID,
		RepoTags:     []string{imageRef},
		RepoDigests:  []string{},
		Config:       config,
		Manifest:     manifest,
		Size:         0,
		Created:      common.NowTimestamp(),
		Architecture: config.Architecture,
		OS:           config.OS,
		Layers:       layers,
	}
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
	defer s.mu.Unlock()

	records, err := s.listRecords()
	if err != nil {
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
		r, err := s.loadRecord(source)
		if err != nil {
			return err
		}
		record = r
	}

	for _, tag := range record.RepoTags {
		if tag == target {
			return nil
		}
	}

	record.RepoTags = append(record.RepoTags, target)
	return s.SaveRecord(record)
}

// Remove removes an image.
func (s *Store) Remove(idOrTag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.listRecords()
	if err != nil {
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
			return err
		}
		record = r
	}

	os.RemoveAll(s.manifestPath(record.ID))
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

// Export exports an image to a Docker-format save tar.
func (s *Store) Export(idOrTag string, writer io.Writer) error {
	record, err := s.Get(idOrTag)
	if err != nil {
		return err
	}

	tw := tar.NewWriter(writer)
	defer tw.Close()

	digestToHex := func(d string) string {
		return strings.TrimPrefix(d, "sha256:")
	}

	// Build and write manifest.json entry.
	type mfEntry struct {
		Config   string   `json:"Config"`
		RepoTags []string `json:"RepoTags"`
		Layers   []string `json:"Layers"`
	}
	entry := mfEntry{
		Config:   digestToHex(record.Manifest.Config.Digest) + ".json",
		RepoTags: record.RepoTags,
	}
	for _, d := range record.Layers {
		entry.Layers = append(entry.Layers, digestToHex(d)+"/layer.tar")
	}

	mfData, _ := json.Marshal([]mfEntry{entry})
	if err := tw.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(mfData)),
		Mode: 0644,
	}); err != nil {
		return err
	}
	if _, err := tw.Write(mfData); err != nil {
		return err
	}

	// Write config blob.
	configPath := s.manifestPath(record.ID)
	configData, err := os.ReadFile(filepath.Join(configPath, record.ID+".json"))
	if err != nil {
		configData, _ = json.Marshal(record.Config)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name: digestToHex(record.Manifest.Config.Digest) + ".json",
		Size: int64(len(configData)),
		Mode: 0644,
	}); err != nil {
		return err
	}
	if _, err := tw.Write(configData); err != nil {
		return err
	}

	// Write each layer.
	for _, d := range record.Layers {
		hex := digestToHex(d)
		layerPath := s.layerPath(d)
		fi, err := os.Stat(layerPath)
		if err != nil {
			return fmt.Errorf("layer %s: %w", d, err)
		}
		f, err := os.Open(layerPath)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: hex + "/layer.tar",
			Size: fi.Size(),
			Mode: 0644,
		}); err != nil {
			f.Close()
			return err
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}

	return nil
}

// Import imports an image from a Docker-format save tar.
func (s *Store) Import(reader io.Reader) (*ImageRecord, error) {
	tr := tar.NewReader(reader)

	var mfData []byte
	var configData []byte
	layers := make(map[string][]byte) // hex -> blob data

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read tar entry %s: %w", hdr.Name, err)
		}

		switch {
		case hdr.Name == "manifest.json":
			mfData = data
		case strings.HasSuffix(hdr.Name, ".json") && !strings.Contains(hdr.Name[0:len(hdr.Name)-5], "/"):
			if configData == nil {
				configData = data
			}
		case strings.HasSuffix(hdr.Name, "/layer.tar"):
			hex := strings.TrimSuffix(hdr.Name, "/layer.tar")
			layers[hex] = data
		}
	}

	if mfData == nil {
		return nil, fmt.Errorf("no manifest.json in tar")
	}

	var mfEntries []struct {
		Config   string   `json:"Config"`
		RepoTags []string `json:"RepoTags"`
		Layers   []string `json:"Layers"`
	}
	if err := json.Unmarshal(mfData, &mfEntries); err != nil {
		return nil, fmt.Errorf("unmarshal manifest.json: %w", err)
	}
	if len(mfEntries) == 0 {
		return nil, fmt.Errorf("empty manifest.json")
	}

	mf := mfEntries[0]
	cfgHex := strings.TrimSuffix(mf.Config, ".json")
	configDigest := "sha256:" + cfgHex

	var layerDigests []string
	for _, l := range mf.Layers {
		hex := strings.TrimSuffix(l, "/layer.tar")
		layerDigests = append(layerDigests, "sha256:"+hex)
	}

	// Write config blob.
	configPath := filepath.Join(s.root, "blobs", configDigest)
	common.EnsureDir(filepath.Dir(configPath))
	if configData == nil {
		configData = []byte("{}")
	}
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	var cfg Config
	json.Unmarshal(configData, &cfg)

	var manifest registry.ManifestV2
	manifest.SchemaVersion = 2
	manifest.Config = registry.ManifestBlob{
		MediaType: "application/vnd.oci.image.config.v1+json",
		Digest:    configDigest,
		Size:      int64(len(configData)),
	}

	// Write layer blobs and build manifest layers.
	for _, hex := range mfEntries[0].Layers {
		hexNoSuffix := strings.TrimSuffix(hex, "/layer.tar")
		digest := "sha256:" + hexNoSuffix
		blobData, ok := layers[hexNoSuffix]
		if !ok {
			return nil, fmt.Errorf("layer %s not found in tar", hex)
		}

		blobPath := filepath.Join(s.root, "blobs", digest)
		common.EnsureDir(filepath.Dir(blobPath))
		if err := os.WriteFile(blobPath, blobData, 0644); err != nil {
			return nil, fmt.Errorf("write blob %s: %w", digest, err)
		}

		// Also save as layer.
		layerPath := s.layerPath(digest)
		common.EnsureDir(filepath.Dir(layerPath))
		if err := os.WriteFile(layerPath, blobData, 0644); err != nil {
			return nil, fmt.Errorf("write layer %s: %w", digest, err)
		}

		manifest.Layers = append(manifest.Layers, registry.ManifestBlob{
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
			Digest:    digest,
			Size:      int64(len(blobData)),
		})
	}

	tags := mf.RepoTags
	if len(tags) == 0 {
		tags = []string{"imported:latest"}
	}

	imageID := configDigest
	record := &ImageRecord{
		ID:           imageID,
		RepoTags:     tags,
		RepoDigests:  []string{},
		Config:       &cfg,
		Manifest:     &manifest,
		Size:         int64(len(configData)),
		Created:      common.NowTimestamp(),
		Architecture: cfg.Architecture,
		OS:           cfg.OS,
		Layers:       layerDigests,
	}

	for _, l := range record.Layers {
		if fi, err := os.Stat(s.layerPath(l)); err == nil {
			record.Size += fi.Size()
		}
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
