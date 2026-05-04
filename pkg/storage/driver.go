package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Driver is the interface for storage drivers.
type Driver interface {
	Get(id, mountLabel string) (string, error)
	Put(id, mountLabel string) (string, error)
	Exists(id string) bool
	Cleanup() error
	Remove(id string) error
	GetMetadata(id string) (map[string]string, error)
	Name() string
}

const (
	DriverFuseOverlayFS = "fuse-overlayfs"
	DriverOverlay2      = "overlay2"
)

// Manager manages storage drivers.
type Manager struct {
	mu      sync.RWMutex
	driver  Driver
	root    string
	drivers map[string]Driver
}

// NewManager creates a new storage manager.
func NewManager(root string, driverName string) (*Manager, error) {
	m := &Manager{
		root:    root,
		drivers: make(map[string]Driver),
	}

	common.EnsureDir(root)

	var driver Driver
	var err error

	switch driverName {
	case DriverFuseOverlayFS:
		driver, err = NewFuseOverlayFSDriver(root)
	case DriverOverlay2:
		driver, err = NewOverlay2Driver(root)
	default:
		driver, err = NewFuseOverlayFSDriver(root)
	}

	if err != nil {
		return nil, fmt.Errorf("create storage driver %s: %w", driverName, err)
	}

	m.driver = driver
	m.drivers[driverName] = driver

	return m, nil
}

// Get returns the path to a container layer directory.
func (m *Manager) Get(id, mountLabel string) (string, error) {
	return m.driver.Get(id, mountLabel)
}

// Put releases a container layer.
func (m *Manager) Put(id, mountLabel string) (string, error) {
	return m.driver.Put(id, mountLabel)
}

// Exists checks if a container layer exists.
func (m *Manager) Exists(id string) bool {
	return m.driver.Exists(id)
}

// Remove removes a container layer.
func (m *Manager) Remove(id string) error {
	return m.driver.Remove(id)
}

// Cleanup performs cleanup of the storage driver.
func (m *Manager) Cleanup() error {
	return m.driver.Cleanup()
}

// GetMetadata returns metadata for a container layer.
func (m *Manager) GetMetadata(id string) (map[string]string, error) {
	return m.driver.GetMetadata(id)
}

// Name returns the name of the current storage driver.
func (m *Manager) Name() string {
	return m.driver.Name()
}

// Root returns the root directory for storage.
func (m *Manager) Root() string {
	return m.root
}

// Driver returns the current driver.
func (m *Manager) Driver() Driver {
	return m.driver
}

// FuseOverlayFSDriver implements the Driver interface using fuse-overlayfs.
type FuseOverlayFSDriver struct {
	root      string
	layerDir  string
	mergeDir  string
	upperDir  string
	workDir   string
}

// NewFuseOverlayFSDriver creates a new fuse-overlayfs driver.
func NewFuseOverlayFSDriver(root string) (*FuseOverlayFSDriver, error) {
	d := &FuseOverlayFSDriver{
		root:     root,
		layerDir: filepath.Join(root, "layers"),
		mergeDir: filepath.Join(root, "merged"),
		upperDir: filepath.Join(root, "diff"),
		workDir:  filepath.Join(root, "work"),
	}

	for _, dir := range []string{d.layerDir, d.mergeDir, d.upperDir, d.workDir} {
		if err := common.EnsureDir(dir); err != nil {
			return nil, err
		}
	}

	return d, nil
}

func (d *FuseOverlayFSDriver) Name() string {
	return DriverFuseOverlayFS
}

func (d *FuseOverlayFSDriver) Get(id, mountLabel string) (string, error) {
	layerPath := filepath.Join(d.layerDir, id)
	if !common.PathExists(layerPath) {
		return "", common.NewErrNotFound("layer", id)
	}

	mergePath := filepath.Join(d.mergeDir, id)
	if err := common.EnsureDir(mergePath); err != nil {
		return "", err
	}

	upperPath := filepath.Join(d.upperDir, id)
	if err := common.EnsureDir(upperPath); err != nil {
		return "", err
	}

	workPath := filepath.Join(d.workDir, id)
	if err := common.EnsureDir(workPath); err != nil {
		return "", err
	}

	// Collect lower dirs.
	var lowerDirs []string
	layers, err := d.getLowerDirs(id)
	if err != nil {
		return "", err
	}
	lowerDirs = append(lowerDirs, layers...)

	if len(lowerDirs) == 0 {
		lowerDirs = append(lowerDirs, layerPath)
	}

	// Mount the overlayfs.
	lowerStr := strings.Join(lowerDirs, ":")
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerStr, upperPath, workPath)

	if err := mountOverlay("overlay", mergePath, "overlay", 0, opts); err != nil {
		return "", fmt.Errorf("mount overlay for %s: %w", id, err)
	}

	return mergePath, nil
}

func (d *FuseOverlayFSDriver) Put(id, mountLabel string) (string, error) {
	mergePath := filepath.Join(d.mergeDir, id)
	if common.PathExists(mergePath) {
		unmountOverlay(mergePath)
	}
	return mergePath, nil
}

func (d *FuseOverlayFSDriver) Exists(id string) bool {
	layerPath := filepath.Join(d.layerDir, id)
	return common.PathExists(layerPath)
}

func (d *FuseOverlayFSDriver) Remove(id string) error {
	for _, dir := range []string{
		filepath.Join(d.layerDir, id),
		filepath.Join(d.mergeDir, id),
		filepath.Join(d.upperDir, id),
		filepath.Join(d.workDir, id),
	} {
		os.RemoveAll(dir)
	}

	return nil
}

func (d *FuseOverlayFSDriver) Cleanup() error {
	for _, dir := range []string{d.mergeDir, d.workDir} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			unmountOverlay(filepath.Join(dir, entry.Name()))
		}
	}
	return nil
}

func (d *FuseOverlayFSDriver) GetMetadata(id string) (map[string]string, error) {
	metaPath := filepath.Join(d.layerDir, id, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var meta map[string]string
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func (d *FuseOverlayFSDriver) getLowerDirs(id string) ([]string, error) {
	parentPath := filepath.Join(d.layerDir, id, "parent")
	if !common.PathExists(parentPath) {
		return nil, nil
	}

	data, err := os.ReadFile(parentPath)
	if err != nil {
		return nil, err
	}

	parents := strings.Fields(string(data))
	var lowerDirs []string
	for _, parent := range parents {
		lowerDirs = append(lowerDirs, filepath.Join(d.layerDir, parent))
		parentLowerDirs, _ := d.getLowerDirs(parent)
		lowerDirs = append(lowerDirs, parentLowerDirs...)
	}

	return lowerDirs, nil
}

// Overlay2Driver implements the Driver interface using native overlay2.
type Overlay2Driver struct {
	root     string
	layerDir string
	mergeDir string
	upperDir string
	workDir  string
}

// NewOverlay2Driver creates a new overlay2 driver.
func NewOverlay2Driver(root string) (*Overlay2Driver, error) {
	d := &Overlay2Driver{
		root:     root,
		layerDir: filepath.Join(root, "layers"),
		mergeDir: filepath.Join(root, "merged"),
		upperDir: filepath.Join(root, "diff"),
		workDir:  filepath.Join(root, "work"),
	}

	for _, dir := range []string{d.layerDir, d.mergeDir, d.upperDir, d.workDir} {
		if err := common.EnsureDir(dir); err != nil {
			return nil, err
		}
	}

	return d, nil
}

func (d *Overlay2Driver) Name() string {
	return DriverOverlay2
}

func (d *Overlay2Driver) Get(id, mountLabel string) (string, error) {
	return (&FuseOverlayFSDriver{
		root:     d.root,
		layerDir: d.layerDir,
		mergeDir: d.mergeDir,
		upperDir: d.upperDir,
		workDir:  d.workDir,
	}).Get(id, mountLabel)
}

func (d *Overlay2Driver) Put(id, mountLabel string) (string, error) {
	return (&FuseOverlayFSDriver{
		root:     d.root,
		layerDir: d.layerDir,
		mergeDir: d.mergeDir,
		upperDir: d.upperDir,
		workDir:  d.workDir,
	}).Put(id, mountLabel)
}

func (d *Overlay2Driver) Exists(id string) bool {
	layerPath := filepath.Join(d.layerDir, id)
	return common.PathExists(layerPath)
}

func (d *Overlay2Driver) Remove(id string) error {
	for _, dir := range []string{
		filepath.Join(d.layerDir, id),
		filepath.Join(d.mergeDir, id),
		filepath.Join(d.upperDir, id),
		filepath.Join(d.workDir, id),
	} {
		os.RemoveAll(dir)
	}
	return nil
}

func (d *Overlay2Driver) Cleanup() error {
	return nil
}

func (d *Overlay2Driver) GetMetadata(id string) (map[string]string, error) {
	metaPath := filepath.Join(d.layerDir, id, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var meta map[string]string
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// Helper functions.
func mountOverlay(source, target, fstype string, flags uintptr, data string) error {
	return osMnt(source, target, fstype, flags, data)
}

func unmountOverlay(target string) error {
	return osUnmount(target)
}

// EnsureDriverDir creates driver directories.
func EnsureDriverDir(root string) error {
	dirs := []string{"layers", "merged", "diff", "work", "containers", "volumes", "images"}
	for _, dir := range dirs {
		if err := common.EnsureDir(filepath.Join(root, dir)); err != nil {
			return err
		}
	}
	return nil
}

// SaveLayerMetadata saves metadata for a layer.
func SaveLayerMetadata(root, id string, meta map[string]string) error {
	layerPath := filepath.Join(root, "layers", id)
	if err := common.EnsureDir(layerPath); err != nil {
		return err
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(layerPath, "metadata.json"), data, 0644)
}

// LinkParent links a child layer to its parent.
func LinkParent(root, childID, parentID string) error {
	childPath := filepath.Join(root, "layers", childID)
	if err := common.EnsureDir(childPath); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(childPath, "parent"), []byte(parentID+"\n"), 0644)
}
