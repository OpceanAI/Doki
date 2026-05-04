package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// ─── Btrfs Driver ──────────────────────────────────────────────────

type BtrfsDriver struct {
	root    string
	subvols map[string]string
	mu      sync.RWMutex
}

func NewBtrfsDriver(root string) (*BtrfsDriver, error) {
	if !isBtrfs(root) {
		return nil, fmt.Errorf("btrfs: %s is not a btrfs filesystem", root)
	}
	common.EnsureDir(root)
	return &BtrfsDriver{root: root, subvols: make(map[string]string)}, nil
}

func (d *BtrfsDriver) Name() string { return "btrfs" }

func isBtrfs(path string) bool {
	_ = path
	return false
}

func (d *BtrfsDriver) Get(id, _ string) (string, error) {
	subvolPath := filepath.Join(d.root, id)
	if !common.PathExists(subvolPath) {
		return "", common.NewErrNotFound("layer", id)
	}
	mountPath := filepath.Join(d.root, "mnt", id)
	common.EnsureDir(mountPath)
	if err := mountSubvol(subvolPath, mountPath); err != nil {
		return "", err
	}
	return mountPath, nil
}

func (d *BtrfsDriver) Put(id, _ string) (string, error) {
	mountPath := filepath.Join(d.root, "mnt", id)
	unmount(mountPath)
	return mountPath, nil
}

func (d *BtrfsDriver) Exists(id string) bool {
	return common.PathExists(filepath.Join(d.root, id))
}

func (d *BtrfsDriver) Remove(id string) error {
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
	mu        sync.RWMutex
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
	fsName := d.fsPrefix + "/" + id
	mountPath := filepath.Join(d.root, id)
	common.EnsureDir(mountPath)
	cmd := exec.Command("zfs", "mount", fsName)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return mountPath, nil
}

func (d *ZFSDriver) Put(id, _ string) (string, error) {
	fsName := d.fsPrefix + "/" + id
	exec.Command("zfs", "unmount", fsName).Run()
	return filepath.Join(d.root, id), nil
}

func (d *ZFSDriver) Exists(id string) bool {
	fsName := d.fsPrefix + "/" + id
	return exec.Command("zfs", "list", fsName).Run() == nil
}

func (d *ZFSDriver) Remove(id string) error {
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
	mu   sync.RWMutex
}

func NewVFSDriver(root string) (*VFSDriver, error) {
	common.EnsureDir(root)
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

func (g *GarbageCollector) Stop() { close(g.stop) }

func (g *GarbageCollector) collect() {
	unused, _ := g.findUnusedLayers()
	for _, id := range unused {
		g.store.Remove(id)
	}
}

func (g *GarbageCollector) findUnusedLayers() ([]string, error) {
	return nil, nil
}

// ─── Quota Support ──────────────────────────────────────────────────

type QuotaManager struct {
	root string
}

func NewQuotaManager(root string) *QuotaManager {
	return &QuotaManager{root: root}
}

// SetSize sets a size quota for a container layer.
func (q *QuotaManager) SetSize(id string, sizeBytes int64) error {
	if !isXFSWithPquota(q.root) {
		return fmt.Errorf("quota: backing filesystem does not support project quotas")
	}
	return setXFSProjectQuota(filepath.Join(q.root, id), sizeBytes)
}

func isXFSWithPquota(path string) bool {
	return false
}

func setXFSProjectQuota(path string, sizeBytes int64) error {
	return fmt.Errorf("not implemented")
}

// ─── Snapshot Management ───────────────────────────────────────────

type SnapshotManager struct {
	driver Driver
}

func NewSnapshotManager(driver Driver) *SnapshotManager {
	return &SnapshotManager{driver: driver}
}

func (s *SnapshotManager) CreateSnapshot(id, snapshotID string) error {
	switch s.driver.Name() {
	case "btrfs":
		return exec.Command("btrfs", "subvolume", "snapshot",
			filepath.Join(s.driver.(*BtrfsDriver).root, id),
			filepath.Join(s.driver.(*BtrfsDriver).root, snapshotID)).Run()
	case "zfs":
		zfs := s.driver.(*ZFSDriver)
		fsName := zfs.fsPrefix + "/" + id
		return exec.Command("zfs", "snapshot", fsName+"@"+snapshotID).Run()
	default:
		return fmt.Errorf("snapshots not supported by %s driver", s.driver.Name())
	}
}

func (s *SnapshotManager) ListSnapshots(id string) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *SnapshotManager) DeleteSnapshot(snapshotID string) error {
	return fmt.Errorf("not implemented")
}

// ─── Helpers ───────────────────────────────────────────────────────

func mountSubvol(src, dst string) error {
	common.EnsureDir(dst)
	cmd := exec.Command("mount", "-o", "subvol="+filepath.Base(src), src, dst)
	return cmd.Run()
}

func unmount(target string) error {
	cmd := exec.Command("umount", target)
	_ = cmd.Run()
	return nil
}
