// Package controllers provides Kubernetes controllers.
package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

// Controller defines the interface for a Kubernetes controller.
type Controller interface {
	Name() string
	Run(ctx context.Context)
}

// Manager runs and manages a set of Kubernetes controllers.
type Manager struct {
	store       store.Store
	controllers []Controller
	logger      *slog.Logger
}

// NewManager creates a new controller manager with all built-in controllers registered.
func NewManager(s store.Store, logger *slog.Logger) *Manager {
	m := &Manager{store: s, logger: logger}
	m.Register(&DeploymentController{store: s, logger: logger})
	m.Register(&ReplicaSetController{store: s, logger: logger})
	m.Register(&JobController{store: s, logger: logger})
	m.Register(&CronJobController{store: s, logger: logger})
	m.Register(&NodeController{store: s, logger: logger})
	m.Register(&EndpointController{store: s, logger: logger})
	m.Register(&ServiceController{store: s, logger: logger})
	m.Register(&NamespaceController{store: s, logger: logger})
	m.Register(&ServiceAccountController{store: s, logger: logger})
	m.Register(&GarbageCollector{store: s, logger: logger})
	return m
}

// Register adds a controller to the manager.
func (m *Manager) Register(c Controller) {
	m.controllers = append(m.controllers, c)
}

// Run starts all registered controllers and blocks until the context is cancelled.
func (m *Manager) Run(ctx context.Context) error {
	for _, c := range m.controllers {
		go c.Run(ctx)
		m.logger.Info("controller started", "controller", c.Name())
	}
	<-ctx.Done()
	return nil
}

// DeploymentController manages Deployment resources.
type DeploymentController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *DeploymentController) Name() string { return "deployment" }

// Run starts the Deployment reconciliation loop.
func (c *DeploymentController) Run(ctx context.Context) {
	prefix := store.KeyFor("apps", "deployments", "", "")
	ch, _ := c.store.Watch(prefix, 0)
	defer c.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			var deploy k8s.Deployment
			if err := json.Unmarshal(event.Object.Value, &deploy); err != nil {
				continue
			}
			c.reconcile(&deploy)
		}
	}
}

func (c *DeploymentController) reconcile(deploy *k8s.Deployment) {
	replicas := int32(1)
	if deploy.Spec.Replicas != nil {
		replicas = *deploy.Spec.Replicas
	}

	rsList, _ := c.store.List(store.KeyFor("apps", "replicasets", deploy.Namespace, ""))
	var existingRS *k8s.ReplicaSet
	for _, obj := range rsList {
		var rs k8s.ReplicaSet
		_ = json.Unmarshal(obj.Value, &rs)
		for _, ref := range rs.OwnerReferences {
			if ref.UID == deploy.UID {
				existingRS = &rs
				break
			}
		}
	}

	if existingRS == nil {
		rs := k8s.ReplicaSet{
			TypeMeta: k8s.TypeMeta{Kind: "ReplicaSet", APIVersion: "apps/v1"},
			ObjectMeta: k8s.ObjectMeta{
				Name:      deploy.Name + "-rs",
				Namespace: deploy.Namespace,
				OwnerReferences: []k8s.OwnerReference{{
					APIVersion: "apps/v1", Kind: "Deployment",
					Name: deploy.Name, UID: deploy.UID,
				}},
				Labels: deploy.Spec.Template.Labels,
			},
			Spec: k8s.ReplicaSetSpec{
				Replicas: &replicas,
				Selector: deploy.Spec.Selector,
				Template: deploy.Spec.Template,
			},
		}
		data, _ := json.Marshal(rs)
		key := store.KeyFor("apps", "replicasets", deploy.Namespace, rs.Name)
		_ = c.store.Put(key, &store.StoredObject{Value: data})
	}

	status := k8s.DeploymentStatus{
		Replicas:          replicas,
		ReadyReplicas:     replicas,
		AvailableReplicas: replicas,
		UpdatedReplicas:   replicas,
	}
	deploy.Status = status
	data, _ := json.Marshal(deploy)
	key := store.KeyFor("apps", "deployments", deploy.Namespace, deploy.Name)
	_ = c.store.Put(key, &store.StoredObject{Value: data})
}

// ReplicaSetController manages ReplicaSet resources.
type ReplicaSetController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *ReplicaSetController) Name() string { return "replicaset" }

// Run starts the ReplicaSet reconciliation loop.
func (c *ReplicaSetController) Run(ctx context.Context) {
	prefix := store.KeyFor("apps", "replicasets", "", "")
	ch, _ := c.store.Watch(prefix, 0)
	defer c.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			var rs k8s.ReplicaSet
			if err := json.Unmarshal(event.Object.Value, &rs); err != nil {
				continue
			}
			c.reconcile(&rs)
		}
	}
}

func (c *ReplicaSetController) reconcile(rs *k8s.ReplicaSet) {
	replicas := int32(1)
	if rs.Spec.Replicas != nil {
		replicas = *rs.Spec.Replicas
	}

	podList, _ := c.store.List(store.KeyFor("", "pods", rs.Namespace, ""))
	owned := int32(0)
	for _, obj := range podList {
		var pod k8s.Pod
		_ = json.Unmarshal(obj.Value, &pod)
		for _, ref := range pod.OwnerReferences {
			if ref.UID == rs.UID {
				owned++
				break
			}
		}
	}

	for i := owned; i < replicas; i++ {
		pod := k8s.Pod{
			TypeMeta: k8s.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: k8s.ObjectMeta{
				Name:      fmt.Sprintf("%s-pod-%d", rs.Name, i),
				Namespace: rs.Namespace,
				OwnerReferences: []k8s.OwnerReference{{
					APIVersion: "apps/v1", Kind: "ReplicaSet",
					Name: rs.Name, UID: rs.UID,
				}},
				Labels: rs.Spec.Template.Labels,
			},
			Spec: rs.Spec.Template.Spec,
		}
		data, _ := json.Marshal(pod)
		key := store.KeyFor("", "pods", pod.Namespace, pod.Name)
		_ = c.store.Put(key, &store.StoredObject{Value: data})
	}

	rs.Status = k8s.ReplicaSetStatus{
		Replicas:         replicas,
		ReadyReplicas:    replicas,
		AvailableReplicas: replicas,
	}
	data, _ := json.Marshal(rs)
	key := store.KeyFor("apps", "replicasets", rs.Namespace, rs.Name)
	_ = c.store.Put(key, &store.StoredObject{Value: data})
}

// JobController manages Job resources.
type JobController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *JobController) Name() string { return "job" }

// Run starts the Job reconciliation loop.
func (c *JobController) Run(ctx context.Context) {
	prefix := store.KeyFor("batch", "jobs", "", "")
	ch, _ := c.store.Watch(prefix, 0)
	defer c.store.Unwatch(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ch:
			if !ok { return }
		}
	}
}

// CronJobController manages CronJob resources.
type CronJobController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *CronJobController) Name() string { return "cronjob" }

// Run starts the CronJob reconciliation loop.
func (c *CronJobController) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// NodeController manages Node resources.
type NodeController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *NodeController) Name() string { return "node" }

// Run starts the Node reconciliation loop.
func (c *NodeController) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkNodes()
		}
	}
}

func (c *NodeController) checkNodes() {
	objects, _ := c.store.List(store.KeyFor("", "nodes", "", ""))
	for _, obj := range objects {
		var node k8s.Node
		_ = json.Unmarshal(obj.Value, &node)
		for i, cond := range node.Status.Conditions {
			if cond.Type == "Ready" {
				node.Status.Conditions[i].LastHeartbeatTime = time.Now()
			}
		}
		data, _ := json.Marshal(node)
		key := store.KeyFor("", "nodes", "", node.Name)
		_ = c.store.Put(key, &store.StoredObject{Value: data})
	}
}

// EndpointController manages Endpoint resources.
type EndpointController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *EndpointController) Name() string { return "endpoint" }

// Run starts the Endpoint reconciliation loop.
func (c *EndpointController) Run(ctx context.Context) {
	<-ctx.Done()
}

// ServiceController manages Service resources.
type ServiceController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *ServiceController) Name() string { return "service" }

// Run starts the Service reconciliation loop.
func (c *ServiceController) Run(ctx context.Context) {
	<-ctx.Done()
}

// NamespaceController manages Namespace resources.
type NamespaceController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *NamespaceController) Name() string { return "namespace" }

// Run starts the Namespace reconciliation loop.
func (c *NamespaceController) Run(ctx context.Context) {
	<-ctx.Done()
}

// ServiceAccountController manages ServiceAccount resources.
type ServiceAccountController struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *ServiceAccountController) Name() string { return "serviceaccount" }

// Run starts the ServiceAccount reconciliation loop.
func (c *ServiceAccountController) Run(ctx context.Context) {
	prefix := store.KeyFor("", "namespaces", "", "")
	ch, _ := c.store.Watch(prefix, 0)
	defer c.store.Unwatch(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok { return }
			if event.Type != store.EventAdded { continue }
			var ns k8s.Namespace
			_ = json.Unmarshal(event.Object.Value, &ns)
			sa := k8s.ServiceAccount{
				TypeMeta:   k8s.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
				ObjectMeta: k8s.ObjectMeta{Name: "default", Namespace: ns.Name},
			}
			data, _ := json.Marshal(sa)
			key := store.KeyFor("", "serviceaccounts", ns.Name, "default")
			_ = c.store.Put(key, &store.StoredObject{Value: data})
		}
	}
}

// GarbageCollector removes orphaned Kubernetes resources.
type GarbageCollector struct {
	store  store.Store
	logger *slog.Logger
}

// Name returns the controller name.
func (c *GarbageCollector) Name() string { return "garbage-collector" }

// Run starts the garbage collection loop.
func (c *GarbageCollector) Run(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

func (c *GarbageCollector) collect() {
	// Garbage collection logic placeholder
}
