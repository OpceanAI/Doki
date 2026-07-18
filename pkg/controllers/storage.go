package controllers

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	k8s "github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

// PVCProvisionerController is a local-path style dynamic provisioner. It watches
// PersistentVolumeClaims and, for each unbound claim on a provisionable storage
// class, creates a host directory, publishes a matching PersistentVolume
// (hostPath) and binds the two. This is the single-node, no-CSI-gRPC path: on
// one node with one process, CreateVolume is just mkdir and the bind is a
// store update — exactly what a real external-provisioner would do over CSI,
// minus the protobuf round-trip to talk to ourselves.
type PVCProvisionerController struct {
	store  store.Store
	logger *slog.Logger
	root   string // base dir for provisioned volume directories
}

// NewPVCProvisionerController creates the provisioner rooted at dir.
func NewPVCProvisionerController(s store.Store, logger *slog.Logger, dir string) *PVCProvisionerController {
	return &PVCProvisionerController{store: s, logger: logger, root: dir}
}

func (c *PVCProvisionerController) Name() string { return "pvc-provisioner" }

func (c *PVCProvisionerController) Run(ctx context.Context) {
	watchLoop(ctx, c.store, store.KeyFor("", "persistentvolumeclaims", "", ""), c.logger, c.handleEvent)
}

func (c *PVCProvisionerController) handleEvent(event store.WatchEvent) {
	if event.Object == nil {
		return
	}
	var pvc k8s.PersistentVolumeClaim
	if err := json.Unmarshal(event.Object.Value, &pvc); err != nil {
		return
	}
	if event.Type == store.EventDeleted {
		c.deprovision(&pvc)
		return
	}
	c.provision(&pvc)
}

// provisionable reports whether this claim's storage class is one we handle.
// A nil/empty class, "standard", or "local-path" are provisioned; an explicit
// foreign class is left for its own provisioner.
func provisionable(pvc *k8s.PersistentVolumeClaim) bool {
	if pvc.Spec.StorageClassName == nil {
		return true
	}
	switch *pvc.Spec.StorageClassName {
	case "", "standard", "local-path":
		return true
	}
	return false
}

func (c *PVCProvisionerController) provision(pvc *k8s.PersistentVolumeClaim) {
	if pvc.Status.Phase == "Bound" || pvc.Spec.VolumeName != "" {
		return
	}
	if !provisionable(pvc) {
		return
	}

	uid := pvc.UID
	if uid == "" {
		uid = pvc.Namespace + "-" + pvc.Name
	}
	pvName := "pvc-" + uid
	dir := filepath.Join(c.root, pvName)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		c.logger.Warn("provisioner: mkdir", "pvc", pvc.Name, "err", err)
		return
	}

	className := ""
	if pvc.Spec.StorageClassName != nil {
		className = *pvc.Spec.StorageClassName
	}
	capacity := pvc.Spec.Resources.Requests // advisory: no quota on fuse-overlayfs

	pv := k8s.PersistentVolume{
		TypeMeta:   k8s.TypeMeta{Kind: "PersistentVolume", APIVersion: "v1"},
		ObjectMeta: k8s.ObjectMeta{Name: pvName},
		Spec: k8s.PersistentVolumeSpec{
			Capacity:                      capacity,
			AccessModes:                   pvc.Spec.AccessModes,
			PersistentVolumeReclaimPolicy: "Delete",
			StorageClassName:              className,
			HostPath:                      &k8s.HostPathVolumeSource{Path: dir},
			ClaimRef: &k8s.ObjectReference{
				Kind: "PersistentVolumeClaim", Namespace: pvc.Namespace,
				Name: pvc.Name, UID: pvc.UID, APIVersion: "v1",
			},
		},
		Status: k8s.PersistentVolumeStatus{Phase: "Bound"},
	}
	pvData, _ := json.Marshal(pv)
	if err := c.store.Put(store.KeyFor("", "persistentvolumes", "", pvName), &store.StoredObject{Value: pvData}); err != nil {
		c.logger.Warn("provisioner: put pv", "pv", pvName, "err", err)
		return
	}

	pvc.Spec.VolumeName = pvName
	pvc.Status.Phase = "Bound"
	pvc.Status.Capacity = capacity
	if len(pvc.Status.AccessModes) == 0 {
		pvc.Status.AccessModes = pvc.Spec.AccessModes
	}
	pvcData, _ := json.Marshal(pvc)
	if err := c.store.Put(store.KeyFor("", "persistentvolumeclaims", pvc.Namespace, pvc.Name), &store.StoredObject{Value: pvcData}); err != nil {
		c.logger.Warn("provisioner: bind pvc", "pvc", pvc.Name, "err", err)
		return
	}
	c.logger.Info("provisioned volume", "pvc", pvc.Namespace+"/"+pvc.Name, "pv", pvName, "path", dir)
}

func (c *PVCProvisionerController) deprovision(pvc *k8s.PersistentVolumeClaim) {
	if pvc.Spec.VolumeName == "" {
		return
	}
	// Only reclaim what we own (Delete policy) — read the PV back to check.
	pvKey := store.KeyFor("", "persistentvolumes", "", pvc.Spec.VolumeName)
	obj, err := c.store.Get(pvKey)
	if err != nil || obj == nil {
		return
	}
	var pv k8s.PersistentVolume
	if err := json.Unmarshal(obj.Value, &pv); err != nil {
		return
	}
	if pv.Spec.PersistentVolumeReclaimPolicy == "Delete" && pv.Spec.HostPath != nil {
		if err := os.RemoveAll(pv.Spec.HostPath.Path); err != nil {
			c.logger.Warn("deprovision: rm dir", "path", pv.Spec.HostPath.Path, "err", err)
		}
		_ = c.store.Delete(pvKey)
		c.logger.Info("deprovisioned volume", "pv", pvc.Spec.VolumeName)
	}
}
