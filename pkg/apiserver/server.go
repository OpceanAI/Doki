// Package apiserver provides the Kubernetes API server.
package apiserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type APIServer struct {
	store      store.Store
	mux        *http.ServeMux
	server     *http.Server
	clusterIPs map[string]bool
	nextIP     byte
	// criClient, when set, lets the apiserver serve real `kubectl logs` by
	// resolving a pod's container log path via the CRI (K5).
	criClient v1.RuntimeServiceClient
}

func NewAPIServer(addr string, s store.Store) *APIServer {
	return NewAPIServerWithCRI(addr, s, "")
}

// NewAPIServerWithCRI builds an apiserver that can serve real pod logs by
// talking to the CRI runtime at criSocket. If the socket is empty or
// unreachable, log requests return an honest error instead of fake data.
func NewAPIServerWithCRI(addr string, s store.Store, criSocket string) *APIServer {
	api := &APIServer{
		store:      s,
		mux:        http.NewServeMux(),
		clusterIPs: make(map[string]bool),
		nextIP:     1,
	}
	if criSocket != "" {
		if conn, err := grpc.NewClient("unix://"+criSocket,
			grpc.WithTransportCredentials(insecure.NewCredentials())); err == nil {
			api.criClient = v1.NewRuntimeServiceClient(conn)
		}
	}
	api.registerRoutes()
	api.server = &http.Server{
		Addr:         addr,
		Handler:      api.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	api.ensureDefaultNamespace()
	return api
}

func (a *APIServer) Start() error {
	return a.server.ListenAndServe()
}

func (a *APIServer) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}

func (a *APIServer) registerRoutes() {
	a.mux.HandleFunc("/api", a.handleAPIVersions)
	a.mux.HandleFunc("/api/v1", a.handleCoreV1Resources)
	a.mux.HandleFunc("/api/v1/namespaces", a.handleNamespaces)
	a.mux.HandleFunc("/api/v1/namespaces/", a.handleNamespacedResources)
	a.mux.HandleFunc("/api/v1/nodes", a.handleNodes)
	a.mux.HandleFunc("/api/v1/persistentvolumes", a.handlePVs)

	a.mux.HandleFunc("/apis", a.handleAPIGroupList)
	a.mux.HandleFunc("/apis/apps/v1", a.handleAppsV1Resources)
	a.mux.HandleFunc("/apis/apps/v1/namespaces/", a.handleAppsNamespaced)
	a.mux.HandleFunc("/apis/batch/v1", a.handleBatchV1Resources)
	a.mux.HandleFunc("/apis/batch/v1/namespaces/", a.handleBatchNamespaced)
	a.mux.HandleFunc("/apis/networking.k8s.io/v1/namespaces/", a.handleNetworkingNamespaced)
	a.mux.HandleFunc("/apis/rbac.authorization.k8s.io/v1/", a.handleRBAC)

	a.mux.HandleFunc("/healthz", a.handleHealthz)
	a.mux.HandleFunc("/readyz", a.handleReadyz)
	a.mux.HandleFunc("/livez", a.handleLivez)
	a.mux.HandleFunc("/version", a.handleVersion)
	a.mux.HandleFunc("/openapi/v2", a.handleOpenAPI)
}

func (a *APIServer) ensureDefaultNamespace() {
	key := store.KeyFor("", "namespaces", "", "default")
	if _, err := a.store.Get(key); err != nil {
		ns := k8s.Namespace{
			TypeMeta:   k8s.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
			ObjectMeta: k8s.ObjectMeta{Name: "default"},
			Status:     k8s.NamespaceStatus{Phase: k8s.NamespaceActive},
		}
		data, _ := json.Marshal(ns)
		_ = a.store.Put(key, &store.StoredObject{Value: data})
	}

	key = store.KeyFor("", "namespaces", "", "kube-system")
	if _, err := a.store.Get(key); err != nil {
		ns := k8s.Namespace{
			TypeMeta:   k8s.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
			ObjectMeta: k8s.ObjectMeta{Name: "kube-system"},
			Status:     k8s.NamespaceStatus{Phase: k8s.NamespaceActive},
		}
		data, _ := json.Marshal(ns)
		_ = a.store.Put(key, &store.StoredObject{Value: data})
	}
}

func (a *APIServer) handleAPIVersions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":     "APIVersions",
		"versions": []string{"v1"},
	})
}

func (a *APIServer) handleCoreV1Resources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":         "APIResourceList",
		"groupVersion": "v1",
		"resources": []map[string]interface{}{
			{"name": "pods", "namespaced": true, "kind": "Pod", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "services", "namespaced": true, "kind": "Service", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "configmaps", "namespaced": true, "kind": "ConfigMap", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "secrets", "namespaced": true, "kind": "Secret", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "namespaces", "namespaced": false, "kind": "Namespace", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "nodes", "namespaced": false, "kind": "Node", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "persistentvolumes", "namespaced": false, "kind": "PersistentVolume", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "persistentvolumeclaims", "namespaced": true, "kind": "PersistentVolumeClaim", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "serviceaccounts", "namespaced": true, "kind": "ServiceAccount", "verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
			{"name": "events", "namespaced": true, "kind": "Event", "verbs": []string{"create", "get", "list", "watch"}},
		},
	})
}

func (a *APIServer) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/namespaces" {
		a.handleNamespacedResources(w, r)
		return
	}
	a.handleResourceList(w, r, "", "namespaces", "")
}

func (a *APIServer) handleNamespacedResources(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/"), "/")
	if len(parts) < 1 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ns := parts[0]

	if len(parts) == 1 {
		a.handleResource(w, r, "", "namespaces", "", ns)
		return
	}

	resource := parts[1]

	if len(parts) == 2 {
		// POST to a collection creates the resource (name comes from the body's
		// metadata.name), matching real Kubernetes. GET lists; watch streams.
		if r.Method == http.MethodPost {
			a.handleResource(w, r, "", resource, ns, "")
			return
		}
		if isWatch(r) {
			a.handleWatch(w, r, "", resource, ns, "")
			return
		}
		a.handleResourceList(w, r, "", resource, ns)
		return
	}

	name := parts[2]

	if len(parts) >= 4 && parts[3] == "status" {
		a.handleResourceStatus(w, r, "", resource, ns, name)
		return
	}
	if len(parts) >= 4 && parts[3] == "scale" {
		a.handleResourceScale(w, r, "", resource, ns, name)
		return
	}
	if len(parts) >= 4 && parts[3] == "log" || len(parts) >= 4 && parts[3] == "logs" {
		a.handlePodLogs(w, r, ns, name)
		return
	}
	if len(parts) >= 4 && (parts[3] == "exec" || parts[3] == "attach") {
		a.handlePodExec(w, r, ns, name)
		return
	}
	if len(parts) >= 4 && parts[3] == "portforward" {
		http.Error(w, "portforward is not supported by k4s", http.StatusNotImplemented)
		return
	}

	a.handleResource(w, r, "", resource, ns, name)
}

func (a *APIServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	a.handleResourceList(w, r, "", "nodes", "")
}

func (a *APIServer) handlePVs(w http.ResponseWriter, r *http.Request) {
	a.handleResourceList(w, r, "", "persistentvolumes", "")
}

func (a *APIServer) handleAPIGroupList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":       "APIGroupList",
		"apiVersion": "v1",
		"groups": []map[string]interface{}{
			{
				"name": "apps",
				"versions": []map[string]string{
					{"groupVersion": "apps/v1", "version": "v1"},
				},
				"preferredVersion": map[string]string{"groupVersion": "apps/v1", "version": "v1"},
			},
			{
				"name": "batch",
				"versions": []map[string]string{
					{"groupVersion": "batch/v1", "version": "v1"},
				},
				"preferredVersion": map[string]string{"groupVersion": "batch/v1", "version": "v1"},
			},
			{
				"name": "networking.k8s.io",
				"versions": []map[string]string{
					{"groupVersion": "networking.k8s.io/v1", "version": "v1"},
				},
				"preferredVersion": map[string]string{"groupVersion": "networking.k8s.io/v1", "version": "v1"},
			},
			{
				"name": "rbac.authorization.k8s.io",
				"versions": []map[string]string{
					{"groupVersion": "rbac.authorization.k8s.io/v1", "version": "v1"},
				},
				"preferredVersion": map[string]string{"groupVersion": "rbac.authorization.k8s.io/v1", "version": "v1"},
			},
		},
	})
}

func (a *APIServer) handleAppsV1Resources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":         "APIResourceList",
		"groupVersion": "apps/v1",
		"resources": []map[string]interface{}{
			{"name": "deployments", "namespaced": true, "kind": "Deployment"},
			{"name": "replicasets", "namespaced": true, "kind": "ReplicaSet"},
			{"name": "statefulsets", "namespaced": true, "kind": "StatefulSet"},
			{"name": "daemonsets", "namespaced": true, "kind": "DaemonSet"},
		},
	})
}

func (a *APIServer) handleAppsNamespaced(w http.ResponseWriter, r *http.Request) {
	a.handleGroupNamespaced(w, r, "apps")
}

func (a *APIServer) handleBatchV1Resources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":         "APIResourceList",
		"groupVersion": "batch/v1",
		"resources": []map[string]interface{}{
			{"name": "jobs", "namespaced": true, "kind": "Job"},
			{"name": "cronjobs", "namespaced": true, "kind": "CronJob"},
		},
	})
}

func (a *APIServer) handleBatchNamespaced(w http.ResponseWriter, r *http.Request) {
	a.handleGroupNamespaced(w, r, "batch")
}

func (a *APIServer) handleNetworkingNamespaced(w http.ResponseWriter, r *http.Request) {
	a.handleGroupNamespaced(w, r, "networking.k8s.io")
}

func (a *APIServer) handleRBAC(w http.ResponseWriter, r *http.Request) {
	a.handleGroupNamespaced(w, r, "rbac.authorization.k8s.io")
}

func (a *APIServer) handleGroupNamespaced(w http.ResponseWriter, r *http.Request, group string) {
	prefix := fmt.Sprintf("/apis/%s/v1/namespaces/", group)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) < 1 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ns := parts[0]
	if len(parts) == 1 {
		http.Error(w, "resource required", http.StatusBadRequest)
		return
	}

	resource := parts[1]

	if len(parts) == 2 {
		if isWatch(r) {
			a.handleWatch(w, r, group, resource, ns, "")
			return
		}
		a.handleResourceList(w, r, group, resource, ns)
		return
	}

	name := parts[2]
	if len(parts) >= 4 && parts[3] == "status" {
		a.handleResourceStatus(w, r, group, resource, ns, name)
		return
	}
	if len(parts) >= 4 && parts[3] == "scale" {
		a.handleResourceScale(w, r, group, resource, ns, name)
		return
	}

	a.handleResource(w, r, group, resource, ns, name)
}

func (a *APIServer) handleResourceList(w http.ResponseWriter, r *http.Request, group, resource, namespace string) {
	key := store.KeyFor(group, resource, namespace, "")
	prefix := key

	objects, err := a.store.List(prefix)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// K3: apply label/field selectors server-side so `kubectl get -l app=x`
	// and informers no longer receive the entire collection.
	labelSel := parseSelector(r.URL.Query().Get("labelSelector"))
	fieldSel := parseSelector(r.URL.Query().Get("fieldSelector"))
	items := make([]json.RawMessage, 0, len(objects))
	for _, obj := range objects {
		if !matchesSelectors(obj.Value, labelSel, fieldSel) {
			continue
		}
		items = append(items, obj.Value)
	}

	kind := resourceToKind(resource)
	listKind := kind + "List"

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":       listKind,
		"apiVersion": groupVersion(group),
		"metadata":   map[string]string{"resourceVersion": fmt.Sprintf("%d", a.store.CurrentRevision())},
		"items":      items,
	})
}

func (a *APIServer) handleResource(w http.ResponseWriter, r *http.Request, group, resource, namespace, name string) {
	key := store.KeyFor(group, resource, namespace, name)

	switch r.Method {
	case http.MethodGet:
		obj, err := a.store.Get(key)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s %q not found", resource, name), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(obj.Value)

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var objMap map[string]interface{}
		if err := json.Unmarshal(body, &objMap); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		meta, _ := objMap["metadata"].(map[string]interface{})
		if meta == nil {
			meta = make(map[string]interface{})
		}
		if _, ok := meta["uid"]; !ok {
			meta["uid"] = generateUUID()
		}
		if _, ok := meta["resourceVersion"]; !ok {
			meta["resourceVersion"] = "1"
		}
		if _, ok := meta["creationTimestamp"]; !ok {
			meta["creationTimestamp"] = time.Now().UTC().Format(time.RFC3339)
		}
		objMap["metadata"] = meta
		// For a POST to a collection the name is in the body, not the path;
		// derive it and recompute the storage key.
		if name == "" {
			if n, ok := meta["name"].(string); ok {
				name = n
			}
			if name == "" {
				http.Error(w, "metadata.name is required", http.StatusBadRequest)
				return
			}
			key = store.KeyFor(group, resource, namespace, name)
		}
		stored, err := json.Marshal(objMap)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		obj := &store.StoredObject{Value: stored}
		if err := a.store.Put(key, obj); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(stored)

	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		obj := &store.StoredObject{Value: body}
		if err := a.store.Put(key, obj); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)

	case http.MethodPatch:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, err := a.store.Get(key)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		ct := r.Header.Get("Content-Type")
		switch ct {
		case "application/merge-patch+json", "application/strategic-merge-patch+json":
			merged, merr := mergePatch(existing.Value, body)
			if merr != nil {
				http.Error(w, merr.Error(), http.StatusBadRequest)
				return
			}
			if err := a.store.Put(key, &store.StoredObject{Value: merged}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(merged)
		case "application/json-patch+json":
			patched, perr := applyJSONPatch(existing.Value, body)
			if perr != nil {
				http.Error(w, perr.Error(), http.StatusBadRequest)
				return
			}
			if err := a.store.Put(key, &store.StoredObject{Value: patched}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(patched)
		default:
			http.Error(w, "unsupported patch content type: "+ct, http.StatusUnsupportedMediaType)
		}

	case http.MethodDelete:
		// K11: return the deleted object (Kubernetes returns the object, not a
		// bare Status) so clients that read the response body work correctly.
		existing, getErr := a.store.Get(key)
		if err := a.store.Delete(key); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if getErr == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(existing.Value)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "Success"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleResourceStatus implements the status subresource. GET returns the
// object; PUT/PATCH updates ONLY the status stanza of the stored object (K2) so
// controller status writes are no longer silently dropped.
func (a *APIServer) handleResourceStatus(w http.ResponseWriter, r *http.Request, group, resource, namespace, name string) {
	key := store.KeyFor(group, resource, namespace, name)
	obj, err := a.store.Get(key)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(obj.Value)
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var cur map[string]interface{}
	if err := json.Unmarshal(obj.Value, &cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var incoming map[string]interface{}
	if err := json.Unmarshal(body, &incoming); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	if st, ok := incoming["status"]; ok {
		cur["status"] = st
	}
	merged, _ := json.Marshal(cur)
	if err := a.store.Put(key, &store.StoredObject{Value: merged}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(merged)
}

// handleResourceScale implements the scale subresource. GET returns a Scale
// object; PUT/PATCH updates spec.replicas on the stored object (K1) so
// `kubectl scale` actually changes the replica count instead of no-oping.
func (a *APIServer) handleResourceScale(w http.ResponseWriter, r *http.Request, group, resource, namespace, name string) {
	key := store.KeyFor(group, resource, namespace, name)
	obj, err := a.store.Get(key)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var cur map[string]interface{}
	if err := json.Unmarshal(obj.Value, &cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	curReplicas := func() int64 {
		if spec, ok := cur["spec"].(map[string]interface{}); ok {
			if rep, ok := spec["replicas"].(float64); ok {
				return int64(rep)
			}
		}
		return 0
	}
	scaleObj := func(replicas int64) map[string]interface{} {
		return map[string]interface{}{
			"kind":       "Scale",
			"apiVersion": "autoscaling/v1",
			"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
			"spec":       map[string]interface{}{"replicas": replicas},
			"status":     map[string]interface{}{"replicas": replicas},
		}
	}

	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, scaleObj(curReplicas()))
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Extract the desired replica count from a Scale (PUT) or a patch body.
	var want map[string]interface{}
	if err := json.Unmarshal(body, &want); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	newReplicas := curReplicas()
	if spec, ok := want["spec"].(map[string]interface{}); ok {
		if rep, ok := spec["replicas"].(float64); ok {
			newReplicas = int64(rep)
		}
	}
	spec, _ := cur["spec"].(map[string]interface{})
	if spec == nil {
		spec = map[string]interface{}{}
	}
	spec["replicas"] = newReplicas
	cur["spec"] = spec
	merged, _ := json.Marshal(cur)
	if err := a.store.Put(key, &store.StoredObject{Value: merged}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scaleObj(newReplicas))
}

// handlePodLogs serves real container logs for a pod (K5). It reads the pod
// object, resolves the target container's log path via the CRI, and streams the
// log file (honoring ?tailLines and ?follow). When no CRI runtime is wired it
// returns an honest error rather than a fabricated log line.
func (a *APIServer) handlePodLogs(w http.ResponseWriter, r *http.Request, namespace, name string) {
	if a.criClient == nil {
		http.Error(w, "pod logs unavailable: apiserver has no CRI runtime configured", http.StatusServiceUnavailable)
		return
	}
	key := store.KeyFor("", "pods", namespace, name)
	obj, err := a.store.Get(key)
	if err != nil {
		http.Error(w, "pod not found", http.StatusNotFound)
		return
	}
	var pod struct {
		Status struct {
			ContainerStatuses []struct {
				Name        string `json:"name"`
				ContainerID string `json:"containerID"`
			} `json:"containerStatuses"`
		} `json:"status"`
	}
	if err := json.Unmarshal(obj.Value, &pod); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wantContainer := r.URL.Query().Get("container")
	var containerID string
	for _, cs := range pod.Status.ContainerStatuses {
		if wantContainer == "" || cs.Name == wantContainer {
			containerID = cs.ContainerID
			break
		}
	}
	// containerID may be prefixed with a scheme like "cri://"; strip it.
	if i := strings.Index(containerID, "://"); i != -1 {
		containerID = containerID[i+3:]
	}
	if containerID == "" {
		http.Error(w, "no running container found for pod", http.StatusNotFound)
		return
	}
	status, err := a.criClient.ContainerStatus(r.Context(), &v1.ContainerStatusRequest{ContainerId: containerID})
	if err != nil {
		http.Error(w, "container status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	logPath := status.GetStatus().GetLogPath()
	if logPath == "" {
		http.Error(w, "container has no log path", http.StatusNotFound)
		return
	}
	f, err := os.Open(logPath)
	if err != nil {
		http.Error(w, "open log: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()

	w.Header().Set("Content-Type", "text/plain")
	if tail := r.URL.Query().Get("tailLines"); tail != "" {
		if n, perr := strconv.Atoi(tail); perr == nil && n >= 0 {
			seekToLastLines(f, n)
		}
	}
	_, _ = io.Copy(w, f)

	if r.URL.Query().Get("follow") == "true" {
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			n, rerr := f.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if rerr != nil {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(200 * time.Millisecond):
				}
			}
		}
	}
}

// handlePodExec (K7) implements the pod exec/attach subresource by resolving
// the pod's container, asking the CRI for a streaming URL, and reverse-proxying
// the client's connection (including the WebSocket upgrade) to that URL. The
// CRI streaming server speaks the same v4/v5.channel.k8s.io protocol kubectl
// uses, so a transparent byte proxy is sufficient.
func (a *APIServer) handlePodExec(w http.ResponseWriter, r *http.Request, namespace, name string) {
	if a.criClient == nil {
		http.Error(w, "exec unavailable: apiserver has no CRI runtime configured", http.StatusServiceUnavailable)
		return
	}
	key := store.KeyFor("", "pods", namespace, name)
	obj, err := a.store.Get(key)
	if err != nil {
		http.Error(w, "pod not found", http.StatusNotFound)
		return
	}
	var pod struct {
		Status struct {
			ContainerStatuses []struct {
				Name        string `json:"name"`
				ContainerID string `json:"containerID"`
			} `json:"containerStatuses"`
		} `json:"status"`
	}
	if err := json.Unmarshal(obj.Value, &pod); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	wantContainer := q.Get("container")
	var containerID string
	for _, cs := range pod.Status.ContainerStatuses {
		if wantContainer == "" || cs.Name == wantContainer {
			containerID = cs.ContainerID
			break
		}
	}
	if i := strings.Index(containerID, "://"); i != -1 {
		containerID = containerID[i+3:]
	}
	if containerID == "" {
		http.Error(w, "no running container found for pod", http.StatusNotFound)
		return
	}
	cmd := q["command"]
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}
	tty := q.Get("tty") == "true" || q.Get("tty") == "1"
	stdin := q.Get("stdin") == "true" || q.Get("stdin") == "1"
	resp, err := a.criClient.Exec(r.Context(), &v1.ExecRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Tty:         tty,
		Stdin:       stdin,
		Stdout:      true,
		Stderr:      !tty,
	})
	if err != nil {
		http.Error(w, "cri exec: "+err.Error(), http.StatusInternalServerError)
		return
	}
	proxyUpgrade(w, r, resp.GetUrl())
}

// proxyUpgrade reverse-proxies an HTTP request (including a hijacked
// Upgrade/WebSocket connection) to targetURL, copying bytes in both directions.
func proxyUpgrade(w http.ResponseWriter, r *http.Request, targetURL string) {
	u, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "bad streaming url: "+err.Error(), http.StatusInternalServerError)
		return
	}
	backend, err := net.Dial("tcp", u.Host)
	if err != nil {
		http.Error(w, "dial streaming backend: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = backend.Close() }()

	// Rewrite the request line to the backend path and forward headers verbatim.
	reqLine := fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, u.RequestURI())
	if _, err := backend.Write([]byte(reqLine)); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = r.Header.Write(backend)
	_, _ = backend.Write([]byte("\r\n"))

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	client, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer func() { _ = client.Close() }()

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(backend, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, backend); done <- struct{}{} }()
	<-done
}

// seekToLastLines positions f so that a subsequent read yields at most the last
// n lines. Best-effort: on any error it seeks to start.
func seekToLastLines(f *os.File, n int) {
	if n == 0 {
		_, _ = f.Seek(0, io.SeekEnd)
		return
	}
	stat, err := f.Stat()
	if err != nil {
		return
	}
	size := stat.Size()
	const chunk = 8192
	var pos = size
	lines := 0
	buf := make([]byte, chunk)
	for pos > 0 {
		readSize := int64(chunk)
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize
		if _, err := f.ReadAt(buf[:readSize], pos); err != nil {
			_, _ = f.Seek(0, io.SeekStart)
			return
		}
		for i := readSize - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				lines++
				if lines > n {
					_, _ = f.Seek(pos+i+1, io.SeekStart)
					return
				}
			}
		}
	}
	_, _ = f.Seek(0, io.SeekStart)
}

// parseSelector parses a comma-separated "k1=v1,k2=v2" selector string into a
// map. Only equality selectors are supported (k4s is deliberately lighter than
// full Kubernetes set-based selectors).
func parseSelector(s string) map[string]string {
	if s == "" {
		return nil
	}
	out := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return out
}

// matchesSelectors reports whether an object satisfies the given label and
// field selectors. Supported fields: metadata.name, metadata.namespace,
// status.phase (the common kubectl field selectors).
func matchesSelectors(raw json.RawMessage, labelSel, fieldSel map[string]string) bool {
	if len(labelSel) == 0 && len(fieldSel) == 0 {
		return true
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return false
	}
	meta, _ := obj["metadata"].(map[string]interface{})
	if len(labelSel) > 0 {
		labels, _ := meta["labels"].(map[string]interface{})
		for k, v := range labelSel {
			lv, ok := labels[k].(string)
			if !ok || lv != v {
				return false
			}
		}
	}
	for k, v := range fieldSel {
		var got string
		switch k {
		case "metadata.name":
			got, _ = meta["name"].(string)
		case "metadata.namespace":
			got, _ = meta["namespace"].(string)
		case "status.phase":
			if status, ok := obj["status"].(map[string]interface{}); ok {
				got, _ = status["phase"].(string)
			}
		default:
			// Unknown field selector: don't match (conservative).
			return false
		}
		if got != v {
			return false
		}
	}
	return true
}

func (a *APIServer) handleWatch(w http.ResponseWriter, r *http.Request, group, resource, namespace, _ string) {
	prefix := store.KeyFor(group, resource, namespace, "")
	labelSel := parseSelector(r.URL.Query().Get("labelSelector"))
	fieldSel := parseSelector(r.URL.Query().Get("fieldSelector"))
	sinceRev := int64(0)
	if rv := r.URL.Query().Get("resourceVersion"); rv != "" {
		parsed, err := strconv.ParseInt(rv, 10, 64)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid resourceVersion %q: %v", rv, err), http.StatusBadRequest)
			return
		}
		sinceRev = parsed
	}

	ch, err := a.store.Watch(prefix, sinceRev)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, private")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			a.store.Unwatch(ch)
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if !matchesSelectors(event.Object.Value, labelSel, fieldSel) {
				continue
			}
			encoded, err := json.Marshal(struct {
				Type   string          `json:"type"`
				Object json.RawMessage `json:"object"`
			}{
				Type:   event.Type,
				Object: event.Object.Value,
			})
			if err != nil {
				continue
			}
			_, _ = w.Write(encoded)
			_, _ = w.Write([]byte("\n"))
			flusher.Flush()
		}
	}
}

func (a *APIServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

func (a *APIServer) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

func (a *APIServer) handleLivez(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

func (a *APIServer) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"major":      "0",
		"minor":      "10",
		"gitVersion": common.DokiVersion,
		"gitCommit":  common.GitCommit,
		"buildDate":  common.BuildDate,
		"platform":   runtime.GOOS + "/" + runtime.GOARCH,
		"goVersion":  runtime.Version(),
		"compiler":   runtime.Compiler,
	})
}

func (a *APIServer) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]string{
			"title":   "Doki Kubernetes API",
			"version": "v" + common.DokiVersion,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func isWatch(r *http.Request) bool {
	return r.URL.Query().Get("watch") == "true"
}

func resourceToKind(resource string) string {
	singular := map[string]string{
		"pods": "Pod", "services": "Service", "configmaps": "ConfigMap",
		"secrets": "Secret", "namespaces": "Namespace", "nodes": "Node",
		"persistentvolumes": "PersistentVolume", "persistentvolumeclaims": "PersistentVolumeClaim",
		"serviceaccounts": "ServiceAccount", "events": "Event",
		"deployments": "Deployment", "replicasets": "ReplicaSet",
		"statefulsets": "StatefulSet", "daemonsets": "DaemonSet",
		"jobs": "Job", "cronjobs": "CronJob",
		"ingresses": "Ingress", "networkpolicies": "NetworkPolicy",
		"roles": "Role", "rolebindings": "RoleBinding",
		"clusterroles": "ClusterRole", "clusterrolebindings": "ClusterRoleBinding",
	}
	if kind, ok := singular[resource]; ok {
		return kind
	}
	return resource
}

func groupVersion(group string) string {
	if group == "" {
		return "v1"
	}
	return group + "/v1"
}

func mergePatch(existing, patch json.RawMessage) (json.RawMessage, error) {
	var base, delta map[string]interface{}
	if err := json.Unmarshal(existing, &base); err != nil {
		return nil, fmt.Errorf("invalid existing object: %w", err)
	}
	if err := json.Unmarshal(patch, &delta); err != nil {
		return nil, fmt.Errorf("invalid patch: %w", err)
	}
	deepMerge(base, delta)
	return json.Marshal(base)
}

// applyJSONPatch applies an RFC 6902 JSON Patch (K6). Supports add, remove,
// replace, copy, move and test with JSON Pointer paths — enough for
// `kubectl patch --type=json`.
func applyJSONPatch(existing, patch json.RawMessage) (json.RawMessage, error) {
	var doc interface{}
	if err := json.Unmarshal(existing, &doc); err != nil {
		return nil, fmt.Errorf("invalid existing object: %w", err)
	}
	var ops []struct {
		Op    string          `json:"op"`
		Path  string          `json:"path"`
		From  string          `json:"from"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(patch, &ops); err != nil {
		return nil, fmt.Errorf("invalid json-patch: %w", err)
	}
	for _, op := range ops {
		var val interface{}
		if len(op.Value) > 0 {
			if err := json.Unmarshal(op.Value, &val); err != nil {
				return nil, fmt.Errorf("invalid value in op %s: %w", op.Op, err)
			}
		}
		switch op.Op {
		case "add", "replace":
			var err error
			doc, err = jsonPointerSet(doc, op.Path, val)
			if err != nil {
				return nil, err
			}
		case "remove":
			var err error
			doc, err = jsonPointerRemove(doc, op.Path)
			if err != nil {
				return nil, err
			}
		case "copy", "move":
			from, err := jsonPointerGet(doc, op.From)
			if err != nil {
				return nil, err
			}
			if op.Op == "move" {
				if doc, err = jsonPointerRemove(doc, op.From); err != nil {
					return nil, err
				}
			}
			if doc, err = jsonPointerSet(doc, op.Path, from); err != nil {
				return nil, err
			}
		case "test":
			got, err := jsonPointerGet(doc, op.Path)
			if err != nil {
				return nil, err
			}
			gb, _ := json.Marshal(got)
			vb, _ := json.Marshal(val)
			if string(gb) != string(vb) {
				return nil, fmt.Errorf("json-patch test failed at %s", op.Path)
			}
		default:
			return nil, fmt.Errorf("unsupported json-patch op %q", op.Op)
		}
	}
	return json.Marshal(doc)
}

func jsonPointerTokens(path string) []string {
	if path == "" || path == "/" {
		return nil
	}
	raw := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, t := range raw {
		t = strings.ReplaceAll(t, "~1", "/")
		raw[i] = strings.ReplaceAll(t, "~0", "~")
	}
	return raw
}

func jsonPointerGet(doc interface{}, path string) (interface{}, error) {
	cur := doc
	for _, tok := range jsonPointerTokens(path) {
		switch node := cur.(type) {
		case map[string]interface{}:
			v, ok := node[tok]
			if !ok {
				return nil, fmt.Errorf("json-pointer %s: key %q not found", path, tok)
			}
			cur = v
		case []interface{}:
			idx, err := strconv.Atoi(tok)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, fmt.Errorf("json-pointer %s: bad index %q", path, tok)
			}
			cur = node[idx]
		default:
			return nil, fmt.Errorf("json-pointer %s: cannot descend", path)
		}
	}
	return cur, nil
}

func jsonPointerSet(doc interface{}, path string, val interface{}) (interface{}, error) {
	tokens := jsonPointerTokens(path)
	if len(tokens) == 0 {
		return val, nil
	}
	if len(tokens) == 1 {
		return setChild(doc, tokens[0], val)
	}
	parent, err := jsonPointerGet(doc, "/"+strings.Join(tokens[:len(tokens)-1], "/"))
	if err != nil {
		return nil, err
	}
	if _, err := setChild(parent, tokens[len(tokens)-1], val); err != nil {
		return nil, err
	}
	return doc, nil
}

func setChild(node interface{}, tok string, val interface{}) (interface{}, error) {
	switch n := node.(type) {
	case map[string]interface{}:
		n[tok] = val
		return n, nil
	case []interface{}:
		if tok == "-" {
			return append(n, val), nil
		}
		idx, err := strconv.Atoi(tok)
		if err != nil || idx < 0 || idx > len(n) {
			return nil, fmt.Errorf("bad array index %q", tok)
		}
		if idx == len(n) {
			return append(n, val), nil
		}
		n[idx] = val
		return n, nil
	default:
		return nil, fmt.Errorf("cannot set child on non-container")
	}
}

func jsonPointerRemove(doc interface{}, path string) (interface{}, error) {
	tokens := jsonPointerTokens(path)
	if len(tokens) == 0 {
		return doc, nil
	}
	if len(tokens) == 1 {
		switch n := doc.(type) {
		case map[string]interface{}:
			delete(n, tokens[0])
			return n, nil
		case []interface{}:
			idx, err := strconv.Atoi(tokens[0])
			if err != nil || idx < 0 || idx >= len(n) {
				return nil, fmt.Errorf("bad array index %q", tokens[0])
			}
			return append(n[:idx], n[idx+1:]...), nil
		}
		return doc, nil
	}
	parent, err := jsonPointerGet(doc, "/"+strings.Join(tokens[:len(tokens)-1], "/"))
	if err != nil {
		return nil, err
	}
	last := tokens[len(tokens)-1]
	switch n := parent.(type) {
	case map[string]interface{}:
		delete(n, last)
	case []interface{}:
		idx, err := strconv.Atoi(last)
		if err != nil || idx < 0 || idx >= len(n) {
			return nil, fmt.Errorf("bad array index %q", last)
		}
		// Note: removing from a nested slice requires reassigning the parent;
		// callers that need this should target the slice directly.
		copy(n[idx:], n[idx+1:])
		n[len(n)-1] = nil
	}
	return doc, nil
}

func deepMerge(dst, src map[string]interface{}) {
	for k, sv := range src {
		if sv == nil {
			delete(dst, k)
			continue
		}
		dv, ok := dst[k]
		if !ok {
			dst[k] = sv
			continue
		}
		if dvMap, dOK := dv.(map[string]interface{}); dOK {
			if svMap, sOK := sv.(map[string]interface{}); sOK {
				deepMerge(dvMap, svMap)
				continue
			}
		}
		dst[k] = sv
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
