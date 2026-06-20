// Package podman provides the Podman-compatible API server.
package podman

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

type PodmanServer struct {
	podMgr     *PodManager
	secretMgr  *SecretManager
	manifestMgr *ManifestManager
}

func NewPodmanServer(root string) *PodmanServer {
	return &PodmanServer{
		podMgr:      NewPodManager(root),
		secretMgr:   NewSecretManager(root),
		manifestMgr: NewManifestManager(root),
	}
}

func (s *PodmanServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/libpod/", s.logMiddleware(http.HandlerFunc(s.route)).ServeHTTP)
}

func (s *PodmanServer) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("podman %s %s -> %d %s", r.Method, r.URL.Path, rw.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *PodmanServer) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/libpod/containers/json":
		s.handleContainersList(w, r)
	case path == "/libpod/containers/create":
		s.handleContainersCreate(w, r)
	case path == "/libpod/containers/prune":
		s.handleContainersPrune(w, r)
	case strings.HasPrefix(path, "/libpod/containers/"):
		s.handleContainersDispatch(w, r)

	case path == "/libpod/pods/create":
		s.handlePodsCreate(w, r)
	case path == "/libpod/pods/json":
		s.handlePodsList(w, r)
	case path == "/libpod/pods/prune":
		s.handlePodsPrune(w, r)
	case strings.HasPrefix(path, "/libpod/pods/"):
		s.handlePodsDispatch(w, r)

	case path == "/libpod/images/json":
		s.handleImagesList(w, r)
	case path == "/libpod/images/pull":
		s.handleImagesPull(w, r)
	case path == "/libpod/images/prune":
		s.handleImagesPrune(w, r)
	case path == "/libpod/images/search":
		s.handleImagesSearch(w, r)
	case strings.HasPrefix(path, "/libpod/images/"):
		s.handleImagesDispatch(w, r)

	case path == "/libpod/manifests/create":
		s.handleManifestsCreate(w, r)
	case strings.HasPrefix(path, "/libpod/manifests/"):
		s.handleManifestsDispatch(w, r)

	case path == "/libpod/secrets/json":
		s.handleSecretsList(w, r)
	case path == "/libpod/secrets/create":
		s.handleSecretsCreate(w, r)
	case strings.HasPrefix(path, "/libpod/secrets/"):
		s.handleSecretsDispatch(w, r)

	case path == "/libpod/volumes/json":
		s.handleVolumesList(w, r)
	case path == "/libpod/volumes/create":
		s.handleVolumesCreate(w, r)
	case path == "/libpod/volumes/prune":
		s.handleVolumesPrune(w, r)
	case strings.HasPrefix(path, "/libpod/volumes/"):
		s.handleVolumesDispatch(w, r)

	case path == "/libpod/networks/json":
		s.handleNetworksList(w, r)
	case path == "/libpod/networks/create":
		s.handleNetworksCreate(w, r)
	case path == "/libpod/networks/prune":
		s.handleNetworksPrune(w, r)
	case strings.HasPrefix(path, "/libpod/networks/"):
		s.handleNetworksDispatch(w, r)

	case strings.HasPrefix(path, "/libpod/generate/"):
		s.handleGenerate(w, r)
	case path == "/libpod/play/kube":
		s.handlePlayKube(w, r)
	case path == "/libpod/auto-update":
		s.handleAutoUpdate(w, r)

	case path == "/libpod/info":
		s.handleSystemInfo(w, r)
	case path == "/libpod/version":
		s.handleSystemVersion(w, r)
	case path == "/libpod/_ping":
		s.handlePing(w, r)
	case path == "/libpod/events":
		s.handleEvents(w, r)
	case path == "/libpod/system/df":
		s.handleSystemDf(w, r)
	case path == "/libpod/system/prune":
		s.handleSystemPrune(w, r)
	case path == "/libpod/system/check":
		s.handleSystemCheck(w, r)
	case path == "/libpod/build":
		s.handleBuild(w, r)
	case strings.HasPrefix(path, "/libpod/quadlets/"):
		s.handleQuadlets(w, r)
	case strings.HasPrefix(path, "/libpod/artifacts/"):
		s.handleArtifacts(w, r)
	default:
		writeError(w, http.StatusNotFound, "endpoint not found")
	}
}

type Pod struct {
	ID           string            `json:"Id"`
	Name         string            `json:"Name"`
	Namespace    string            `json:"Namespace,omitempty"`
	Created      time.Time         `json:"Created"`
	Hostname     string            `json:"Hostname,omitempty"`
	Labels       map[string]string `json:"Labels,omitempty"`
	State        string            `json:"State"`
	Containers   []PodContainer    `json:"Containers"`
	InfraID      string            `json:"InfraContainerId"`
	CgroupPath   string            `json:"CgroupPath,omitempty"`
	SharedNS     []string          `json:"SharedNamespaces,omitempty"`
	CPUShares    int64             `json:"CpuShares,omitempty"`
	ExitPolicy   string            `json:"ExitPolicy,omitempty"`
	Restart      string            `json:"Restart,omitempty"`
	MemoryLimit  int64             `json:"MemoryLimit,omitempty"`
}

type PodContainer struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State string `json:"State"`
}

type PodCreateConfig struct {
	Name            string                     `json:"name"`
	Hostname        string                     `json:"hostname,omitempty"`
	DomainName      string                     `json:"domainname,omitempty"`
	Labels          map[string]string          `json:"labels,omitempty"`
	ShareNamespaces []string                   `json:"share_parent,omitempty"`
	InfraImage      string                     `json:"infra_image,omitempty"`
	InfraCommand    []string                   `json:"infra_command,omitempty"`
	InfraName       string                     `json:"infra_name,omitempty"`
	PortBindings    map[string][]PortBinding   `json:"portmappings,omitempty"`
	ExitPolicy      string                     `json:"exit_policy,omitempty"`
	CPUShares       int64                      `json:"cpu_shares,omitempty"`
	MemoryLimit     int64                      `json:"memory_limit,omitempty"`
	CPULimit        float64                    `json:"cpu_limit,omitempty"`
	Restart         string                     `json:"restart,omitempty"`
	Networks        map[string]NetworkOptions  `json:"networks,omitempty"`
	SecurityOpt     []string                   `json:"security_opt,omitempty"`
	NoInfra         bool                       `json:"no_infra,omitempty"`
	CgroupParent    string                     `json:"cgroup_parent,omitempty"`
	Pid             string                     `json:"pid,omitempty"`
	Userns          string                     `json:"userns,omitempty"`
	Uts             string                     `json:"uts,omitempty"`
	Volumes         []string                   `json:"volumes,omitempty"`
	DNSOptions      []string                   `json:"dns_option,omitempty"`
	DNSSearch       []string                   `json:"dns_search,omitempty"`
	DNServers       []string                   `json:"dns_server,omitempty"`
	HostAdd         []string                   `json:"hostadd,omitempty"`
}

type NetworkOptions struct {
	Aliases    []string `json:"aliases,omitempty"`
	StaticIP   string   `json:"static_ip,omitempty"`
	StaticMAC  string   `json:"static_mac,omitempty"`
}

type PortBinding struct {
	HostIP   string `json:"host_ip,omitempty"`
	HostPort int    `json:"host_port"`
}

type SecretSpec struct {
	Name   string            `json:"Name"`
	Labels map[string]string `json:"Labels,omitempty"`
	Driver string            `json:"Driver,omitempty"`
}

type Secret struct {
	ID        string     `json:"ID"`
	Spec      SecretSpec `json:"Spec"`
	Created   time.Time  `json:"CreatedAt"`
	Updated   time.Time  `json:"UpdatedAt"`
}

type ManifestList struct {
	Name      string          `json:"name"`
	Images    []ManifestEntry `json:"images"`
	MediaType string          `json:"mediaType"`
	Created   time.Time       `json:"created"`
	Modified  time.Time       `json:"modified"`
}

type ManifestEntry struct {
	Image     string   `json:"image"`
	Digest    string   `json:"digest"`
	MediaType string   `json:"mediaType"`
	Platform  Platform `json:"platform"`
}

type Platform struct {
	Architecture string   `json:"architecture"`
	OS           string   `json:"os"`
	OSVersion    string   `json:"os.version,omitempty"`
	OSFeatures   []string `json:"os.features,omitempty"`
	Variant      string   `json:"variant,omitempty"`
}

type Quadlet struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Path     string    `json:"path"`
	Status   string    `json:"status"`
	Created  time.Time `json:"created"`
}

type Artifact struct {
	Name      string    `json:"name"`
	Digest    string    `json:"digest"`
	MediaType string    `json:"media_type"`
	Size      int64     `json:"size"`
	Created   time.Time `json:"created"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{
		"cause":    msg,
		"message":  msg,
		"response": status,
	})
}

func parseDispatch(prefix, path string) (id, action string, ok bool) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || rest == "/" {
		return "", "", false
	}
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.SplitN(rest, "/", 2)
	id = parts[0]
	if id == "" {
		return "", "", false
	}
	if len(parts) == 2 {
		action = parts[1]
	}
	return id, action, true
}

func (s *PodmanServer) handlePodAction(w http.ResponseWriter, nameOrID, action string, r *http.Request) {
	var err error
	switch action {
	case "start":
		err = s.podMgr.StartPod(nameOrID)
	case "stop":
		err = s.podMgr.StopPod(nameOrID)
	case "kill":
		signal := r.URL.Query().Get("signal")
		if signal == "" {
			signal = "SIGTERM"
		}
		err = s.podMgr.KillPod(nameOrID, signal)
	case "restart":
		err = s.podMgr.RestartPod(nameOrID)
	case "pause":
		err = s.podMgr.PausePod(nameOrID)
	case "unpause":
		err = s.podMgr.UnpausePod(nameOrID)
	default:
		writeError(w, http.StatusNotFound, "unsupported pod action: "+action)
		return
	}
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *PodmanServer) handleContainerAction(w http.ResponseWriter, _, action string, _ *http.Request) {
	switch action {
	case "start", "stop", "kill", "restart", "pause", "unpause":
		writeError(w, http.StatusNotImplemented, "container lifecycle actions not yet supported by this build")
	default:
		writeError(w, http.StatusNotFound, "unsupported container action: "+action)
	}
}

func (s *PodmanServer) handleImageAction(w http.ResponseWriter, _, action string, _ *http.Request) {
	switch action {
	case "tag", "untag", "push", "save":
		writeError(w, http.StatusNotImplemented, "image action "+action+" not yet supported by this build")
	default:
		writeError(w, http.StatusNotFound, "unsupported image action: "+action)
	}
}

func (s *PodmanServer) handleManifestAction(w http.ResponseWriter, name, action string, r *http.Request) {
	switch action {
	case "add":
		var body struct {
			Image    string   `json:"image"`
			OS       string   `json:"os"`
			Arch     string   `json:"architecture"`
			Variant  string   `json:"variant"`
			Features []string `json:"osFeatures"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Image == "" {
			writeError(w, http.StatusBadRequest, "image is required")
			return
		}
		if err := s.manifestMgr.Add(name, body.Image, Platform{
			Architecture: body.Arch, OS: body.OS, Variant: body.Variant, OSFeatures: body.Features,
		}); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{})
	case "remove":
		image := r.URL.Query().Get("image")
		if err := s.manifestMgr.Remove(name, image); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "annotate":
		var body Platform
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		image := r.URL.Query().Get("image")
		if err := s.manifestMgr.Annotate(name, image, body); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusNotFound, "unsupported manifest action: "+action)
	}
}

func (s *PodmanServer) handleSecretAction(w http.ResponseWriter, nameOrID, action string, _ *http.Request) {
	switch action {
	case "inspect":
		secret, err := s.secretMgr.Get(nameOrID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, secret)
	default:
		writeError(w, http.StatusNotFound, "unsupported secret action: "+action)
	}
}

func (s *PodmanServer) handleVolumeAction(w http.ResponseWriter, nameOrID, action string, _ *http.Request) {
	switch action {
	case "inspect":
		writeJSON(w, http.StatusOK, map[string]interface{}{"Name": nameOrID})
	default:
		writeError(w, http.StatusNotFound, "unsupported volume action: "+action)
	}
}

func (s *PodmanServer) handleNetworkAction(w http.ResponseWriter, nameOrID, action string, _ *http.Request) {
	switch action {
	case "inspect":
		writeJSON(w, http.StatusOK, map[string]interface{}{"name": nameOrID})
	default:
		writeError(w, http.StatusNotFound, "unsupported network action: "+action)
	}
}

func (s *PodmanServer) handleContainersList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleContainersCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var cfg map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, _ := cfg["name"].(string)
	if name == "" {
		writeError(w, http.StatusBadRequest, "container name is required")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"Id":       common.ContainerID(),
		"Warnings": []string{},
	})
}

func (s *PodmanServer) handleContainersPrune(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Id": []string{}, "SpaceReclaimed": 0})
}

func (s *PodmanServer) handleContainersDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/containers/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]interface{}{"Id": id})
		case http.MethodDelete:
			writeJSON(w, http.StatusOK, map[string]interface{}{})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	s.handleContainerAction(w, id, action, r)
}

func (s *PodmanServer) handlePodsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var cfg PodCreateConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pod, err := s.podMgr.CreatePod(&cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pod)
}

func (s *PodmanServer) handlePodsList(w http.ResponseWriter, _ *http.Request) {
	pods := s.podMgr.ListPods()
	writeJSON(w, http.StatusOK, pods)
}

func (s *PodmanServer) handlePodsPrune(w http.ResponseWriter, _ *http.Request) {
	removed := s.podMgr.PrunePods()
	writeJSON(w, http.StatusOK, removed)
}

func (s *PodmanServer) handlePodsDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/pods/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "pod not found")
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			pod, err := s.podMgr.GetPod(id)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, pod)
		case http.MethodDelete:
			force := r.URL.Query().Get("force") == "true"
			if err := s.podMgr.RemovePod(id, force); err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	s.handlePodAction(w, id, action, r)
}

func (s *PodmanServer) handleImagesList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleImagesPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "pulling"})
}

func (s *PodmanServer) handleImagesPrune(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Id": []string{}, "SpaceReclaimed": 0})
}

func (s *PodmanServer) handleImagesSearch(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("term")
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleImagesDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/images/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]interface{}{"Id": id})
		case http.MethodDelete:
			writeJSON(w, http.StatusOK, map[string]interface{}{})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	s.handleImageAction(w, id, action, r)
}

func (s *PodmanServer) handleManifestsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Name   string   `json:"name"`
		Images []string `json:"images"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ml, err := s.manifestMgr.Create(body.Name, body.Images)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ml)
}

func (s *PodmanServer) handleManifestsDispatch(w http.ResponseWriter, r *http.Request) {
	name, action, ok := parseDispatch("/libpod/manifests/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "manifest not found")
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			ml, err := s.manifestMgr.Inspect(name)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, ml)
		case http.MethodDelete:
			if err := s.manifestMgr.Delete(name); err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	s.handleManifestAction(w, name, action, r)
}

func (s *PodmanServer) handleSecretsList(w http.ResponseWriter, _ *http.Request) {
	secrets := s.secretMgr.ListSecrets()
	writeJSON(w, http.StatusOK, secrets)
}

func (s *PodmanServer) handleSecretsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Name   string            `json:"name"`
		Data   string            `json:"data"`
		Driver string            `json:"Driver"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	secret, err := s.secretMgr.Create(body.Name, []byte(body.Data), body.Driver, body.Labels)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"ID": secret.ID})
}

func (s *PodmanServer) handleSecretsDispatch(w http.ResponseWriter, r *http.Request) {
	nameOrID, action, ok := parseDispatch("/libpod/secrets/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			secret, err := s.secretMgr.Get(nameOrID)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, secret)
		case http.MethodDelete:
			if err := s.secretMgr.Remove(nameOrID); err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	s.handleSecretAction(w, nameOrID, action, r)
}

func (s *PodmanServer) handleVolumesList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Volumes": []interface{}{}, "Warnings": []string{}})
}

func (s *PodmanServer) handleVolumesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "volume name is required")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"Name": body.Name})
}

func (s *PodmanServer) handleVolumesPrune(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"VolumesDeleted": []string{}, "SpaceReclaimed": 0})
}

func (s *PodmanServer) handleVolumesDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/volumes/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "volume not found")
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]interface{}{"Name": id})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	s.handleVolumeAction(w, id, action, r)
}

func (s *PodmanServer) handleNetworksList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleNetworksCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, _ := body["name"].(string)
	if name == "" {
		writeError(w, http.StatusBadRequest, "network name is required")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"name": name})
}

func (s *PodmanServer) handleNetworksPrune(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"NetworksDeleted": []string{}})
}

func (s *PodmanServer) handleNetworksDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/networks/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "network not found")
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]interface{}{"name": id})
		case http.MethodDelete:
			writeJSON(w, http.StatusOK, map[string]interface{}{})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	s.handleNetworkAction(w, id, action, r)
}

func (s *PodmanServer) handleGenerate(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/libpod/generate/")
	if rest == "" || strings.Contains(rest, "/") {
		writeError(w, http.StatusNotFound, "unsupported generate target")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *PodmanServer) handlePlayKube(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"PlayKubeReport": []interface{}{}})
}

func (s *PodmanServer) handleAutoUpdate(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Results": []interface{}{}})
}

func (s *PodmanServer) handleSystemInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"host": map[string]interface{}{
			"arch":           "arm64",
			"os":             "linux",
			"kernel":         "6.8.0",
			"MemTotal":       8 * 1024 * 1024 * 1024,
			"SwapTotal":      int64(0),
			"Conmon":         map[string]interface{}{"package": "unknown", "path": "/usr/bin/conmon"},
			"OCIRuntime":     map[string]interface{}{"name": "doki", "path": "/usr/bin/doki"},
			"Security":       map[string]interface{}{"rootless": true},
		},
		"store": map[string]interface{}{
			"GraphDriverName": "overlay",
			"ImageStore":      map[string]interface{}{"number": 0},
		},
		"version": map[string]interface{}{
			"APIVersion": "5.0.0",
			"Version":    "0.10.0",
			"GoVersion":  "go1.26",
			"OsArch":     "linux/arm64",
		},
	})
}

func (s *PodmanServer) handleSystemVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Client": map[string]interface{}{
			"APIVersion": "5.0.0",
			"Version":    "0.10.0",
			"GoVersion":  "go1.26",
			"OsArch":     "linux/arm64",
		},
	})
}

func (s *PodmanServer) handlePing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "OK")
}

func (s *PodmanServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	interval := 5 * time.Second
	if v := r.URL.Query().Get("interval"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ctx := r.Context()
	if err := writeEvent(w, map[string]interface{}{"status": "connected", "time": time.Now().Unix()}); err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			event := map[string]interface{}{
				"Time":   t.Unix(),
				"Type":   "ping",
				"Status": "ok",
			}
			if err := writeEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return nil
}

func (s *PodmanServer) handleSystemDf(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Images":     []interface{}{},
		"Containers": []interface{}{},
		"Volumes":    []interface{}{},
	})
}

func (s *PodmanServer) handleSystemPrune(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"SpaceReclaimed": 0})
}

func (s *PodmanServer) handleSystemCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Status": "ok"})
}

func (s *PodmanServer) handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"stream": "build not implemented in this build"})
}

func (s *PodmanServer) handleQuadlets(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/libpod/quadlets/")
	switch {
	case rest == "" || rest == "json":
		writeJSON(w, http.StatusOK, []Quadlet{})
	case strings.HasSuffix(rest, "/status"):
		name := strings.TrimSuffix(rest, "/status")
		writeJSON(w, http.StatusOK, []map[string]string{{"name": name, "status": "unknown"}})
	default:
		writeJSON(w, http.StatusOK, []interface{}{})
	}
}

func (s *PodmanServer) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/libpod/artifacts/")
	if rest == "" {
		writeJSON(w, http.StatusOK, []Artifact{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"name": rest})
}
