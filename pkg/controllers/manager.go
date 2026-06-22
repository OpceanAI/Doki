// Package controllers provides Kubernetes controllers.
package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strconv"
	"strings"
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

// watchLoop establishes a watch on prefix and dispatches every event to handler.
// It retries with exponential backoff on watch errors or channel close until the
// context is cancelled, so a transient store failure never permanently stops a
// controller.
func watchLoop(ctx context.Context, s store.Store, prefix string, logger *slog.Logger, handler func(store.WatchEvent)) {
	const maxBackoff = 30 * time.Second
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		ch, err := s.Watch(prefix, 0)
		if err != nil {
			if logger != nil {
				logger.Error("watch failed, retrying", "prefix", prefix, "err", err, "backoff", backoff)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		channelClosed := false
		for !channelClosed {
			select {
			case <-ctx.Done():
				s.Unwatch(ch)
				return
			case event, ok := <-ch:
				if !ok {
					channelClosed = true
				} else {
					handler(event)
				}
			}
		}
		s.Unwatch(ch)
		if logger != nil {
			logger.Warn("watch channel closed, retrying", "prefix", prefix, "backoff", backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// labelsMatch returns true if podLabels satisfies every key/value pair in selector.
// An empty selector matches nothing (a selector-less service is not a pod selector).
func labelsMatch(podLabels, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}

// podReady reports whether a pod has the Ready condition set to True.
func podReady(pod *k8s.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == "Ready" {
			return cond.Status == "True"
		}
	}
	return false
}

// metaName is a minimal projection used to extract an object's name from its
// JSON payload without unmarshaling the full typed object.
type metaName struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
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
	watchLoop(ctx, c.store, prefix, c.logger, c.handleEvent)
}

func (c *DeploymentController) handleEvent(event store.WatchEvent) {
	if event.Object == nil {
		return
	}
	var deploy k8s.Deployment
	if err := json.Unmarshal(event.Object.Value, &deploy); err != nil {
		return
	}
	c.reconcile(&deploy)
}

func (c *DeploymentController) reconcile(deploy *k8s.Deployment) {
	replicas := int32(1)
	if deploy.Spec.Replicas != nil {
		replicas = *deploy.Spec.Replicas
	}

	rsList, _ := c.store.List(store.KeyFor("apps", "replicasets", deploy.Namespace, ""))
	var existingRS *k8s.ReplicaSet
outer:
	for _, obj := range rsList {
		var rs k8s.ReplicaSet
		_ = json.Unmarshal(obj.Value, &rs)
		for _, ref := range rs.OwnerReferences {
			if ref.UID == deploy.UID {
				existingRS = &rs
				break outer
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

	// Count the actual pods owned by this deployment's ReplicaSet(s) so the
	// status reflects observed state rather than the desired replica count.
	var totalReplicas, readyReplicas int32
	if existingRS != nil {
		podList, _ := c.store.List(store.KeyFor("", "pods", deploy.Namespace, ""))
		for _, obj := range podList {
			var pod k8s.Pod
			if err := json.Unmarshal(obj.Value, &pod); err != nil {
				continue
			}
			for _, ref := range pod.OwnerReferences {
				if ref.UID == existingRS.UID {
					totalReplicas++
					if pod.Status.Phase == k8s.PodRunning {
						readyReplicas++
					}
					break
				}
			}
		}
	}

	status := k8s.DeploymentStatus{
		Replicas:          totalReplicas,
		ReadyReplicas:     readyReplicas,
		AvailableReplicas: readyReplicas,
		UpdatedReplicas:   readyReplicas,
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
	watchLoop(ctx, c.store, prefix, c.logger, c.handleEvent)
}

func (c *ReplicaSetController) handleEvent(event store.WatchEvent) {
	if event.Object == nil {
		return
	}
	var rs k8s.ReplicaSet
	if err := json.Unmarshal(event.Object.Value, &rs); err != nil {
		return
	}
	c.reconcile(&rs)
}

func (c *ReplicaSetController) reconcile(rs *k8s.ReplicaSet) {
	replicas := int32(1)
	if rs.Spec.Replicas != nil {
		replicas = *rs.Spec.Replicas
	}

	podList, _ := c.store.List(store.KeyFor("", "pods", rs.Namespace, ""))
	var ownedPods []k8s.Pod
	for _, obj := range podList {
		var pod k8s.Pod
		if err := json.Unmarshal(obj.Value, &pod); err != nil {
			continue
		}
		for _, ref := range pod.OwnerReferences {
			if ref.UID == rs.UID {
				ownedPods = append(ownedPods, pod)
				break
			}
		}
	}
	owned := int32(len(ownedPods))

	// Create missing pods up to the desired replica count.
	for i := owned; i < replicas; i++ {
		pod := k8s.Pod{
			TypeMeta: k8s.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: k8s.ObjectMeta{
				Name:              fmt.Sprintf("%s-pod-%d", rs.Name, i),
				Namespace:         rs.Namespace,
				CreationTimestamp: time.Now(),
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
		ownedPods = append(ownedPods, pod)
	}

	// Delete excess pods when the desired count has been scaled down. The most
	// recently created pods are removed first so stable, older pods survive.
	if owned > replicas {
		sort.Slice(ownedPods, func(i, j int) bool {
			return ownedPods[i].CreationTimestamp.After(ownedPods[j].CreationTimestamp)
		})
		for i := int32(0); i < owned-replicas; i++ {
			key := store.KeyFor("", "pods", ownedPods[i].Namespace, ownedPods[i].Name)
			_ = c.store.Delete(key)
		}
		ownedPods = ownedPods[:replicas]
	}

	// Status reflects the observed ready pod count, not the desired replicas.
	var ready int32
	for _, pod := range ownedPods {
		if pod.Status.Phase == k8s.PodRunning {
			ready++
		}
	}
	rs.Status = k8s.ReplicaSetStatus{
		Replicas:          int32(len(ownedPods)),
		ReadyReplicas:     ready,
		AvailableReplicas: ready,
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

// Run starts the Job reconciliation loop. It watches both Jobs and Pods so
// that pod phase transitions (success/failure) drive job reconciliation even
// when the Job object itself is unchanged.
func (c *JobController) Run(ctx context.Context) {
	go watchLoop(ctx, c.store, store.KeyFor("", "pods", "", ""), c.logger, c.handlePodEvent)
	watchLoop(ctx, c.store, store.KeyFor("batch", "jobs", "", ""), c.logger, c.handleJobEvent)
}

func (c *JobController) handleJobEvent(event store.WatchEvent) {
	if event.Object == nil {
		return
	}
	var job k8s.Job
	if err := json.Unmarshal(event.Object.Value, &job); err != nil {
		return
	}
	c.reconcile(&job)
}

func (c *JobController) handlePodEvent(event store.WatchEvent) {
	if event.Object == nil {
		return
	}
	var pod k8s.Pod
	if err := json.Unmarshal(event.Object.Value, &pod); err != nil {
		return
	}
	for _, ref := range pod.OwnerReferences {
		if ref.Kind != "Job" {
			continue
		}
		key := store.KeyFor("batch", "jobs", pod.Namespace, ref.Name)
		obj, err := c.store.Get(key)
		if err != nil {
			continue
		}
		var job k8s.Job
		if err := json.Unmarshal(obj.Value, &job); err != nil {
			continue
		}
		if job.UID != ref.UID {
			continue
		}
		c.reconcile(&job)
	}
}

func (c *JobController) reconcile(job *k8s.Job) {
	var parallelism int32 = 1
	if job.Spec.Parallelism != nil {
		parallelism = *job.Spec.Parallelism
	}
	var completions int32 = 1
	if job.Spec.Completions != nil {
		completions = *job.Spec.Completions
	}

	podList, _ := c.store.List(store.KeyFor("", "pods", job.Namespace, ""))
	var active, succeeded, failed int32
	maxIndex := int32(-1)
	prefix := job.Name + "-"
	for _, obj := range podList {
		var pod k8s.Pod
		if err := json.Unmarshal(obj.Value, &pod); err != nil {
			continue
		}
		owned := false
		for _, ref := range pod.OwnerReferences {
			if ref.UID == job.UID {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		if strings.HasPrefix(pod.Name, prefix) {
			if idx, err := strconv.Atoi(pod.Name[len(prefix):]); err == nil {
				if int32(idx) > maxIndex {
					maxIndex = int32(idx)
				}
			}
		}
		switch pod.Status.Phase {
		case k8s.PodSucceeded:
			succeeded++
		case k8s.PodFailed:
			failed++
		default:
			active++
		}
	}

	// Create new pods up to the parallelism limit (and never beyond what is
	// needed to reach the completions target).
	created := int32(0)
	if succeeded < completions && active < parallelism {
		toCreate := parallelism - active
		if remaining := completions - succeeded - active; remaining > 0 && remaining < toCreate {
			toCreate = remaining
		}
		startIdx := maxIndex + 1
		for i := int32(0); i < toCreate; i++ {
			idx := startIdx + i
			pod := k8s.Pod{
				TypeMeta: k8s.TypeMeta{Kind: "Pod", APIVersion: "v1"},
				ObjectMeta: k8s.ObjectMeta{
					Name:              fmt.Sprintf("%s-%d", job.Name, idx),
					Namespace:         job.Namespace,
					CreationTimestamp: time.Now(),
					OwnerReferences: []k8s.OwnerReference{{
						APIVersion: "batch/v1", Kind: "Job",
						Name: job.Name, UID: job.UID,
					}},
					Labels: job.Spec.Template.Labels,
				},
				Spec: job.Spec.Template.Spec,
			}
			data, _ := json.Marshal(pod)
			key := store.KeyFor("", "pods", pod.Namespace, pod.Name)
			_ = c.store.Put(key, &store.StoredObject{Value: data})
			created++
		}
		active += created
	}

	// Derive new status and only persist when something actually changed so the
	// controller does not spin on its own status updates.
	hasComplete := hasJobCondition(job.Status.Conditions, "Complete")
	hasFailed := hasJobCondition(job.Status.Conditions, "Failed")

	now := time.Now()
	newConditions := job.Status.Conditions
	addedComplete := false
	addedFailed := false
	newCompletionTime := job.Status.CompletionTime
	if succeeded >= completions && !hasComplete {
		newCompletionTime = &now
		newConditions = append(newConditions, k8s.Condition{
			Type:               "Complete",
			Status:             "True",
			LastTransitionTime: now,
			Reason:             "CompletionsReached",
		})
		addedComplete = true
	}
	if job.Spec.BackoffLimit != nil && failed > *job.Spec.BackoffLimit && !hasFailed {
		newConditions = append(newConditions, k8s.Condition{
			Type:               "Failed",
			Status:             "True",
			LastTransitionTime: now,
			Reason:             "BackoffLimitExceeded",
		})
		addedFailed = true
	}

	statusChanged := job.Status.Active != active ||
		job.Status.Succeeded != succeeded ||
		job.Status.Failed != failed ||
		addedComplete || addedFailed || created > 0
	if !statusChanged {
		return
	}

	job.Status = k8s.JobStatus{
		Active:         active,
		Succeeded:      succeeded,
		Failed:         failed,
		StartTime:      job.Status.StartTime,
		CompletionTime: newCompletionTime,
		Conditions:     newConditions,
	}
	if job.Status.StartTime == nil {
		start := now
		job.Status.StartTime = &start
	}
	data, _ := json.Marshal(job)
	key := store.KeyFor("batch", "jobs", job.Namespace, job.Name)
	_ = c.store.Put(key, &store.StoredObject{Value: data})
}

// hasJobCondition reports whether a Job already carries a True condition of t.
func hasJobCondition(conds []k8s.Condition, t string) bool {
	for _, c := range conds {
		if c.Type == t && c.Status == "True" {
			return true
		}
	}
	return false
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

// Run starts the Endpoint reconciliation loop. It watches pods and rebuilds
// the Endpoints object for every service whose selector matches the pod.
func (c *EndpointController) Run(ctx context.Context) {
	watchLoop(ctx, c.store, store.KeyFor("", "pods", "", ""), c.logger, c.handlePodEvent)
}

func (c *EndpointController) handlePodEvent(event store.WatchEvent) {
	if event.Object == nil {
		return
	}
	var pod k8s.Pod
	if err := json.Unmarshal(event.Object.Value, &pod); err != nil {
		return
	}
	svcList, _ := c.store.List(store.KeyFor("", "services", pod.Namespace, ""))
	for _, obj := range svcList {
		var svc k8s.Service
		if err := json.Unmarshal(obj.Value, &svc); err != nil {
			continue
		}
		if !labelsMatch(pod.Labels, svc.Spec.Selector) {
			continue
		}
		c.reconcileEndpoints(&svc)
	}
}

// reconcileEndpoints rebuilds the Endpoints object for a service from all
// currently running and ready pods that match the service selector.
func (c *EndpointController) reconcileEndpoints(svc *k8s.Service) {
	podList, _ := c.store.List(store.KeyFor("", "pods", svc.Namespace, ""))
	var addresses []k8s.EndpointAddress
	for _, obj := range podList {
		var pod k8s.Pod
		if err := json.Unmarshal(obj.Value, &pod); err != nil {
			continue
		}
		if !labelsMatch(pod.Labels, svc.Spec.Selector) {
			continue
		}
		if pod.Status.Phase != k8s.PodRunning || pod.Status.PodIP == "" {
			continue
		}
		if !podReady(&pod) {
			continue
		}
		addresses = append(addresses, k8s.EndpointAddress{
			IP: pod.Status.PodIP,
			TargetRef: &k8s.ObjectReference{
				Kind:      "Pod",
				Namespace: pod.Namespace,
				Name:      pod.Name,
				UID:       pod.UID,
			},
		})
	}

	var ports []k8s.EndpointPort
	for _, p := range svc.Spec.Ports {
		ports = append(ports, k8s.EndpointPort{
			Name:     p.Name,
			Port:     p.Port,
			Protocol: p.Protocol,
		})
	}

	ep := k8s.Endpoints{
		TypeMeta: k8s.TypeMeta{Kind: "Endpoints", APIVersion: "v1"},
		ObjectMeta: k8s.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			OwnerReferences: []k8s.OwnerReference{{
				APIVersion: "v1", Kind: "Service",
				Name: svc.Name, UID: svc.UID,
			}},
		},
		Subsets: []k8s.EndpointSubset{{Addresses: addresses, Ports: ports}},
	}
	data, _ := json.Marshal(ep)
	key := store.KeyFor("", "endpoints", svc.Namespace, svc.Name)
	_ = c.store.Put(key, &store.StoredObject{Value: data})
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
	watchLoop(ctx, c.store, store.KeyFor("", "services", "", ""), c.logger, c.handleEvent)
}

func (c *ServiceController) handleEvent(event store.WatchEvent) {
	if event.Object == nil || event.Type == store.EventDeleted {
		return
	}
	var svc k8s.Service
	if err := json.Unmarshal(event.Object.Value, &svc); err != nil {
		return
	}
	c.reconcile(&svc)
}

func (c *ServiceController) reconcile(svc *k8s.Service) {
	// Allocate a ClusterIP from 10.96.0.0/16 when none has been assigned.
	if svc.Spec.ClusterIP == "" {
		svc.Spec.ClusterIP = allocateClusterIP(svc.Name)
		data, _ := json.Marshal(svc)
		key := store.KeyFor("", "services", svc.Namespace, svc.Name)
		_ = c.store.Put(key, &store.StoredObject{Value: data})
	}

	// Ensure an Endpoints object exists for the service; the EndpointController
	// will populate it with ready pod addresses as pods come online.
	ep := k8s.Endpoints{
		TypeMeta: k8s.TypeMeta{Kind: "Endpoints", APIVersion: "v1"},
		ObjectMeta: k8s.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			OwnerReferences: []k8s.OwnerReference{{
				APIVersion: "v1", Kind: "Service",
				Name: svc.Name, UID: svc.UID,
			}},
		},
	}
	data, _ := json.Marshal(ep)
	key := store.KeyFor("", "endpoints", svc.Namespace, svc.Name)
	_ = c.store.Put(key, &store.StoredObject{Value: data})
}

// allocateClusterIP derives a stable ClusterIP in 10.96.0.0/16 from the service
// name via a 32-bit FNV-1a hash. The last octet is biased away from 0/255.
func allocateClusterIP(name string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	sum := h.Sum32()
	third := (sum >> 8) & 0xFF
	fourth := sum & 0xFF
	if fourth == 0 {
		fourth = 1
	} else if fourth == 255 {
		fourth = 254
	}
	return fmt.Sprintf("10.96.%d.%d", third, fourth)
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
	watchLoop(ctx, c.store, store.KeyFor("", "namespaces", "", ""), c.logger, c.handleEvent)
}

func (c *NamespaceController) handleEvent(event store.WatchEvent) {
	if event.Object == nil {
		return
	}
	var ns k8s.Namespace
	if err := json.Unmarshal(event.Object.Value, &ns); err != nil {
		return
	}
	if event.Type == store.EventDeleted {
		c.deleteNamespaceContents(ns.Name)
		return
	}
	// Mark the namespace Active on create/update; only persist when the phase
	// actually changes to avoid a self-sustaining update loop.
	if ns.Status.Phase != k8s.NamespaceActive {
		ns.Status.Phase = k8s.NamespaceActive
		data, _ := json.Marshal(ns)
		key := store.KeyFor("", "namespaces", "", ns.Name)
		_ = c.store.Put(key, &store.StoredObject{Value: data})
	}
}

// deleteNamespaceContents removes every namespaced resource that lived in the
// given namespace, covering the core and apps/batch workloads.
func (c *NamespaceController) deleteNamespaceContents(namespace string) {
	resources := []struct{ group, resource string }{
		{"", "pods"},
		{"", "services"},
		{"", "endpoints"},
		{"", "configmaps"},
		{"", "secrets"},
		{"", "serviceaccounts"},
		{"", "events"},
		{"", "persistentvolumeclaims"},
		{"apps", "deployments"},
		{"apps", "replicasets"},
		{"apps", "statefulsets"},
		{"apps", "daemonsets"},
		{"batch", "jobs"},
		{"batch", "cronjobs"},
	}
	for _, r := range resources {
		prefix := store.KeyFor(r.group, r.resource, namespace, "")
		objs, _ := c.store.List(prefix)
		for _, obj := range objs {
			var meta metaName
			if err := json.Unmarshal(obj.Value, &meta); err != nil {
				continue
			}
			key := store.KeyFor(r.group, r.resource, namespace, meta.Metadata.Name)
			_ = c.store.Delete(key)
		}
	}
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
	watchLoop(ctx, c.store, prefix, c.logger, c.handleEvent)
}

func (c *ServiceAccountController) handleEvent(event store.WatchEvent) {
	if event.Type != store.EventAdded {
		return
	}
	var ns k8s.Namespace
	if err := json.Unmarshal(event.Object.Value, &ns); err != nil {
		return
	}
	sa := k8s.ServiceAccount{
		TypeMeta:   k8s.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
		ObjectMeta: k8s.ObjectMeta{Name: "default", Namespace: ns.Name},
	}
	data, _ := json.Marshal(sa)
	key := store.KeyFor("", "serviceaccounts", ns.Name, "default")
	_ = c.store.Put(key, &store.StoredObject{Value: data})
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

// kindToResource maps a Kubernetes Kind to its (group, resource) store prefix
// so the garbage collector can reconstruct the key of an owner or child.
var kindToResource = map[string][2]string{
	"Deployment":            {"apps", "deployments"},
	"ReplicaSet":            {"apps", "replicasets"},
	"StatefulSet":           {"apps", "statefulsets"},
	"DaemonSet":             {"apps", "daemonsets"},
	"Job":                   {"batch", "jobs"},
	"CronJob":               {"batch", "cronjobs"},
	"Pod":                   {"", "pods"},
	"Service":               {"", "services"},
	"Endpoints":             {"", "endpoints"},
	"ConfigMap":             {"", "configmaps"},
	"Secret":                {"", "secrets"},
	"ServiceAccount":        {"", "serviceaccounts"},
	"Namespace":             {"", "namespaces"},
	"Node":                  {"", "nodes"},
	"Event":                 {"", "events"},
	"PersistentVolumeClaim": {"", "persistentvolumeclaims"},
	"PersistentVolume":      {"", "persistentvolumes"},
}

func (c *GarbageCollector) collect() {
	// Repeat until no orphans are found in a pass so that a deletion cascade
	// (Deployment -> ReplicaSet -> Pod) completes within a single tick.
	for c.collectOnce() {
	}
}

// collectOnce scans every object in the store and deletes any whose owners have
// all vanished. It returns true when at least one object was deleted.
func (c *GarbageCollector) collectOnce() bool {
	allObjs, _ := c.store.List("")

	type objInfo struct {
		key    string
		uid    string
		owners []k8s.OwnerReference
	}

	existingUIDs := make(map[string]bool)
	infos := make([]objInfo, 0, len(allObjs))
	for _, obj := range allObjs {
		var meta struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name            string               `json:"name"`
				Namespace       string               `json:"namespace"`
				UID             string               `json:"uid"`
				OwnerReferences []k8s.OwnerReference `json:"ownerReferences"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(obj.Value, &meta); err != nil {
			continue
		}
		existingUIDs[meta.Metadata.UID] = true
		gr, ok := kindToResource[meta.Kind]
		if !ok {
			continue
		}
		key := store.KeyFor(gr[0], gr[1], meta.Metadata.Namespace, meta.Metadata.Name)
		infos = append(infos, objInfo{
			key:    key,
			uid:    meta.Metadata.UID,
			owners: meta.Metadata.OwnerReferences,
		})
	}

	deleted := false
	for _, info := range infos {
		if len(info.owners) == 0 {
			continue
		}
		hasLiveOwner := false
		for _, ref := range info.owners {
			if existingUIDs[ref.UID] {
				hasLiveOwner = true
				break
			}
		}
		if !hasLiveOwner {
			if err := c.store.Delete(info.key); err == nil {
				deleted = true
				existingUIDs[info.uid] = false
			}
		}
	}
	return deleted
}
