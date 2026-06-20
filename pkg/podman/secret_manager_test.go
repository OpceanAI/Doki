package podman

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretManagerCreateAndGet(t *testing.T) {
	dir := t.TempDir()
	sm := NewSecretManager(dir)

	secret, err := sm.Create("api-key", []byte("super-secret"), "", map[string]string{"env": "prod"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if secret.Spec.Name != "api-key" {
		t.Fatalf("unexpected name: %s", secret.Spec.Name)
	}
	if secret.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	got, err := sm.Get("api-key")
	if err != nil {
		t.Fatalf("Get by name: %v", err)
	}
	if got.ID != secret.ID {
		t.Fatalf("ID mismatch: %s vs %s", got.ID, secret.ID)
	}

	data, err := sm.GetData("api-key")
	if err != nil {
		t.Fatalf("GetData: %v", err)
	}
	if !bytes.Equal(data, []byte("super-secret")) {
		t.Fatalf("decrypted mismatch: %q", data)
	}
}

func TestSecretManagerValidation(t *testing.T) {
	dir := t.TempDir()
	sm := NewSecretManager(dir)
	if _, err := sm.Create("", []byte("x"), "", nil); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestSecretManagerDuplicateDetectedByName(t *testing.T) {
	dir := t.TempDir()
	sm := NewSecretManager(dir)
	if _, err := sm.Create("dup", []byte("a"), "", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := sm.Create("dup", []byte("b"), "", nil)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestSecretManagerRemove(t *testing.T) {
	dir := t.TempDir()
	sm := NewSecretManager(dir)
	secret, err := sm.Create("to-remove", []byte("value"), "", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sm.Remove("to-remove"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if sm.Exists("to-remove") {
		t.Fatal("Exists should be false after remove")
	}

	encPath := filepath.Join(dir, "secrets", secret.ID+".enc")
	if _, err := os.Stat(encPath); !os.IsNotExist(err) {
		t.Fatalf("expected enc file removed, stat err=%v", err)
	}
	linkPath := filepath.Join(dir, "secrets", "names", "to-remove")
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("expected name link removed, stat err=%v", err)
	}
}

func TestSecretManagerPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	sm1 := NewSecretManager(dir)
	if _, err := sm1.Create("persist", []byte("payload"), "", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}

	sm2 := NewSecretManager(dir)
	got, err := sm2.GetData("persist")
	if err != nil {
		t.Fatalf("GetData after restart: %v", err)
	}
	if !bytes.Equal(got, []byte("payload")) {
		t.Fatalf("decrypt mismatch: %q", got)
	}
}
