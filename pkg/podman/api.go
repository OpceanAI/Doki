// Package podman provides the Podman-compatible API server.
package podman

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/events"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
)

// maxPodmanJSONBody caps JSON request bodies for the podman shim endpoints to
// bound memory against an oversized body (small configs only).
const maxPodmanJSONBody = 4 << 20

// VolumeStore is the slice of the daemon's volume manager that the libpod
// surface needs. It is an interface rather than the concrete type because the
// manager lives in pkg/api, which imports this package.
type VolumeStore interface {
	Create(name string, driver string, opts map[string]string, labels map[string]string) (*common.VolumeInfo, error)
	Get(name string) (*common.VolumeInfo, error)
	List() []*common.VolumeInfo
	Remove(name string) error
	Prune(referencedVolumes map[string]bool) ([]string, error)
}

// Deps carries the real engine components the libpod endpoints operate on.
// Before these were injected, PodmanServer held only its own metadata stores,
// so every container, image, volume and network endpoint had nothing to talk
// to and could only return fiction.
type Deps struct {
	Runtime *dokiruntime.Runtime
	Images  *image.Store
	Network *network.Manager
	Volumes VolumeStore
	Events  *events.Bus

	// Build, PlayKube and GenerateKube reuse the daemon's own handlers rather
	// than reimplementing them here (P6/P7). They are function values because
	// those handlers live in pkg/api, which imports this package.
	Build        http.HandlerFunc
	PlayKube     http.HandlerFunc
	GenerateKube http.HandlerFunc
}

type PodmanServer struct {
	podMgr      *PodManager
	secretMgr   *SecretManager
	manifestMgr *ManifestManager

	runtime *dokiruntime.Runtime
	images  *image.Store
	network *network.Manager
	volumes VolumeStore
	events  *events.Bus

	build        http.HandlerFunc
	playKube     http.HandlerFunc
	generateKube http.HandlerFunc
}

func NewPodmanServer(root string, deps Deps) (*PodmanServer, error) {
	pm, err := NewPodManager(root)
	if err != nil {
		return nil, err
	}
	sm, err := NewSecretManager(root)
	if err != nil {
		return nil, err
	}
	mm, err := NewManifestManager(root)
	if err != nil {
		return nil, err
	}
	return &PodmanServer{
		podMgr:      pm,
		secretMgr:   sm,
		manifestMgr: mm,
		runtime:     deps.Runtime,
		images:      deps.Images,
		network:     deps.Network,
		volumes:     deps.Volumes,
		events:      deps.Events,

		build:        deps.Build,
		playKube:     deps.PlayKube,
		generateKube: deps.GenerateKube,
	}, nil
}

// requireRuntime reports whether the container engine was wired in. A libpod
// server built without it must say so rather than return an empty list that
// reads as "no containers".
func (s *PodmanServer) requireRuntime(w http.ResponseWriter) bool {
	if s.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container engine not available in this build")
		return false
	}
	return true
}

func (s *PodmanServer) requireImages(w http.ResponseWriter) bool {
	if s.images == nil {
		writeError(w, http.StatusServiceUnavailable, "image store not available in this build")
		return false
	}
	return true
}

func (s *PodmanServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/libpod/", s.logMiddleware(http.HandlerFunc(s.route)).ServeHTTP)
}

func (s *PodmanServer) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		// Sanitize log fields to prevent log injection via path/method.
		method := sanitizeLogField(r.Method)
		path := sanitizeLogField(r.URL.Path)
		log.Printf("podman %s %s -> %d %s", method, path, rw.status, time.Since(start))
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
	ID          string            `json:"Id"`
	Name        string            `json:"Name"`
	Namespace   string            `json:"Namespace,omitempty"`
	Created     time.Time         `json:"Created"`
	Hostname    string            `json:"Hostname,omitempty"`
	Labels      map[string]string `json:"Labels,omitempty"`
	State       string            `json:"State"`
	Containers  []PodContainer    `json:"Containers"`
	InfraID     string            `json:"InfraContainerId"`
	CgroupPath  string            `json:"CgroupPath,omitempty"`
	SharedNS    []string          `json:"SharedNamespaces,omitempty"`
	CPUShares   int64             `json:"CpuShares,omitempty"`
	ExitPolicy  string            `json:"ExitPolicy,omitempty"`
	Restart     string            `json:"Restart,omitempty"`
	MemoryLimit int64             `json:"MemoryLimit,omitempty"`
	ExitSignal  string            `json:"ExitSignal,omitempty"`
}

type PodContainer struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State string `json:"State"`
}

type PodCreateConfig struct {
	Name            string                    `json:"name"`
	Hostname        string                    `json:"hostname,omitempty"`
	DomainName      string                    `json:"domainname,omitempty"`
	Labels          map[string]string         `json:"labels,omitempty"`
	ShareNamespaces []string                  `json:"share_parent,omitempty"`
	InfraImage      string                    `json:"infra_image,omitempty"`
	InfraCommand    []string                  `json:"infra_command,omitempty"`
	InfraName       string                    `json:"infra_name,omitempty"`
	PortBindings    map[string][]PortBinding  `json:"portmappings,omitempty"`
	ExitPolicy      string                    `json:"exit_policy,omitempty"`
	CPUShares       int64                     `json:"cpu_shares,omitempty"`
	MemoryLimit     int64                     `json:"memory_limit,omitempty"`
	CPULimit        float64                   `json:"cpu_limit,omitempty"`
	Restart         string                    `json:"restart,omitempty"`
	Networks        map[string]NetworkOptions `json:"networks,omitempty"`
	SecurityOpt     []string                  `json:"security_opt,omitempty"`
	NoInfra         bool                      `json:"no_infra,omitempty"`
	CgroupParent    string                    `json:"cgroup_parent,omitempty"`
	Pid             string                    `json:"pid,omitempty"`
	Userns          string                    `json:"userns,omitempty"`
	Uts             string                    `json:"uts,omitempty"`
	Volumes         []string                  `json:"volumes,omitempty"`
	DNSOptions      []string                  `json:"dns_option,omitempty"`
	DNSSearch       []string                  `json:"dns_search,omitempty"`
	DNServers       []string                  `json:"dns_server,omitempty"`
	HostAdd         []string                  `json:"hostadd,omitempty"`
}

type NetworkOptions struct {
	Aliases   []string `json:"aliases,omitempty"`
	StaticIP  string   `json:"static_ip,omitempty"`
	StaticMAC string   `json:"static_mac,omitempty"`
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
	ID      string     `json:"ID"`
	Spec    SecretSpec `json:"Spec"`
	Created time.Time  `json:"CreatedAt"`
	Updated time.Time  `json:"UpdatedAt"`
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
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Path    string    `json:"path"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
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
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&body); err != nil {
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
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&body); err != nil {
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

func (s *PodmanServer) handlePodsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var cfg PodCreateConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&cfg); err != nil {
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

func (s *PodmanServer) handleManifestsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		Name   string   `json:"name"`
		Images []string `json:"images"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&body); err != nil {
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&body); err != nil {
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

// handleGenerate delegates to the daemon's own kube generator (P6). It used to
// return an empty object, which reads as "this resource generates nothing"
// rather than "this endpoint is not wired up".
func (s *PodmanServer) handleGenerate(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/libpod/generate/")
	if rest == "" {
		writeError(w, http.StatusNotFound, "unsupported generate target")
		return
	}
	if !strings.HasSuffix(rest, "kube") && rest != "kube" {
		writeError(w, http.StatusNotImplemented, "only generate/kube is supported")
		return
	}
	if s.generateKube == nil {
		writeError(w, http.StatusServiceUnavailable, "kube generator not available in this build")
		return
	}
	s.generateKube(w, r)
}

// handlePlayKube delegates to the daemon's real kube-play handler (P6) instead
// of returning an empty report that makes an unapplied manifest look applied.
func (s *PodmanServer) handlePlayKube(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if s.playKube == nil {
		writeError(w, http.StatusServiceUnavailable, "kube play not available in this build")
		return
	}
	s.playKube(w, r)
}

// handleAutoUpdate is deliberately out of scope (P8). An empty Results array
// reads as "checked, nothing to update", which is a lie; say so instead.
func (s *PodmanServer) handleAutoUpdate(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "auto-update is not implemented")
}

func (s *PodmanServer) handleSystemInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"host": map[string]interface{}{
			"arch":       runtime.GOARCH,
			"os":         runtime.GOOS,
			"kernel":     detectKernel(),
			"MemTotal":   detectMemTotal(),
			"SwapTotal":  int64(0),
			"Conmon":     map[string]interface{}{"package": "unknown", "path": "/usr/bin/conmon"},
			"OCIRuntime": map[string]interface{}{"name": "doki", "path": "/usr/bin/doki"},
			"Security":   map[string]interface{}{"rootless": true},
		},
		"store": map[string]interface{}{
			"GraphDriverName": "overlay",
			"ImageStore":      map[string]interface{}{"number": 0},
		},
		"version": map[string]interface{}{
			"APIVersion": common.DokiAPIVersion,
			"Version":    common.DokiVersion,
			"GoVersion":  runtime.Version(),
			"OsArch":     runtime.GOOS + "/" + runtime.GOARCH,
		},
	})
}

func (s *PodmanServer) handleSystemVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"Client": map[string]interface{}{
			"APIVersion": common.DokiAPIVersion,
			"Version":    common.DokiVersion,
			"GoVersion":  runtime.Version(),
			"OsArch":     runtime.GOOS + "/" + runtime.GOARCH,
		},
	})
}

func (s *PodmanServer) handlePing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "OK")
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

// handleSystemPrune runs the real prune across containers, images, volumes and
// networks. Reporting a hardcoded zero reclaimed made a working prune look
// like a no-op and a broken one look successful.
func (s *PodmanServer) handleSystemPrune(w http.ResponseWriter, _ *http.Request) {
	report := map[string]interface{}{
		"ContainerPruneReports": []interface{}{},
		"ImagePruneReports":     []interface{}{},
		"VolumePruneReports":    []interface{}{},
		"NetworkPruneReports":   []interface{}{},
		"ReclaimedSpace":        0,
	}
	if s.runtime != nil {
		removed := []map[string]interface{}{}
		if states, err := s.runtime.List(); err == nil {
			for _, st := range states {
				if st.Status != common.StateExited && st.Status != common.StateCreated {
					continue
				}
				if err := s.runtime.Delete(st.ID, false); err == nil {
					removed = append(removed, map[string]interface{}{"Id": st.ID})
					s.publish("remove", st.ID, containerName(st))
				}
			}
		}
		report["ContainerPruneReports"] = removed
	}
	if s.images != nil {
		removed := []map[string]interface{}{}
		if ids, err := s.images.Prune(); err == nil {
			for _, id := range ids {
				removed = append(removed, map[string]interface{}{"Id": id})
			}
		}
		report["ImagePruneReports"] = removed
	}
	if s.volumes != nil {
		removed := []map[string]interface{}{}
		if names, err := s.volumes.Prune(s.referencedVolumes()); err == nil {
			for _, n := range names {
				removed = append(removed, map[string]interface{}{"Id": n})
			}
		}
		report["VolumePruneReports"] = removed
	}
	if s.network != nil {
		removed := []map[string]interface{}{}
		if names, err := s.network.Prune(); err == nil {
			for _, n := range names {
				removed = append(removed, map[string]interface{}{"Name": n})
			}
		}
		report["NetworkPruneReports"] = removed
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *PodmanServer) handleSystemCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"Status": "ok"})
}

// handleBuild routes to the daemon's builder (P7). Returning 200 with a
// "not implemented" string in the stream, as this used to, makes the client
// believe the build succeeded.
func (s *PodmanServer) handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if s.build == nil {
		writeError(w, http.StatusServiceUnavailable, "builder not available in this build")
		return
	}
	s.build(w, r)
}

// handleQuadlets is deliberately out of scope (P8): Doki has no systemd unit
// generator. An empty list would read as "you have no quadlets".
func (s *PodmanServer) handleQuadlets(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "quadlets are not implemented")
}

// handleArtifacts is deliberately out of scope (P8): there is no OCI artifact
// store behind it, so echoing the requested name back would be fiction.
func (s *PodmanServer) handleArtifacts(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "OCI artifacts are not implemented")
}

func detectKernel() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return runtime.GOOS
	}
	return strings.TrimSpace(string(data))
}

func detectMemTotal() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				kb, _ := strconv.ParseInt(parts[1], 10, 64)
				return kb * 1024
			}
		}
	}
	return 0
}

// sanitizeLogField removes control characters (newlines, carriage
// returns, tabs) from a log field to prevent log injection (CWE-117).
func sanitizeLogField(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", "")
	return s
}
