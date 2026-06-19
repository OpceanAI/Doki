package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Snapshot represents a storage snapshot.
type Snapshot struct {
	ID        string            `json:"id"`
	ParentID  string            `json:"parent_id,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	Size      int64             `json:"size"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SnapshotManager manages storage snapshots.
type SnapshotManager struct {
	root   string
	driver Driver
}

// NewSnapshotManager creates a new snapshot manager.
func NewSnapshotManager(root string, driver Driver) *SnapshotManager {
	_ = common.EnsureDir(filepath.Join(root, "snapshots"))
	return &SnapshotManager{root: root, driver: driver}
}

// Create creates a snapshot of a container layer.
func (sm *SnapshotManager) Create(id string, metadata map[string]string) (*Snapshot, error) {
	snap := &Snapshot{
		ID:        common.GenerateID(32),
		ParentID:  id,
		CreatedAt: time.Now(),
		Metadata:  metadata,
	}

	snapDir := filepath.Join(sm.root, "snapshots", snap.ID)
	if err := common.EnsureDir(snapDir); err != nil {
		return nil, err
	}

	// Copy the layer data to the snapshot directory.
	// BUG-02 fix: layers are stored under "<root>/layers/<id>", not
	// "<root>/<id>". The previous code used the wrong path, causing
	// the snapshot to either fail with ErrNotFound or snapshot wrong data.
	srcDir := filepath.Join(sm.root, "layers", id)
	if !common.PathExists(srcDir) {
		return nil, common.NewErrNotFound("layer", id)
	}

	if err := common.CopyDir(srcDir, snapDir); err != nil {
		_ = os.RemoveAll(snapDir)
		return nil, fmt.Errorf("copy layer: %w", err)
	}

	// Calculate size.
	size, _ := dirSize(snapDir)
	snap.Size = size

	// Save snapshot metadata.
	if err := sm.saveSnapshot(snap); err != nil {
		_ = os.RemoveAll(snapDir)
		return nil, err
	}

	return snap, nil
}

// List returns all snapshots.
func (sm *SnapshotManager) List() ([]*Snapshot, error) {
	snapDir := filepath.Join(sm.root, "snapshots")
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		return nil, err
	}

	var snapshots []*Snapshot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		snap, err := sm.Load(entry.Name())
		if err != nil {
			continue
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// Load loads a snapshot by ID.
func (sm *SnapshotManager) Load(id string) (*Snapshot, error) {
	metaPath := filepath.Join(sm.root, "snapshots", id, "snapshot.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", id, err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot %s: %w", id, err)
	}
	return &snap, nil
}

// Delete deletes a snapshot.
func (sm *SnapshotManager) Delete(id string) error {
	snapDir := filepath.Join(sm.root, "snapshots", id)
	if !common.PathExists(snapDir) {
		return common.NewErrNotFound("snapshot", id)
	}
	return os.RemoveAll(snapDir)
}

// Restore restores a snapshot to a container layer.
func (sm *SnapshotManager) Restore(snapID, targetID string) error {
	snapDir := filepath.Join(sm.root, "snapshots", snapID)
	if !common.PathExists(snapDir) {
		return common.NewErrNotFound("snapshot", snapID)
	}

	// BUG-02 fix: same path fix as Create — layers are under "<root>/layers/<id>".
	targetDir := filepath.Join(sm.root, "layers", targetID)
	// Remove existing target.
	_ = os.RemoveAll(targetDir)

	// Copy snapshot to target.
	if err := common.CopyDir(snapDir, targetDir); err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}

	// Remove snapshot metadata from target (it's a snapshot, not a container).
	_ = os.Remove(filepath.Join(targetDir, "snapshot.json"))

	return nil
}

func (sm *SnapshotManager) saveSnapshot(snap *Snapshot) error {
	metaPath := filepath.Join(sm.root, "snapshots", snap.ID, "snapshot.json")
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
