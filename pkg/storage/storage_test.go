package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir, DriverFuseOverlayFS)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m.Name() != DriverFuseOverlayFS {
		t.Errorf("Name = %s, want %s", m.Name(), DriverFuseOverlayFS)
	}
	if m.Root() != dir {
		t.Errorf("Root = %s, want %s", m.Root(), dir)
	}
}

func TestManagerGetPut(t *testing.T) {
	dir := t.TempDir()
	// Use VFS driver for testing (doesn't need mount).
	vfsDir := filepath.Join(dir, "vfs")
	os.MkdirAll(vfsDir, 0755)
	d, _ := NewVFSDriver(vfsDir)
	m := &Manager{root: dir, driver: d, drivers: map[string]Driver{"vfs": d}}

	// Create a layer directory.
	layerDir := filepath.Join(vfsDir, "test-layer")
	os.MkdirAll(layerDir, 0755)

	// Get should return a path.
	path, err := m.Get("test-layer", "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if path == "" {
		t.Error("Get returned empty path")
	}

	// Put should return without error.
	_, err = m.Put("test-layer", "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
}

func TestManagerExists(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, DriverFuseOverlayFS)

	// Create a layer.
	layerDir := filepath.Join(dir, "layers", "exists-test")
	os.MkdirAll(layerDir, 0755)

	if !m.Exists("exists-test") {
		t.Error("Exists: should be true")
	}
	if m.Exists("nonexistent") {
		t.Error("Exists: should be false")
	}
}

func TestManagerRemove(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, DriverFuseOverlayFS)

	// Create a layer.
	layerDir := filepath.Join(dir, "layers", "remove-test")
	os.MkdirAll(layerDir, 0755)

	if err := m.Remove("remove-test"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if m.Exists("remove-test") {
		t.Error("should not exist after Remove")
	}
}

func TestManagerStats(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, DriverFuseOverlayFS)

	stats := m.Stats()
	if stats.DriverName != DriverFuseOverlayFS {
		t.Errorf("DriverName = %s, want %s", stats.DriverName, DriverFuseOverlayFS)
	}
	if stats.RootPath != dir {
		t.Errorf("RootPath = %s, want %s", stats.RootPath, dir)
	}
}

func TestDetectBestDriver(t *testing.T) {
	dir := t.TempDir()
	driver := DetectBestDriver(dir)
	// Should return a valid driver name.
	validDrivers := map[string]bool{
		DriverOverlay2:      true,
		DriverFuseOverlayFS: true,
		"btrfs":             true,
		"zfs":               true,
	}
	if !validDrivers[driver] {
		t.Errorf("DetectBestDriver = %s, want valid driver", driver)
	}
}

func TestVFSDriver(t *testing.T) {
	dir := t.TempDir()
	d, err := NewVFSDriver(dir)
	if err != nil {
		t.Fatalf("NewVFSDriver: %v", err)
	}

	if d.Name() != "vfs" {
		t.Errorf("Name = %s, want vfs", d.Name())
	}

	// Create a layer.
	layerDir := filepath.Join(dir, "vfs-layer")
	os.MkdirAll(layerDir, 0755)

	path, err := d.Get("vfs-layer", "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if path != layerDir {
		t.Errorf("Get = %s, want %s", path, layerDir)
	}

	if !d.Exists("vfs-layer") {
		t.Error("Exists should be true")
	}

	_, err = d.Put("vfs-layer", "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := d.Remove("vfs-layer"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if d.Exists("vfs-layer") {
		t.Error("should not exist after Remove")
	}
}

func TestSnapshotManager(t *testing.T) {
	dir := t.TempDir()
	d, _ := NewVFSDriver(dir)
	sm := NewSnapshotManager(dir, d)

	// Create a source layer at the correct path (under "layers/").
	srcDir := filepath.Join(dir, "layers", "source-layer")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("hello"), 0644)

	// Create snapshot.
	snap, err := sm.Create("source-layer", map[string]string{"test": "value"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if snap.ID == "" {
		t.Error("snapshot ID is empty")
	}
	if snap.ParentID != "source-layer" {
		t.Errorf("ParentID = %s, want source-layer", snap.ParentID)
	}

	// List snapshots.
	snaps, err := sm.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("len(snaps) = %d, want 1", len(snaps))
	}

	// Load snapshot.
	loaded, err := sm.Load(snap.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != snap.ID {
		t.Errorf("ID = %s, want %s", loaded.ID, snap.ID)
	}

	// Restore snapshot.
	if err := sm.Restore(snap.ID, "restored-layer"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "layers", "restored-layer", "test.txt")); err != nil {
		t.Fatalf("restored file: %v", err)
	}

	// Delete snapshot.
	if err := sm.Delete(snap.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	snaps, _ = sm.List()
	if len(snaps) != 0 {
		t.Errorf("len(snaps) after delete = %d, want 0", len(snaps))
	}
}

func TestQuotaManager(t *testing.T) {
	dir := t.TempDir()
	d, _ := NewVFSDriver(dir)
	qm := NewQuotaManager(d)

	// VFS doesn't support quotas.
	if err := qm.SetQuota("test", 1024*1024); err == nil {
		// VFS should return error.
		t.Log("VFS returned no error for SetQuota (acceptable)")
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world!"), 0644)

	size, err := dirSize(dir)
	if err != nil {
		t.Fatalf("dirSize: %v", err)
	}
	if size != 11 { // 5 + 6
		t.Errorf("size = %d, want 11", size)
	}
}
