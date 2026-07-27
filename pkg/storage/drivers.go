// Package storage provides storage driver management.
package storage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

var validLayerID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func validateLayerID(id string) error {
	if id == "" || !validLayerID.MatchString(id) || len(id) > 128 {
		return fmt.Errorf("invalid layer id: %q", id)
	}
	return nil
}

// ─── Btrfs Driver ──────────────────────────────────────────────────

// BtrfsDriver implements the storage driver using btrfs subvolumes.
type BtrfsDriver struct {
	root    string
	subvols map[string]string
}

// NewBtrfsDriver creates a new btrfs storage driver rooted at root.
func NewBtrfsDriver(root string) (*BtrfsDriver, error) {
	if !isBtrfs(root) {
		return nil, fmt.Errorf("btrfs: %s is not a btrfs filesystem", root)
	}
	_ = common.EnsureDir(root)
	return &BtrfsDriver{root: root, subvols: make(map[string]string)}, nil
}

// Name returns the name of the btrfs driver.
func (d *BtrfsDriver) Name() string { return "btrfs" }

func isBtrfs(path string) bool {
	_ = path
	return false
}

// Get mounts the btrfs subvolume for the given layer id and returns its mount path.
func (d *BtrfsDriver) Get(id, _ string) (string, error) {
	if err := validateLayerID(id); err != nil {
		return "", err
	}
	subvolPath := filepath.Join(d.root, id)
	if !common.PathExists(subvolPath) {
		return "", common.NewErrNotFound("layer", id)
	}
	mountPath := filepath.Join(d.root, "mnt", id)
	_ = common.EnsureDir(mountPath)
	if err := mountSubvol(subvolPath, mountPath); err != nil {
		return "", err
	}
	return mountPath, nil
}

// Put unmounts the btrfs subvolume for the given layer id and returns its mount path.
func (d *BtrfsDriver) Put(id, _ string) (string, error) {
	if err := validateLayerID(id); err != nil {
		return "", err
	}
	mountPath := filepath.Join(d.root, "mnt", id)
	_ = unmount(mountPath)
	return mountPath, nil
}

// Exists checks whether a btrfs subvolume for the given layer id exists.
func (d *BtrfsDriver) Exists(id string) bool {
	if err := validateLayerID(id); err != nil {
		return false
	}
	return common.PathExists(filepath.Join(d.root, id))
}

// Remove deletes the btrfs subvolume for the given layer id.
func (d *BtrfsDriver) Remove(id string) error {
	if err := validateLayerID(id); err != nil {
		return err
	}
	subvolPath := filepath.Join(d.root, id)
	return exec.Command("btrfs", "subvolume", "delete", subvolPath).Run()
}

// Cleanup releases resources held by the btrfs driver.
func (d *BtrfsDriver) Cleanup() error { return nil }

// GetMetadata returns metadata for the given layer.
func (d *BtrfsDriver) GetMetadata(_ string) (map[string]string, error) {
	return nil, nil
}

// ─── ZFS Driver ────────────────────────────────────────────────────

// ZFSDriver implements the storage driver using ZFS filesystems.
type ZFSDriver struct {
	root     string
	pool     string
	fsPrefix string
}

// NewZFSDriver creates a new ZFS storage driver.
func NewZFSDriver(root, pool, fsPrefix string) (*ZFSDriver, error) {
	if !isZFS() {
		return nil, fmt.Errorf("zfs: not available")
	}
	return &ZFSDriver{root: root, pool: pool, fsPrefix: fsPrefix}, nil
}

func isZFS() bool {
	_, err := exec.LookPath("zfs")
	return err == nil
}

// Name returns the name of the ZFS driver.
func (d *ZFSDriver) Name() string { return "zfs" }

// Get mounts the ZFS filesystem for the given layer id and returns its mount path.
func (d *ZFSDriver) Get(id, _ string) (string, error) {
	if err := validateLayerID(id); err != nil {
		return "", err
	}
	fsName := d.fsPrefix + "/" + id
	mountPath := filepath.Join(d.root, id)
	_ = common.EnsureDir(mountPath)
	cmd := exec.Command("zfs", "mount", fsName)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return mountPath, nil
}

// Put unmounts the ZFS filesystem for the given layer id and returns its mount path.
func (d *ZFSDriver) Put(id, _ string) (string, error) {
	if err := validateLayerID(id); err != nil {
		return "", err
	}
	fsName := d.fsPrefix + "/" + id
	if err := exec.Command("zfs", "unmount", fsName).Run(); err != nil {
		return filepath.Join(d.root, id), fmt.Errorf("zfs unmount %s: %w", fsName, err)
	}
	return filepath.Join(d.root, id), nil
}

// Exists checks whether a ZFS filesystem for the given layer id exists.
func (d *ZFSDriver) Exists(id string) bool {
	if err := validateLayerID(id); err != nil {
		return false
	}
	fsName := d.fsPrefix + "/" + id
	return exec.Command("zfs", "list", fsName).Run() == nil
}

// Remove destroys the ZFS filesystem for the given layer id.
func (d *ZFSDriver) Remove(id string) error {
	if err := validateLayerID(id); err != nil {
		return err
	}
	fsName := d.fsPrefix + "/" + id
	return exec.Command("zfs", "destroy", "-r", fsName).Run()
}

// Cleanup releases resources held by the ZFS driver.
func (d *ZFSDriver) Cleanup() error { return nil }

// GetMetadata returns metadata for the given layer.
func (d *ZFSDriver) GetMetadata(_ string) (map[string]string, error) {
	return nil, nil
}

// ─── VFS Driver (naive, for testing) ──────────────────────────────

// VFSDriver implements the storage driver using the host filesystem directly.
type VFSDriver struct {
	root string
}

// NewVFSDriver creates a new VFS storage driver.
func NewVFSDriver(root string) (*VFSDriver, error) {
	_ = common.EnsureDir(root)
	return &VFSDriver{root: root}, nil
}

// Name returns the name of the VFS driver.
func (d *VFSDriver) Name() string { return "vfs" }

// Get returns the filesystem path for the given layer id.
func (d *VFSDriver) Get(id, _ string) (string, error) {
	// MED-3: validate the id so "../../.." cannot address paths outside root.
	if err := validateLayerID(id); err != nil {
		return "", err
	}
	path := filepath.Join(d.root, id)
	if !common.PathExists(path) {
		return "", common.NewErrNotFound("layer", id)
	}
	return path, nil
}

// Put returns the filesystem path for the given layer id.
func (d *VFSDriver) Put(id, _ string) (string, error) {
	if err := validateLayerID(id); err != nil {
		return "", err
	}
	return filepath.Join(d.root, id), nil
}

// Exists checks whether the directory for the given layer id exists.
func (d *VFSDriver) Exists(id string) bool {
	if err := validateLayerID(id); err != nil {
		return false
	}
	return common.PathExists(filepath.Join(d.root, id))
}

// Remove deletes the directory for the given layer id.
func (d *VFSDriver) Remove(id string) error {
	// MED-3: without validation, id="../../.." would RemoveAll host dirs.
	if err := validateLayerID(id); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(d.root, id))
}

// Cleanup releases resources held by the VFS driver.
func (d *VFSDriver) Cleanup() error { return nil }

// GetMetadata returns metadata for the given layer.
func (d *VFSDriver) GetMetadata(_ string) (map[string]string, error) {
	return nil, nil
}

// ─── Garbage Collection ────────────────────────────────────────────

// GCConfig holds configuration for garbage collection.
type GCConfig struct {
	Enabled      bool
	Interval     time.Duration
	MaxAge       time.Duration
	MinFreeSpace int64
}

// GarbageCollector runs periodic garbage collection on unused layers.
type GarbageCollector struct {
	store *Manager
	cfg   GCConfig
	stop  chan struct{}
	once  sync.Once
}

// NewGarbageCollector creates a new garbage collector for the given storage manager.
func NewGarbageCollector(store *Manager, cfg GCConfig) *GarbageCollector {
	return &GarbageCollector{store: store, cfg: cfg, stop: make(chan struct{})}
}

// Start begins periodic garbage collection.
func (g *GarbageCollector) Start() {
	if !g.cfg.Enabled {
		return
	}
	go func() {
		ticker := time.NewTicker(g.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				g.collect()
			case <-g.stop:
				return
			}
		}
	}()
}

// Stop halts the garbage collector.
func (g *GarbageCollector) Stop() {
	g.once.Do(func() { close(g.stop) })
}

func (g *GarbageCollector) collect() {
	unused, err := g.findUnusedLayers()
	if err != nil {
		slog.Warn("GC: findUnusedLayers failed", "err", err)
		return
	}
	for _, id := range unused {
		_ = g.store.Remove(id)
	}
}

func (g *GarbageCollector) findUnusedLayers() ([]string, error) {
	root := g.store.Root()
	layersDir := filepath.Join(root, "layers")

	entries, err := os.ReadDir(layersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read layers dir: %w", err)
	}

	allLayers := make(map[string]struct{})
	for _, e := range entries {
		if e.IsDir() {
			allLayers[e.Name()] = struct{}{}
		}
	}
	if len(allLayers) == 0 {
		return nil, nil
	}

	containersDir := filepath.Join(root, "containers")
	ce, err := os.ReadDir(containersDir)
	if err == nil {
		for _, entry := range ce {
			if !entry.IsDir() {
				continue
			}
			statePath := filepath.Join(containersDir, entry.Name(), "state.json")
			data, err := os.ReadFile(statePath)
			if err != nil {
				continue
			}
			var s struct {
				Config *struct {
					ImageLayers []string `json:"ImageLayers"`
				} `json:"config"`
			}
			if err := json.Unmarshal(data, &s); err != nil {
				continue
			}
			if s.Config != nil {
				for _, layer := range s.Config.ImageLayers {
					delete(allLayers, filepath.Base(layer))
					delete(allLayers, filepath.Base(filepath.Dir(layer)))
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read containers dir: %w", err)
	}

	now := time.Now()
	var unused []string
	for id := range allLayers {
		layerPath := filepath.Join(layersDir, id)
		fi, err := os.Stat(layerPath)
		if err != nil {
			continue
		}
		if g.cfg.MaxAge > 0 && now.Sub(fi.ModTime()) < g.cfg.MaxAge {
			continue
		}
		unused = append(unused, id)
	}

	return unused, nil
}

// ─── Helpers ───────────────────────────────────────────────────────

func mountSubvol(src, dst string) error {
	_ = common.EnsureDir(dst)
	cmd := exec.Command("mount", "-o", "subvol="+filepath.Base(src), src, dst)
	return cmd.Run()
}

func unmount(target string) error {
	cmd := exec.Command("umount", target)
	if err := cmd.Run(); err != nil {
		slog.Warn("unmount failed", "target", target, "err", err)
		return err
	}
	return nil
}
