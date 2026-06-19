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

type BtrfsDriver struct {
	root    string
	subvols map[string]string
}

func NewBtrfsDriver(root string) (*BtrfsDriver, error) {
	if !isBtrfs(root) {
		return nil, fmt.Errorf("btrfs: %s is not a btrfs filesystem", root)
	}
	_ = common.EnsureDir(root)
	return &BtrfsDriver{root: root, subvols: make(map[string]string)}, nil
}

func (d *BtrfsDriver) Name() string { return "btrfs" }

func isBtrfs(path string) bool {
	_ = path
	return false
}

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

func (d *BtrfsDriver) Put(id, _ string) (string, error) {
	if err := validateLayerID(id); err != nil {
		return "", err
	}
	mountPath := filepath.Join(d.root, "mnt", id)
	_ = unmount(mountPath)
	return mountPath, nil
}

func (d *BtrfsDriver) Exists(id string) bool {
	if err := validateLayerID(id); err != nil {
		return false
	}
	return common.PathExists(filepath.Join(d.root, id))
}

func (d *BtrfsDriver) Remove(id string) error {
	if err := validateLayerID(id); err != nil {
		return err
	}
	subvolPath := filepath.Join(d.root, id)
	return exec.Command("btrfs", "subvolume", "delete", subvolPath).Run()
}

func (d *BtrfsDriver) Cleanup() error  { return nil }
func (d *BtrfsDriver) GetMetadata(id string) (map[string]string, error) {
	return nil, nil
}

// ─── ZFS Driver ────────────────────────────────────────────────────

type ZFSDriver struct {
	root      string
	pool      string
	fsPrefix  string
}

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

func (d *ZFSDriver) Name() string { return "zfs" }

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

func (d *ZFSDriver) Exists(id string) bool {
	if err := validateLayerID(id); err != nil {
		return false
	}
	fsName := d.fsPrefix + "/" + id
	return exec.Command("zfs", "list", fsName).Run() == nil
}

func (d *ZFSDriver) Remove(id string) error {
	if err := validateLayerID(id); err != nil {
		return err
	}
	fsName := d.fsPrefix + "/" + id
	return exec.Command("zfs", "destroy", "-r", fsName).Run()
}

func (d *ZFSDriver) Cleanup() error  { return nil }
func (d *ZFSDriver) GetMetadata(id string) (map[string]string, error) {
	return nil, nil
}

// ─── VFS Driver (naive, for testing) ──────────────────────────────

type VFSDriver struct {
	root string
}

func NewVFSDriver(root string) (*VFSDriver, error) {
	_ = common.EnsureDir(root)
	return &VFSDriver{root: root}, nil
}

func (d *VFSDriver) Name() string { return "vfs" }

func (d *VFSDriver) Get(id, _ string) (string, error) {
	path := filepath.Join(d.root, id)
	if !common.PathExists(path) {
		return "", common.NewErrNotFound("layer", id)
	}
	return path, nil
}

func (d *VFSDriver) Put(id, _ string) (string, error) {
	return filepath.Join(d.root, id), nil
}

func (d *VFSDriver) Exists(id string) bool {
	return common.PathExists(filepath.Join(d.root, id))
}

func (d *VFSDriver) Remove(id string) error {
	return os.RemoveAll(filepath.Join(d.root, id))
}

func (d *VFSDriver) Cleanup() error  { return nil }
func (d *VFSDriver) GetMetadata(id string) (map[string]string, error) {
	return nil, nil
}

// ─── Garbage Collection ────────────────────────────────────────────

type GCConfig struct {
	Enabled       bool
	Interval      time.Duration
	MaxAge        time.Duration
	MinFreeSpace  int64
}

type GarbageCollector struct {
	store *Manager
	cfg   GCConfig
	stop  chan struct{}
	once  sync.Once
}

func NewGarbageCollector(store *Manager, cfg GCConfig) *GarbageCollector {
	return &GarbageCollector{store: store, cfg: cfg, stop: make(chan struct{})}
}

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
