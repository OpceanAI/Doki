package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestManagerCreateAndInspect(t *testing.T) {
	dir := t.TempDir()
	mm, err := NewManifestManager(dir)
	if err != nil {
		t.Fatalf("NewManifestManager: %v", err)
	}

	ml, err := mm.Create("ml1", []string{"alpine:3.19", "alpine:arm64"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ml.Name != "ml1" {
		t.Fatalf("unexpected name: %s", ml.Name)
	}
	if len(ml.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(ml.Images))
	}

	got, err := mm.Inspect("ml1")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.Name != "ml1" {
		t.Fatalf("unexpected inspect name: %s", got.Name)
	}
}

func TestManifestManagerValidation(t *testing.T) {
	dir := t.TempDir()
	mm, err := NewManifestManager(dir)
	if err != nil {
		t.Fatalf("NewManifestManager: %v", err)
	}
	if _, err := mm.Create("", nil); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestManifestManagerAddAndRemove(t *testing.T) {
	dir := t.TempDir()
	mm, err := NewManifestManager(dir)
	if err != nil {
		t.Fatalf("NewManifestManager: %v", err)
	}
	if _, err := mm.Create("ml2", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mm.Add("ml2", "ubuntu:22.04", Platform{Architecture: "amd64", OS: "linux"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, _ := mm.Inspect("ml2")
	if len(got.Images) != 1 || got.Images[0].Image != "ubuntu:22.04" {
		t.Fatalf("Add did not persist, got %+v", got.Images)
	}

	if err := mm.Annotate("ml2", "ubuntu:22.04", Platform{Architecture: "arm64", OS: "linux"}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	got, _ = mm.Inspect("ml2")
	if got.Images[0].Platform.Architecture != "arm64" {
		t.Fatalf("Annotate did not apply, got %+v", got.Images[0].Platform)
	}

	if err := mm.Remove("ml2", "ubuntu:22.04"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, _ = mm.Inspect("ml2")
	if len(got.Images) != 0 {
		t.Fatalf("expected empty after Remove, got %d", len(got.Images))
	}
}

func TestManifestManagerDelete(t *testing.T) {
	dir := t.TempDir()
	mm, err := NewManifestManager(dir)
	if err != nil {
		t.Fatalf("NewManifestManager: %v", err)
	}
	if _, err := mm.Create("todelete", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mm.Delete("todelete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if mm.Exists("todelete") {
		t.Fatal("Exists should be false after delete")
	}
	jsonPath := filepath.Join(dir, "manifests", "todelete.json")
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err=%v", err)
	}
}

func TestManifestManagerDuplicateName(t *testing.T) {
	dir := t.TempDir()
	mm, err := NewManifestManager(dir)
	if err != nil {
		t.Fatalf("NewManifestManager: %v", err)
	}
	if _, err := mm.Create("dup", nil); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err = mm.Create("dup", nil)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}
