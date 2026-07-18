package controllers

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	k8s "github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

func TestPVCProvisionerBindsClaim(t *testing.T) {
	s := store.NewMemoryStore()
	root := t.TempDir()
	c := NewPVCProvisionerController(s, slog.New(slog.NewTextHandler(io.Discard, nil)), root)

	sc := "standard"
	pvc := k8s.PersistentVolumeClaim{
		TypeMeta:   k8s.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: k8s.ObjectMeta{Name: "data", Namespace: "default", UID: "uid-123"},
		Spec: k8s.PersistentVolumeClaimSpec{
			AccessModes:      []string{"ReadWriteOnce"},
			StorageClassName: &sc,
			Resources:        k8s.ResourceRequirements{Requests: k8s.ResourceList{"storage": "1Gi"}},
		},
	}
	data, _ := json.Marshal(pvc)
	c.handleEvent(store.WatchEvent{Type: store.EventAdded, Object: &store.StoredObject{Value: data}})

	// The PVC must now be Bound with a volume name.
	obj, err := s.Get(store.KeyFor("", "persistentvolumeclaims", "default", "data"))
	if err != nil || obj == nil {
		t.Fatalf("pvc not found after provision: %v", err)
	}
	var bound k8s.PersistentVolumeClaim
	_ = json.Unmarshal(obj.Value, &bound)
	if bound.Status.Phase != "Bound" {
		t.Fatalf("pvc phase = %q, want Bound", bound.Status.Phase)
	}
	if bound.Spec.VolumeName == "" {
		t.Fatal("pvc has no bound volume name")
	}

	// The PV must exist, be Bound, and point at a real host directory.
	pvObj, err := s.Get(store.KeyFor("", "persistentvolumes", "", bound.Spec.VolumeName))
	if err != nil || pvObj == nil {
		t.Fatalf("pv %q not created", bound.Spec.VolumeName)
	}
	var pv k8s.PersistentVolume
	_ = json.Unmarshal(pvObj.Value, &pv)
	if pv.Spec.HostPath == nil || pv.Spec.HostPath.Path == "" {
		t.Fatal("pv has no hostPath")
	}
	if _, err := os.Stat(pv.Spec.HostPath.Path); err != nil {
		t.Fatalf("pv host dir not created: %v", err)
	}
	if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.Name != "data" {
		t.Fatal("pv claimRef not set to the PVC")
	}

	// Idempotent: re-processing the now-bound PVC must not create a second PV.
	data2, _ := json.Marshal(bound)
	c.handleEvent(store.WatchEvent{Type: store.EventModified, Object: &store.StoredObject{Value: data2}})
	pvs, _ := s.List(store.KeyFor("", "persistentvolumes", "", ""))
	if len(pvs) != 1 {
		t.Fatalf("expected exactly 1 PV, got %d", len(pvs))
	}
}

func TestPVCProvisionerSkipsForeignClass(t *testing.T) {
	s := store.NewMemoryStore()
	c := NewPVCProvisionerController(s, slog.New(slog.NewTextHandler(io.Discard, nil)), t.TempDir())
	foreign := "ceph-rbd"
	pvc := k8s.PersistentVolumeClaim{
		ObjectMeta: k8s.ObjectMeta{Name: "x", Namespace: "default", UID: "u"},
		Spec:       k8s.PersistentVolumeClaimSpec{StorageClassName: &foreign},
	}
	data, _ := json.Marshal(pvc)
	c.handleEvent(store.WatchEvent{Type: store.EventAdded, Object: &store.StoredObject{Value: data}})
	pvs, _ := s.List(store.KeyFor("", "persistentvolumes", "", ""))
	if len(pvs) != 0 {
		t.Fatalf("foreign storage class should not be provisioned, got %d PVs", len(pvs))
	}
}
