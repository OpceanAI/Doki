package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

type APIServer struct {
	store      store.Store
	mux        *http.ServeMux
	server     *http.Server
	clusterIPs map[string]bool
	nextIP     byte
}

func NewAPIServer(addr string, s store.Store) *APIServer {
	api := &APIServer{
		store:      s,
		mux:        http.NewServeMux(),
		clusterIPs: make(map[string]bool),
		nextIP:     1,
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
	a.mux.HandleFunc("/apis/networking/v1/namespaces/", a.handleNetworkingNamespaced)
	a.mux.HandleFunc("/apis/rbac/v1/", a.handleRBAC)

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

func (a *APIServer) handleAPIVersions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":     "APIVersions",
		"versions": []string{"v1"},
	})
}

func (a *APIServer) handleCoreV1Resources(w http.ResponseWriter, r *http.Request) {
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

	a.handleResource(w, r, "", resource, ns, name)
}

func (a *APIServer) handleNodes(w http.ResponseWriter, r *http.Request) {
	a.handleResourceList(w, r, "", "nodes", "")
}

func (a *APIServer) handlePVs(w http.ResponseWriter, r *http.Request) {
	a.handleResourceList(w, r, "", "persistentvolumes", "")
}

func (a *APIServer) handleAPIGroupList(w http.ResponseWriter, r *http.Request) {
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
					{"groupVersion": "networking/v1", "version": "v1"},
				},
				"preferredVersion": map[string]string{"groupVersion": "networking/v1", "version": "v1"},
			},
			{
				"name": "rbac.authorization.k8s.io",
				"versions": []map[string]string{
					{"groupVersion": "rbac/v1", "version": "v1"},
				},
				"preferredVersion": map[string]string{"groupVersion": "rbac/v1", "version": "v1"},
			},
		},
	})
}

func (a *APIServer) handleAppsV1Resources(w http.ResponseWriter, r *http.Request) {
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

func (a *APIServer) handleBatchV1Resources(w http.ResponseWriter, r *http.Request) {
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
	a.handleGroupNamespaced(w, r, "networking")
}

func (a *APIServer) handleRBAC(w http.ResponseWriter, r *http.Request) {
	a.handleGroupNamespaced(w, r, "rbac")
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

	items := make([]json.RawMessage, 0, len(objects))
	for _, obj := range objects {
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
		obj := &store.StoredObject{Value: body}
		if err := a.store.Put(key, obj); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)

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
		_ = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(existing.Value)

	case http.MethodDelete:
		if err := a.store.Delete(key); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "Success"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *APIServer) handleResourceStatus(w http.ResponseWriter, r *http.Request, group, resource, namespace, name string) {
	key := store.KeyFor(group, resource, namespace, name)
	obj, err := a.store.Get(key)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(obj.Value)
}

func (a *APIServer) handleResourceScale(w http.ResponseWriter, r *http.Request, group, resource, namespace, name string) {
	key := store.KeyFor(group, resource, namespace, name)
	obj, err := a.store.Get(key)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(obj.Value)
}

func (a *APIServer) handlePodLogs(w http.ResponseWriter, r *http.Request, namespace, name string) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = fmt.Fprintf(w, "logs for pod %s/%s\n", namespace, name)
}

func (a *APIServer) handleWatch(w http.ResponseWriter, r *http.Request, group, resource, namespace, name string) {
	prefix := store.KeyFor(group, resource, namespace, "")
	sinceRev := int64(0)
	if rv := r.URL.Query().Get("resourceVersion"); rv != "" {
		_, _ = fmt.Sscanf(rv, "%d", &sinceRev)
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
			_ = json.NewEncoder(w).Encode(event)
			flusher.Flush()
		}
	}
}

func (a *APIServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

func (a *APIServer) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

func (a *APIServer) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "ok")
}

func (a *APIServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"major":        "0",
		"minor":        "10",
		"gitVersion":   "v0.10.0",
		"platform":     "linux/arm64",
		"goVersion":    "go1.26",
		"compiler":     "gc",
	})
}

func (a *APIServer) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]string{
			"title":   "Doki Kubernetes API",
			"version": "v0.10.0",
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
