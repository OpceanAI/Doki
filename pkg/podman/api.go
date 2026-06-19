package podman

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
	mux.HandleFunc("/libpod/containers/json", s.handleContainersList)
	mux.HandleFunc("/libpod/containers/create", s.handleContainersCreate)
	mux.HandleFunc("/libpod/containers/prune", s.handleContainersPrune)
	mux.HandleFunc("/libpod/containers/", s.handleContainersDispatch)

	mux.HandleFunc("/libpod/pods/create", s.handlePodsCreate)
	mux.HandleFunc("/libpod/pods/json", s.handlePodsList)
	mux.HandleFunc("/libpod/pods/prune", s.handlePodsPrune)
	mux.HandleFunc("/libpod/pods/", s.handlePodsDispatch)

	mux.HandleFunc("/libpod/images/json", s.handleImagesList)
	mux.HandleFunc("/libpod/images/pull", s.handleImagesPull)
	mux.HandleFunc("/libpod/images/prune", s.handleImagesPrune)
	mux.HandleFunc("/libpod/images/search", s.handleImagesSearch)
	mux.HandleFunc("/libpod/images/", s.handleImagesDispatch)

	mux.HandleFunc("/libpod/manifests/create", s.handleManifestsCreate)
	mux.HandleFunc("/libpod/manifests/", s.handleManifestsDispatch)

	mux.HandleFunc("/libpod/secrets/json", s.handleSecretsList)
	mux.HandleFunc("/libpod/secrets/create", s.handleSecretsCreate)
	mux.HandleFunc("/libpod/secrets/", s.handleSecretsDispatch)

	mux.HandleFunc("/libpod/volumes/json", s.handleVolumesList)
	mux.HandleFunc("/libpod/volumes/create", s.handleVolumesCreate)
	mux.HandleFunc("/libpod/volumes/prune", s.handleVolumesPrune)
	mux.HandleFunc("/libpod/volumes/", s.handleVolumesDispatch)

	mux.HandleFunc("/libpod/networks/json", s.handleNetworksList)
	mux.HandleFunc("/libpod/networks/create", s.handleNetworksCreate)
	mux.HandleFunc("/libpod/networks/prune", s.handleNetworksPrune)
	mux.HandleFunc("/libpod/networks/", s.handleNetworksDispatch)

	mux.HandleFunc("/libpod/generate/", s.handleGenerate)
	mux.HandleFunc("/libpod/play/kube", s.handlePlayKube)
	mux.HandleFunc("/libpod/auto-update", s.handleAutoUpdate)

	mux.HandleFunc("/libpod/info", s.handleSystemInfo)
	mux.HandleFunc("/libpod/version", s.handleSystemVersion)
	mux.HandleFunc("/libpod/_ping", s.handlePing)
	mux.HandleFunc("/libpod/events", s.handleEvents)
	mux.HandleFunc("/libpod/system/df", s.handleSystemDf)
	mux.HandleFunc("/libpod/system/prune", s.handleSystemPrune)
	mux.HandleFunc("/libpod/system/check", s.handleSystemCheck)

	mux.HandleFunc("/libpod/build", s.handleBuild)

	mux.HandleFunc("/libpod/quadlets/", s.handleQuadlets)
	mux.HandleFunc("/libpod/artifacts/", s.handleArtifacts)
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
	Name            string            `json:"name"`
	Hostname        string            `json:"hostname,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	ShareNamespaces []string          `json:"share_parent,omitempty"`
	InfraImage      string            `json:"infra_image,omitempty"`
	InfraCommand    []string          `json:"infra_command,omitempty"`
	InfraName       string            `json:"infra_name,omitempty"`
	PortBindings    map[string][]PortBinding `json:"portmappings,omitempty"`
	ExitPolicy      string            `json:"exit_policy,omitempty"`
	CPUShares       int64             `json:"cpu_shares,omitempty"`
	MemoryLimit     int64             `json:"memory_limit,omitempty"`
	Restart         string            `json:"restart,omitempty"`
	Networks        map[string]NetworkOptions `json:"networks,omitempty"`
	SecurityOpt     []string          `json:"security_opt,omitempty"`
	NoInfra         bool              `json:"no_infra,omitempty"`
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

type Secret struct {
	ID        string            `json:"ID"`
	Name      string            `json:"Spec"`
	Created   time.Time         `json:"CreatedAt"`
	Updated   time.Time         `json:"UpdatedAt"`
	Labels    map[string]string `json:"Spec,omitempty"`
	Driver    string            `json:"Driver,omitempty"`
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
		"cause":   msg,
		"message": msg,
		"response": status,
	})
}

func (s *PodmanServer) handleContainersList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleContainersCreate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusCreated, map[string]interface{}{"Id": "stub", "Warnings": []string{}})
}

func (s *PodmanServer) handleContainersPrune(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Id": []string{}, "SpaceReclaimed": 0})
}

func (s *PodmanServer) handleContainersDispatch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "container not found")
}

func (s *PodmanServer) handlePodsCreate(w http.ResponseWriter, r *http.Request) {
	var cfg PodCreateConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pod, err := s.podMgr.CreatePod(&cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pod)
}

func (s *PodmanServer) handlePodsList(w http.ResponseWriter, r *http.Request) {
	pods := s.podMgr.ListPods()
	writeJSON(w, http.StatusOK, pods)
}

func (s *PodmanServer) handlePodsPrune(w http.ResponseWriter, r *http.Request) {
	removed := s.podMgr.PrunePods()
	writeJSON(w, http.StatusOK, removed)
}

func (s *PodmanServer) handlePodsDispatch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "pod not found")
}

func (s *PodmanServer) handleImagesList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleImagesPull(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *PodmanServer) handleImagesPrune(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Id": []string{}, "SpaceReclaimed": 0})
}

func (s *PodmanServer) handleImagesSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleImagesDispatch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "image not found")
}

func (s *PodmanServer) handleManifestsCreate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusCreated, map[string]interface{}{"Id": "stub"})
}

func (s *PodmanServer) handleManifestsDispatch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "manifest not found")
}

func (s *PodmanServer) handleSecretsList(w http.ResponseWriter, r *http.Request) {
	secrets := s.secretMgr.ListSecrets()
	writeJSON(w, http.StatusOK, secrets)
}

func (s *PodmanServer) handleSecretsCreate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusCreated, map[string]interface{}{"ID": "stub"})
}

func (s *PodmanServer) handleSecretsDispatch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "secret not found")
}

func (s *PodmanServer) handleVolumesList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Volumes": []interface{}{}})
}

func (s *PodmanServer) handleVolumesCreate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

func (s *PodmanServer) handleVolumesPrune(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"VolumesDeleted": []string{}, "SpaceReclaimed": 0})
}

func (s *PodmanServer) handleVolumesDispatch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "volume not found")
}

func (s *PodmanServer) handleNetworksList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleNetworksCreate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

func (s *PodmanServer) handleNetworksPrune(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"NetworksDeleted": []string{}})
}

func (s *PodmanServer) handleNetworksDispatch(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "network not found")
}

func (s *PodmanServer) handleGenerate(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}

func (s *PodmanServer) handlePlayKube(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *PodmanServer) handleAutoUpdate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Results": []interface{}{}})
}

func (s *PodmanServer) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
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

func (s *PodmanServer) handleSystemVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Client": map[string]interface{}{
			"APIVersion": "5.0.0",
			"Version":    "0.10.0",
			"GoVersion":  "go1.26",
			"OsArch":     "linux/arm64",
		},
	})
}

func (s *PodmanServer) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "OK")
}

func (s *PodmanServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func (s *PodmanServer) handleSystemDf(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Images":     []interface{}{},
		"Containers": []interface{}{},
		"Volumes":    []interface{}{},
	})
}

func (s *PodmanServer) handleSystemPrune(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"SpaceReclaimed": 0})
}

func (s *PodmanServer) handleSystemCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Status": "ok"})
}

func (s *PodmanServer) handleBuild(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *PodmanServer) handleQuadlets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *PodmanServer) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}
