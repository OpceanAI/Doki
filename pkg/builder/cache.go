package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// BuildCache provides content-addressable layer caching for builds.
// Each layer is identified by its content hash, allowing reuse across builds.
type BuildCache struct {
	root    string
	log     *slog.Logger
	mu      sync.RWMutex
	index   map[string]*CacheEntry
	maxSize int64 // max cache size in bytes
	curSize int64 // current cache size in bytes
}

// CacheEntry represents a cached build layer.
type CacheEntry struct {
	Key       string    `json:"key"`
	Digest    string    `json:"digest"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
	HitCount  int64     `json:"hit_count"`
	LayerPath string    `json:"layer_path"`
}

// CacheStats returns cache statistics.
type CacheStats struct {
	Entries   int   `json:"entries"`
	TotalSize int64 `json:"total_size"`
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	HitRate   float64 `json:"hit_rate"`
}

// NewBuildCache creates a new build cache.
func NewBuildCache(root string, maxSize int64) *BuildCache {
	if maxSize == 0 {
		maxSize = 500 * 1024 * 1024 // 500MB default
	}
	c := &BuildCache{
		root:    root,
		log:     slog.Default().With("component", "builder.cache"),
		index:   make(map[string]*CacheEntry),
		maxSize: maxSize,
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		c.log.Warn("create cache dir failed", "path", root, "error", err)
	}
	c.loadIndex()
	return c
}

// Get retrieves a cached layer by its key.
// Returns the layer path and true if found, or "" and false if not.
func (c *BuildCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.index[key]
	if !ok {
		return "", false
	}

	// Check if the layer file still exists.
	if _, err := os.Stat(entry.LayerPath); err != nil {
		delete(c.index, key)
		return "", false
	}

	entry.LastUsed = time.Now()
	entry.HitCount++
	c.saveIndex()
	return entry.LayerPath, true
}

// Put stores a layer in the cache.
func (c *BuildCache) Put(key string, layerPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate content digest.
	digest, size, err := c.hashFile(layerPath)
	if err != nil {
		return fmt.Errorf("hash layer: %w", err)
	}

	// Copy to cache directory.
	cachePath := filepath.Join(c.root, digest+".tar")
	if err := c.copyFile(layerPath, cachePath); err != nil {
		return fmt.Errorf("copy to cache: %w", err)
	}

	entry := &CacheEntry{
		Key:       key,
		Digest:    digest,
		Size:      size,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		HitCount:  0,
		LayerPath: cachePath,
	}

	c.index[key] = entry
	c.curSize += size

	// Evict if over capacity.
	c.evict()

	c.saveIndex()
	return nil
}

// PutBytes stores byte content in the cache.
func (c *BuildCache) PutBytes(key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	digest := c.hashBytes(data)
	cachePath := filepath.Join(c.root, digest+".tar")

	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}

	entry := &CacheEntry{
		Key:       key,
		Digest:    digest,
		Size:      int64(len(data)),
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		HitCount:  0,
		LayerPath: cachePath,
	}

	c.index[key] = entry
	c.curSize += int64(len(data))
	c.evict()
	c.saveIndex()
	return nil
}

// GetBytes retrieves cached content as bytes.
func (c *BuildCache) GetBytes(key string) ([]byte, bool) {
	path, ok := c.Get(key)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// Has checks if a key exists in the cache.
func (c *BuildCache) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.index[key]
	if !ok {
		return false
	}
	_, err := os.Stat(entry.LayerPath)
	return err == nil
}

// Delete removes a cached layer.
func (c *BuildCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.index[key]
	if !ok {
		return
	}
	if err := os.Remove(entry.LayerPath); err != nil {
		c.log.Warn("remove cache entry failed", "path", entry.LayerPath, "error", err)
	}
	c.curSize -= entry.Size
	if c.curSize < 0 {
		c.curSize = 0
	}
	delete(c.index, key)
	c.saveIndex()
}

// Purge removes all cached layers.
func (c *BuildCache) Purge() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(c.root, entry.Name())); err != nil {
			c.log.Warn("remove cached file failed", "name", entry.Name(), "error", err)
		}
	}
	c.index = make(map[string]*CacheEntry)
	c.curSize = 0
	c.saveIndex()
	return nil
}

// Stats returns cache statistics.
func (c *BuildCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalHits int64
	for _, entry := range c.index {
		totalHits += entry.HitCount
	}

	hitRate := float64(0)
	if len(c.index) > 0 {
		hitRate = float64(totalHits) / float64(len(c.index))
	}

	return CacheStats{
		Entries:   len(c.index),
		TotalSize: c.curSize,
		Hits:      totalHits,
		HitRate:   hitRate,
	}
}

// GenerateKey generates a cache key from instruction and context.
func GenerateKey(instType string, args []string, env map[string]string) string {
	h := sha256.New()
	h.Write([]byte(instType))
	for _, a := range args {
		h.Write([]byte(a))
		h.Write([]byte{0})
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write([]byte(env[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateKeyFromReader generates a cache key from a reader.
func GenerateKeyFromReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (c *BuildCache) hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func (c *BuildCache) hashBytes(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func (c *BuildCache) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			c.log.Warn("close output file failed", "path", dst, "error", err)
		}
	}()

	_, err = io.Copy(out, in)
	return err
}

func (c *BuildCache) evict() {
	for c.curSize > c.maxSize && len(c.index) > 0 {
		// Find least recently used entry.
		var oldest *CacheEntry
		var oldestKey string
		for key, entry := range c.index {
			if oldest == nil || entry.LastUsed.Before(oldest.LastUsed) {
				oldest = entry
				oldestKey = key
			}
		}
		if oldest != nil {
			if err := os.Remove(oldest.LayerPath); err != nil {
				c.log.Warn("remove evicted cache entry failed", "path", oldest.LayerPath, "error", err)
			}
			c.curSize -= oldest.Size
			delete(c.index, oldestKey)
			c.log.Debug("evicted cache entry", "key", oldestKey, "size", oldest.Size)
		}
	}
}

func (c *BuildCache) loadIndex() {
indexPath := filepath.Join(c.root, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return
	}
	var entries map[string]*CacheEntry
	if json.Unmarshal(data, &entries) == nil {
		c.index = entries
		for _, e := range entries {
			c.curSize += e.Size
		}
	}
}

func (c *BuildCache) saveIndex() {
indexPath := filepath.Join(c.root, "index.json")
	data, err := json.MarshalIndent(c.index, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		c.log.Warn("write cache index failed", "path", indexPath, "error", err)
	}
}
