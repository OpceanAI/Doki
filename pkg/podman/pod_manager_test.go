package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPodManagerCreateAndList(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPodManager(dir)
	if err != nil {
		t.Fatalf("NewPodManager: %v", err)
	}

	pod, err := pm.CreatePod(&PodCreateConfig{Name: "web", Hostname: "web.local"})
	if err != nil {
		t.Fatalf("CreatePod: %v", err)
	}
	if pod.Name != "web" {
		t.Fatalf("unexpected pod name: %s", pod.Name)
	}
	if pod.State != "Created" {
		t.Fatalf("unexpected state: %s", pod.State)
	}

	pods := pm.ListPods()
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}

	if _, err := pm.GetPod("web"); err != nil {
		t.Fatalf("GetPod by name: %v", err)
	}
	if _, err := pm.GetPod(pod.ID); err != nil {
		t.Fatalf("GetPod by id: %v", err)
	}
}

func TestPodManagerValidation(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPodManager(dir)
	if err != nil {
		t.Fatalf("NewPodManager: %v", err)
	}

	if _, err := pm.CreatePod(&PodCreateConfig{Name: ""}); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := pm.CreatePod(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestPodManagerLifecycleAndPersistence(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPodManager(dir)
	if err != nil {
		t.Fatalf("NewPodManager: %v", err)
	}

	pod, err := pm.CreatePod(&PodCreateConfig{Name: "lifecycle"})
	if err != nil {
		t.Fatalf("CreatePod: %v", err)
	}

	if err := pm.StartPod(pod.ID); err != nil {
		t.Fatalf("StartPod: %v", err)
	}
	got, _ := pm.GetPod(pod.ID)
	if got.State != "Running" {
		t.Fatalf("expected Running, got %s", got.State)
	}

	if err := pm.StopPod(pod.ID); err != nil {
		t.Fatalf("StopPod: %v", err)
	}
	got, _ = pm.GetPod(pod.ID)
	if got.State != "Stopped" {
		t.Fatalf("expected Stopped, got %s", got.State)
	}

	if err := pm.KillPod(pod.ID, "SIGKILL"); err != nil {
		t.Fatalf("KillPod: %v", err)
	}
	got, _ = pm.GetPod(pod.ID)
	if got.State != "Exited" {
		t.Fatalf("expected Exited, got %s", got.State)
	}

	if !pm.Exists(pod.ID) {
		t.Fatal("Exists should be true")
	}
	if pm.Exists("missing") {
		t.Fatal("Exists should be false")
	}

	if err := pm.RemovePod(pod.ID, true); err != nil {
		t.Fatalf("RemovePod: %v", err)
	}
	if pm.Exists(pod.ID) {
		t.Fatal("pod should be removed")
	}

	jsonPath := filepath.Join(dir, "pods", pod.ID+".json")
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err=%v", err)
	}
}

func TestPodManagerDuplicateName(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPodManager(dir)
	if err != nil {
		t.Fatalf("NewPodManager: %v", err)
	}
	if _, err := pm.CreatePod(&PodCreateConfig{Name: "dup"}); err != nil {
		t.Fatalf("first CreatePod: %v", err)
	}
	_, err = pm.CreatePod(&PodCreateConfig{Name: "dup"})
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}
