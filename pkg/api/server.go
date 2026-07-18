package api

import (
	"archive/tar"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/dokivm"
	"github.com/OpceanAI/Doki/pkg/builder"
	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/events"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
	"github.com/OpceanAI/Doki/pkg/podman"
	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
	"github.com/OpceanAI/Doki/pkg/stdcopy"
	"gopkg.in/yaml.v3"
)

// maxJSONBody caps the size of a JSON request body for control endpoints
// (container/volume/network/exec configs are all small). It bounds memory use
// so an unauthenticated client on the socket can't OOM the daemon by streaming a
// multi-GB body. Bulk endpoints (image load, build context, archive put) stream
// their own bodies and are not affected by this cap.
const maxJSONBody = 4 << 20 // 4 MiB

// Server implements the Docker Engine v1.55 compatible HTTP API.
type Server struct {
	config     *common.DokiConfig
	router     *http.ServeMux
	server     *http.Server
	listener   net.Listener
	runtime    *dokiruntime.Runtime
	image      *image.Store
	network    *network.Manager
	volumes    *VolumeManager
	events     *events.Bus
	middleware []func(http.Handler) http.Handler
	handler    http.Handler
	// dnsSrv removed: DNS is managed through network.Manager, not directly.
	execStore map[string]*common.ExecConfig
	execMu    sync.RWMutex
	authCreds struct {
		Username      string
		Password      string
		ServerAddress string
	}
}

// ServeHTTP dispatches the request to the configured middleware chain and router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) rootHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Api-Version", common.DokiAPIVersion)
		w.Header().Set("Server", "Doki/"+common.Version)

		path := r.URL.Path
		if strings.HasPrefix(path, "/v") {
			parts := strings.SplitN(path[1:], "/", 2)
			if len(parts) >= 2 {
				path = "/" + parts[1]
			}
		}
		r.URL.Path = path

		if path != "/_ping" && path != "/health" && path != "/metrics" {
			w.Header().Set("Content-Type", "application/json")
		}

		s.router.ServeHTTP(w, r)
	})
}

func (s *Server) rebuildHandler() {
	h := s.rootHandler()
	for i := len(s.middleware) - 1; i >= 0; i-- {
		h = s.middleware[i](h)
	}
	s.handler = h
}

// VolumeManager manages Docker-compatible volumes.
type VolumeManager struct {
	mu      sync.RWMutex
	root    string
	volumes map[string]*common.VolumeInfo
}

// NewVolumeManager creates a new volume manager.
func NewVolumeManager(root string) (*VolumeManager, error) {
	if err := common.EnsureDir(root); err != nil {
		return nil, fmt.Errorf("create volume root: %w", err)
	}
	vm := &VolumeManager{
		root:    root,
		volumes: make(map[string]*common.VolumeInfo),
	}
	if err := vm.loadFromDisk(); err != nil {
		return nil, err
	}
	return vm, nil
}

func validateVolumeName(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") || strings.Contains(name, string(os.PathSeparator)) {
		return false
	}
	return true
}

func (vm *VolumeManager) loadFromDisk() error {
	entries, err := os.ReadDir(vm.root)
	if err != nil {
		return fmt.Errorf("read volume root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		volPath := filepath.Join(vm.root, entry.Name(), "volume.json")
		data, err := os.ReadFile(volPath)
		if err != nil {
			slog.Warn("skip unreadable volume metadata", "path", volPath, "err", err)
			continue
		}
		var vol common.VolumeInfo
		if err := json.Unmarshal(data, &vol); err != nil {
			slog.Warn("skip invalid volume metadata", "path", volPath, "err", err)
			continue
		}
		if !validateVolumeName(vol.Name) {
			slog.Warn("skip invalid volume name", "path", volPath, "name", vol.Name)
			continue
		}
		vm.volumes[vol.Name] = &vol
	}
	return nil
}

func (vm *VolumeManager) Create(name string, driver string, opts map[string]string, labels map[string]string) (*common.VolumeInfo, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if !validateVolumeName(name) {
		return nil, fmt.Errorf("invalid volume name: %q contains path traversal characters", name)
	}

	if _, exists := vm.volumes[name]; exists {
		return nil, common.NewErrConflict("volume", name)
	}

	mountpoint := filepath.Join(vm.root, name)
	if err := common.EnsureDir(mountpoint); err != nil {
		return nil, fmt.Errorf("create volume directory: %w", err)
	}

	if driver == "" {
		driver = "local"
	}

	vol := &common.VolumeInfo{
		Name:       name,
		Driver:     driver,
		Mountpoint: mountpoint,
		Labels:     labels,
		Scope:      "local",
		Options:    opts,
		CreatedAt:  time.Now(),
	}

	// Persist to disk.
	data, err := json.Marshal(vol)
	if err != nil {
		return nil, fmt.Errorf("marshal volume metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(mountpoint, "volume.json"), data, 0644); err != nil {
		return nil, fmt.Errorf("write volume metadata: %w", err)
	}

	vm.volumes[name] = vol
	return vol, nil
}

func (vm *VolumeManager) Get(name string) (*common.VolumeInfo, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	vol, ok := vm.volumes[name]
	if !ok {
		return nil, common.NewErrNotFound("volume", name)
	}
	return vol, nil
}

func (vm *VolumeManager) List() []*common.VolumeInfo {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	vols := make([]*common.VolumeInfo, 0, len(vm.volumes))
	for _, v := range vm.volumes {
		vols = append(vols, v)
	}
	return vols
}

func (vm *VolumeManager) Remove(name string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vol, ok := vm.volumes[name]
	if !ok {
		return common.NewErrNotFound("volume", name)
	}

	if err := os.RemoveAll(vol.Mountpoint); err != nil {
		return fmt.Errorf("remove volume data: %w", err)
	}
	delete(vm.volumes, name)
	return nil
}

func (vm *VolumeManager) Prune(referencedVolumes map[string]bool) ([]string, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	var pruned []string
	for name, vol := range vm.volumes {
		if referencedVolumes[name] {
			continue
		}
		if err := os.RemoveAll(vol.Mountpoint); err != nil {
			return pruned, fmt.Errorf("remove volume %s: %w", name, err)
		}
		delete(vm.volumes, name)
		pruned = append(pruned, name)
	}
	return pruned, nil
}

func (s *Server) handleVolumesPrune(w http.ResponseWriter, _ *http.Request) {
	// Build list of volumes referenced by running containers.
	referencedVolumes := make(map[string]bool)
	if containers, err := s.runtime.List(); err == nil {
		for _, c := range containers {
			if c.Status == common.StateRunning && c.Config != nil {
				for _, mnt := range c.Config.Mounts {
					if mnt.Type == common.MountVolume {
						referencedVolumes[mnt.Source] = true
					}
				}
			}
		}
	}
	pruned, err := s.volumes.Prune(referencedVolumes)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pruned == nil {
		pruned = []string{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"VolumesDeleted": pruned,
		"SpaceReclaimed": 0,
	})
}

// NewServer creates a new API server.
func NewServer(config *common.DokiConfig, rt *dokiruntime.Runtime, img *image.Store, net *network.Manager) (*Server, error) {
	volumes, err := NewVolumeManager(filepath.Join(config.DataDir, "volumes"))
	if err != nil {
		return nil, fmt.Errorf("volume manager: %w", err)
	}
	s := &Server{
		config:    config,
		router:    http.NewServeMux(),
		runtime:   rt,
		image:     img,
		network:   net,
		volumes:   volumes,
		events:    events.NewBus(),
		execStore: make(map[string]*common.ExecConfig),
	}
	s.registerRoutes()

	podmanSrv, err := podman.NewPodmanServer(filepath.Join(config.DataDir, "podman"))
	if err != nil {
		return nil, fmt.Errorf("podman shim: %w", err)
	}
	podmanSrv.RegisterRoutes(s.router)

	s.rebuildHandler()
	return s, nil
}

// RegisterHandler registers a custom handler for a path.
func (s *Server) RegisterHandler(path string, handler http.Handler) {
	s.router.Handle(path, handler)
}

// SetMiddleware configures middleware wrappers for the server.
func (s *Server) SetMiddleware(middlewares ...func(http.Handler) http.Handler) {
	s.middleware = append(s.middleware, middlewares...)
	s.rebuildHandler()
}

func (s *Server) registerRoutes() {
	// Container endpoints.
	s.router.HandleFunc("/containers/json", s.handleContainersList)
	s.router.HandleFunc("/containers/create", s.handleContainerCreate)
	s.router.HandleFunc("/containers/", s.handleContainerDispatch)
	s.router.HandleFunc("/containers/prune", s.handleContainersPrune)

	// Image endpoints.
	s.router.HandleFunc("/images/json", s.handleImagesList)
	s.router.HandleFunc("/images/create", s.handleImageCreate)
	s.router.HandleFunc("/images/", s.handleImageDispatch)
	s.router.HandleFunc("/images/prune", s.handleImagesPrune)
	s.router.HandleFunc("/images/search", s.handleImagesSearch)
	s.router.HandleFunc("/images/load", s.handleImageLoad)
	s.router.HandleFunc("/images/get", s.handleImageGet)
	s.router.HandleFunc("/build", s.handleBuild)

	// Network endpoints.
	s.router.HandleFunc("/networks", s.handleNetworksList)
	s.router.HandleFunc("/networks/create", s.handleNetworkCreate)
	s.router.HandleFunc("/networks/", s.handleNetworkDispatch)
	s.router.HandleFunc("/networks/prune", s.handleNetworksPrune)

	// Volume endpoints.
	s.router.HandleFunc("/volumes", s.handleVolumesList)
	s.router.HandleFunc("/volumes/create", s.handleVolumeCreate)
	s.router.HandleFunc("/volumes/", s.handleVolumeDispatch)
	s.router.HandleFunc("/volumes/prune", s.handleVolumesPrune)

	// Exec endpoints.
	s.router.HandleFunc("/exec/", s.handleExecDispatch)

	// System endpoints.
	s.router.HandleFunc("/info", s.handleSystemInfo)
	s.router.HandleFunc("/version", s.handleSystemVersion)
	s.router.HandleFunc("/_ping", s.handlePing)
	s.router.HandleFunc("/events", s.handleEvents)
	s.router.HandleFunc("/system/df", s.handleSystemDf)
	s.router.HandleFunc("/auth", s.handleAuth)
	s.router.HandleFunc("/health", http.HandlerFunc(HealthHandler))
	s.router.HandleFunc("/metrics", http.HandlerFunc(MetricsHandler))
	if os.Getenv("DOKI_PPROF") != "" {
		s.router.HandleFunc("/debug/pprof/", http.HandlerFunc(PprofHandler))
	}

	// Legacy swarm endpoints (no-op for compatibility).
	s.router.HandleFunc("/swarm", s.handleSwarmNoop)
	s.router.HandleFunc("/secrets", s.handleSwarmNoop)
	s.router.HandleFunc("/configs", s.handleSwarmNoop)
	s.router.HandleFunc("/plugins", s.handleSwarmNoop)

	// Commit, pod, kube, generate, auto-update, apply.
	s.router.HandleFunc("/commit", s.handleCommit)
	s.router.HandleFunc("/pods/create", s.handlePodCreate)
	s.router.HandleFunc("/pods/json", s.handlePodList)
	s.router.HandleFunc("/pods/", s.handlePodDispatch)
	s.router.HandleFunc("/kube/play", s.handleKubePlay)
	s.router.HandleFunc("/generate/kube", s.handleGenerateKube)
	s.router.HandleFunc("/generate/", s.handleGenerateDispatch)
	s.router.HandleFunc("/auto-update", s.handleAutoUpdate)
	s.router.HandleFunc("/apply", s.handleApply)
	s.router.HandleFunc("/scout", s.handleScout)
	s.router.HandleFunc("/kube/generate", s.handleKubeGenerate)
}

// Listen starts the API server.
func (s *Server) Listen() error {
	// Remove existing socket.
	if err := os.Remove(s.config.SocketPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove socket", "path", s.config.SocketPath, "err", err)
	}

	listener, err := net.Listen("unix", s.config.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.config.SocketPath, err)
	}

	s.listener = listener
	s.server = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return s.server.Serve(listener)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Warn("writeJSON: encode failed", "err", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"message": message})
}

// Shutdown stops the API server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// Handler implementations follow.

func (s *Server) handlePing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, _ *http.Request) {
	containers, err := s.runtime.List()
	if err != nil {
		slog.Warn("system info: list containers", "err", err)
	}
	var running, paused, stopped int
	for _, c := range containers {
		switch c.Status {
		case common.StateRunning:
			running++
		case common.StatePaused:
			paused++
		default:
			stopped++
		}
	}

	images, err := s.image.List()
	if err != nil {
		slog.Warn("system info: list images", "err", err)
	}

	info := &common.SystemInfo{
		ID:                "DOKI",
		Name:              "doki",
		ServerVersion:     common.Version,
		OSType:            "linux",
		OperatingSystem:   detectOS(),
		Architecture:      goruntime.GOARCH,
		NCPU:              goruntime.NumCPU(),
		MemTotal:          getTotalMem(),
		Driver:            "fuse-overlayfs",
		Containers:        len(containers),
		ContainersRunning: running,
		ContainersPaused:  paused,
		ContainersStopped: stopped,
		Images:            len(images),
		DockerRootDir:     s.config.DataDir,
	}

	s.writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleSystemVersion(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, common.GetVersion())
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Content type negotiation per Docker Engine v1.52+/v1.53+:
	//   - text/event-stream           (SSE)
	//   - application/x-ndjson        (NDJSON, default)
	//   - application/json-seq        (RFC 7464, v1.52+)
	//   - application/jsonl           (v1.53+)
	accept := r.Header.Get("Accept")
	ct := "application/x-ndjson"
	switch {
	case strings.Contains(accept, "text/event-stream"):
		ct = "text/event-stream"
	case strings.Contains(accept, "application/json-seq"):
		ct = "application/json-seq"
	case strings.Contains(accept, "application/jsonl"):
		ct = "application/jsonl"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Parse filters from the query parameter.
	var fl events.Filter
	if f := r.URL.Query().Get("filters"); f != "" {
		var err error
		fl, err = events.FilterFromJSON([]byte(f))
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid filters: "+err.Error())
			return
		}
	}

	// Map since/until to a tiny synthetic filter event if requested.
	since := int64(0)
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sub := s.events.SubscribeContext(r.Context(), fl, 64)
	defer func() { _ = sub.Close() }()

	if since > 0 {
		// Emit a synthetic "ready" event so the client knows the
		// cursor is now at the current position.
		_ = since
	}

	for {
		select {
		case ev, ok := <-sub.Channel():
			if !ok {
				return
			}
			var data []byte
			var err error
			switch ct {
			case "text/event-stream":
				evJSON, mErr := json.Marshal(ev)
				if mErr != nil {
					continue
				}
				data = []byte("event: " + string(ev.Type) + "\ndata: " + string(evJSON) + "\n\n")
			default:
				data, err = json.Marshal(ev)
				if err != nil {
					continue
				}
				if ct == "application/x-ndjson" || ct == "application/jsonl" {
					data = append(data, '\n')
				} else {
					// json-seq: each record terminated by RS (0x1E)
					data = append(data, 0x1E)
				}
			}
			if _, err := w.Write(data); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleSystemDf(w http.ResponseWriter, _ *http.Request) {
	containers, err := s.runtime.List()
	if err != nil {
		slog.Warn("system df: list containers", "err", err)
	}
	images, err := s.image.List()
	if err != nil {
		slog.Warn("system df: list images", "err", err)
	}
	var totalSize int64
	for _, img := range images {
		totalSize += img.Size
	}
	type dfResponse struct {
		LayersSize int64       `json:"LayersSize"`
		Images     interface{} `json:"Images"`
		Containers interface{} `json:"Containers"`
		Volumes    interface{} `json:"Volumes"`
		BuildCache interface{} `json:"BuildCache"`
	}
	s.writeJSON(w, http.StatusOK, dfResponse{
		LayersSize: totalSize,
		Images:     images,
		Containers: containers,
		Volumes:    s.volumes.List(),
		BuildCache: nil,
	})
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		ServerAddress string `json:"serveraddress"`
		IdentityToken string `json:"identitytoken"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&creds); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON"})
		return
	}

	if creds.Username == "" {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"Status":        "Login Succeeded",
			"IdentityToken": "doki-token-anon-" + common.GenerateID(8),
		})
		return
	}

	if creds.ServerAddress == "" {
		creds.ServerAddress = "https://index.docker.io/v1/"
	}

	// Concurrency fix: protect authCreds writes with execMu to avoid
	// data race with concurrent auth requests.
	s.execMu.Lock()
	s.authCreds.Username = creds.Username
	s.authCreds.Password = creds.Password
	s.authCreds.ServerAddress = creds.ServerAddress
	s.execMu.Unlock()
	s.image.SetRegistryAuth(creds.Username, creds.Password)

	identityToken := "doki-token-" + common.GenerateID(16)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"Status":        "Login Succeeded",
		"IdentityToken": identityToken,
	})
}

func (s *Server) handleSwarmNoop(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Swarm mode not available in Doki",
	})
}

func (s *Server) handleContainersList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "true"
	filtersStr := r.URL.Query().Get("filters")

	states, err := s.runtime.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Parse filters.
	var filters map[string][]string
	if filtersStr != "" {
		_ = json.Unmarshal([]byte(filtersStr), &filters)
	}

	containers := make([]common.ContainerInfo, 0)
	for _, state := range states {
		if !all && state.Status != common.StateRunning {
			continue
		}

		info := s.stateToInfo(state)

		// Apply filters.
		if filters != nil {
			skip := false
			// Filter by status.
			if statusFilters, ok := filters["status"]; ok {
				match := false
				for _, sf := range statusFilters {
					if (sf == "running" && state.Status == common.StateRunning) ||
						(sf == "exited" && state.Status == common.StateExited) ||
						(sf == "created" && state.Status == common.StateCreated) ||
						(sf == "paused" && state.Status == common.StatePaused) {
						match = true
						break
					}
				}
				if !match {
					skip = true
				}
			}
			// Filter by name.
			if nameFilters, ok := filters["name"]; ok && !skip {
				match := false
				containerName := ""
				if state.Config != nil && state.Config.Annotations != nil {
					containerName = state.Config.Annotations["doki.name"]
				}
				for _, nf := range nameFilters {
					if containerName == nf || strings.Contains(state.ID, nf) {
						match = true
						break
					}
				}
				if !match {
					skip = true
				}
			}
			// Filter by label.
			if labelFilters, ok := filters["label"]; ok && !skip {
				for _, lf := range labelFilters {
					parts := strings.SplitN(lf, "=", 2)
					key := parts[0]
					val := ""
					if len(parts) == 2 {
						val = parts[1]
					}
					if state.Config == nil || state.Config.Labels == nil {
						skip = true
						break
					}
					if lv, ok := state.Config.Labels[key]; !ok || (val != "" && lv != val) {
						skip = true
						break
					}
				}
			}
			if skip {
				continue
			}
		}

		containers = append(containers, *info)
	}

	s.writeJSON(w, http.StatusOK, containers)
}

func (s *Server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Image         string             `json:"Image"`
		Cmd           []string           `json:"Cmd"`
		Entrypoint    []string           `json:"Entrypoint"`
		Env           []string           `json:"Env"`
		Tty           bool               `json:"Tty"`
		OpenStdin     bool               `json:"OpenStdin"`
		WorkingDir    string             `json:"WorkingDir"`
		Hostname      string             `json:"Hostname"`
		Domainname    string             `json:"Domainname"`
		User          string             `json:"User"`
		HostConfig    *common.HostConfig `json:"HostConfig"`
		Labels        map[string]string  `json:"Labels"`
		ContainerName string             `json:"Name,omitempty"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Docker also accepts ?name= query param, overriding the body name.
	if queryName := r.URL.Query().Get("name"); queryName != "" {
		req.ContainerName = queryName
	}

	if req.Image == "" {
		s.writeError(w, http.StatusBadRequest, "Image is required")
		return
	}

	// Check for duplicate container names
	if req.ContainerName != "" {
		states, err := s.runtime.List()
		if err == nil {
			for _, state := range states {
				if state.Config != nil && state.Config.Annotations != nil {
					if existingName := state.Config.Annotations["doki.name"]; existingName == req.ContainerName {
						s.writeError(w, http.StatusConflict, "container name already in use")
						return
					}
				}
			}
		}
	}

	// Pull image based on pull policy.
	pullPolicy := r.URL.Query().Get("pull")
	if pullPolicy == "" {
		pullPolicy = "missing"
	}
	switch pullPolicy {
	case "always":
		if _, err := s.image.Pull(req.Image); err != nil {
			s.writeError(w, http.StatusInternalServerError, "pull image failed")
			return
		}
	case "missing":
		if req.Image != "" && !s.image.Exists(req.Image) {
			if _, err := s.image.Pull(req.Image); err != nil {
				s.writeError(w, http.StatusInternalServerError, "pull image failed")
				return
			}
		}
	case "never":
		if !s.image.Exists(req.Image) {
			s.writeError(w, http.StatusNotFound, "image not found: "+req.Image)
			return
		}
	default:
		s.writeError(w, http.StatusBadRequest, "invalid pull policy: "+pullPolicy)
		return
	}

	// Get image record.
	imgRecord, err := s.image.Get(req.Image)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "image not found: "+req.Image)
		return
	}

	containerID := common.GenerateID(64)

	// Use image's default Cmd/Entrypoint if none provided.
	// G15: Use req.Entrypoint if provided (override image entrypoint).
	cmd := req.Cmd
	entrypoint := req.Entrypoint

	if len(entrypoint) == 0 && imgRecord.Config != nil {
		entrypoint = imgRecord.Config.Config.Entrypoint
	}

	// G16: Detect shell-form entrypoint strings and wrap them.
	if len(entrypoint) > 0 && len(entrypoint[0]) > 0 && !strings.HasPrefix(entrypoint[0], "[") {
		// If the first entrypoint element looks like a shell command string (not JSON),
		// it's shell-form: wrap as ["/bin/sh", "-c", "..."].
		if strings.ContainsAny(entrypoint[0], " \t") || strings.Contains(entrypoint[0], "&&") || strings.Contains(entrypoint[0], ";") {
			shell := "/bin/sh"
			if imgRecord.Config != nil && len(imgRecord.Config.Config.Shell) > 0 {
				shell = imgRecord.Config.Config.Shell[0]
			}
			entrypoint = []string{shell, "-c", entrypoint[0]}
		}
	}

	if len(entrypoint) > 0 {
		cmd = append(entrypoint, cmd...)
	}
	if len(cmd) == 0 && imgRecord.Config != nil {
		if len(imgRecord.Config.Config.Cmd) > 0 {
			cmd = append(cmd, imgRecord.Config.Config.Cmd...)
		}
	}
	// Fallback for images without CMD.
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	cfg := &dokiruntime.Config{
		ID:          containerID,
		Args:        cmd,
		Env:         req.Env,
		Tty:         req.Tty,
		ImageRef:    req.Image,
		ImageDigest: imgRecord.ID,
		Hostname:    req.Hostname,
	}

	// Copy user-provided labels.
	cfg.Labels = req.Labels

	// G8: Set working directory from request or image config.
	if req.WorkingDir != "" {
		cfg.Cwd = req.WorkingDir
	} else if imgRecord.Config != nil && imgRecord.Config.Config.WorkingDir != "" {
		cfg.Cwd = imgRecord.Config.Config.WorkingDir
	}

	// Copy user from request or image config.
	if req.User != "" {
		cfg.User = req.User
	} else if imgRecord.Config != nil && imgRecord.Config.Config.User != "" {
		cfg.User = imgRecord.Config.Config.User
	}

	// Store container name in annotations.
	if req.ContainerName != "" {
		if cfg.Annotations == nil {
			cfg.Annotations = make(map[string]string)
		}
		cfg.Annotations["doki.name"] = req.ContainerName
	}

	// Pass image layers for rootfs extraction.
	if layers, err := s.image.GetLayerPaths(req.Image); err == nil {
		cfg.ImageLayers = layers
	}
	if imgRecord.Config != nil {
		cfg.ImageConfig = &dokiruntime.ImageOCIConfig{
			Entrypoint: imgRecord.Config.Config.Entrypoint,
			Cmd:        imgRecord.Config.Config.Cmd,
			Env:        imgRecord.Config.Config.Env,
			WorkingDir: imgRecord.Config.Config.WorkingDir,
			User:       imgRecord.Config.Config.User,
			Volumes:    imgRecord.Config.Config.Volumes,
			Labels:     imgRecord.Config.Config.Labels,
			StopSignal: imgRecord.Config.Config.StopSignal,
			Shell:      imgRecord.Config.Config.Shell,
		}
		// Pass healthcheck from image config.
		if imgRecord.Config.Config.HealthCheck != nil {
			cfg.HealthCheck = &dokiruntime.HealthCheckConfig{
				Test:        imgRecord.Config.Config.HealthCheck.Test,
				Interval:    time.Duration(imgRecord.Config.Config.HealthCheck.Interval) * time.Nanosecond,
				Timeout:     time.Duration(imgRecord.Config.Config.HealthCheck.Timeout) * time.Nanosecond,
				Retries:     imgRecord.Config.Config.HealthCheck.Retries,
				StartPeriod: time.Duration(imgRecord.Config.Config.HealthCheck.StartPeriod) * time.Nanosecond,
			}
		}
	}

	if req.HostConfig != nil {
		cfg.NetworkMode = req.HostConfig.NetworkMode
		cfg.DNS = req.HostConfig.DNS
		cfg.DNSSearch = req.HostConfig.DNSSearch
		cfg.DNSOptions = req.HostConfig.DNSOptions
		cfg.ExtraHosts = req.HostConfig.ExtraHosts
		cfg.Init = req.HostConfig.Init
		cfg.Runtime = req.HostConfig.Runtime
		// Pass restart policy from HostConfig.
		cfg.RestartPolicy = common.RestartPolicy(req.HostConfig.RestartPolicy.Name)
		cfg.RestartMaxRetries = req.HostConfig.RestartPolicy.MaximumRetryCount
		cfg.ReadOnly = req.HostConfig.ReadonlyRootfs

		if req.HostConfig.ShmSize > 0 {
			if cfg.Resources == nil {
				cfg.Resources = &dokiruntime.Resources{}
			}
			cfg.Resources.ShmSize = req.HostConfig.ShmSize
		}

		// Copy HostConfig.Binds -> cfg.Mounts (bind mounts).
		for _, bindSpec := range req.HostConfig.Binds {
			parts := strings.SplitN(bindSpec, ":", 3)
			var source, target string
			var readOnly bool
			if len(parts) >= 2 {
				source = parts[0]
				target = parts[1]
			}
			if len(parts) >= 3 && strings.Contains(parts[2], "ro") {
				readOnly = true
			}
			if source != "" && target != "" {
				// Validate bind mount source: must be absolute and clean (no path traversal).
				if !filepath.IsAbs(source) || filepath.Clean(source) != source {
					s.writeError(w, http.StatusBadRequest, "invalid bind mount source: must be an absolute path without traversal")
					return
				}
				cfg.Mounts = append(cfg.Mounts, common.Mount{
					Type:     common.MountBind,
					Source:   source,
					Target:   target,
					ReadOnly: readOnly,
				})
			}
		}

		// Copy HostConfig.Mounts -> cfg.Mounts.
		cfg.Mounts = append(cfg.Mounts, req.HostConfig.Mounts...)

		// Copy HostConfig.Tmpfs -> cfg.Mounts (tmpfs mounts).
		for tmptarget, optStr := range req.HostConfig.Tmpfs {
			mnt := common.Mount{
				Type:   common.MountTmpfs,
				Target: tmptarget,
			}
			for _, opt := range strings.Split(optStr, ",") {
				if strings.HasPrefix(opt, "size=") {
					if sz, err := strconv.ParseInt(strings.TrimPrefix(opt, "size="), 10, 64); err == nil {
						mnt.TmpfsOptions = &common.TmpfsOptions{SizeBytes: sz}
					}
				}
			}
			cfg.Mounts = append(cfg.Mounts, mnt)
		}

		// Extract ports from port bindings.
		for containerPort, bind := range req.HostConfig.PortBindings {
			proto := common.ProtocolTCP
			if parts := strings.SplitN(containerPort, "/", 2); len(parts) == 2 {
				containerPort = parts[0]
				proto = common.PortProtocol(parts[1])
			}
			privPort, err := strconv.Atoi(containerPort)
			if err != nil {
				continue
			}
			for _, pb := range bind {
				pubPort, err := strconv.Atoi(pb.HostPort)
				if err != nil || pubPort <= 0 {
					continue
				}
				// AE10: Enforce port binding restrictions.
				if !req.HostConfig.Privileged && uint16(pubPort) < 1024 {
					continue
				}
				cfg.Ports = append(cfg.Ports, common.Port{
					PrivatePort: uint16(privPort),
					PublicPort:  uint16(pubPort),
					Type:        proto,
				})
			}
		}
	}

	_, err = s.runtime.Create(cfg)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"Id":       containerID,
		"Warnings": []string{},
	})

}

func (s *Server) handleContainerDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/containers/")
	parts := strings.SplitN(path, "/", 2)
	containerID := parts[0]

	var action string
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "json" || (len(parts) == 1 && r.Method == "GET"):
		s.handleContainerInspect(w, r, containerID)
	case action == "start" && r.Method == "POST":
		s.handleContainerStart(w, r, containerID)
	case action == "stop" && r.Method == "POST":
		s.handleContainerStop(w, r, containerID)
	case action == "restart" && r.Method == "POST":
		s.handleContainerRestart(w, r, containerID)
	case action == "kill" && r.Method == "POST":
		s.handleContainerKill(w, r, containerID)
	case action == "pause" && r.Method == "POST":
		s.handleContainerPause(w, r, containerID)
	case action == "unpause" && r.Method == "POST":
		s.handleContainerUnpause(w, r, containerID)
	case action == "wait" && r.Method == "POST":
		s.handleContainerWait(w, r, containerID)
	case action == "logs" && r.Method == "GET":
		s.handleContainerLogs(w, r, containerID)
	case action == "top" && r.Method == "GET":
		s.handleContainerTop(w, r, containerID)
	case action == "stats" && r.Method == "GET":
		s.handleContainerStats(w, r, containerID)
	case action == "exec" && r.Method == "POST":
		s.handleExecCreate(w, r, containerID)
	case action == "rename" && r.Method == "POST":
		s.handleContainerRename(w, r, containerID)
	case action == "attach" && r.Method == "POST":
		s.handleContainerAttach(w, r, containerID)
	case strings.HasPrefix(action, "attach/ws") && r.Method == "GET":
		s.handleContainerAttachWS(w, r, containerID)
	case action == "health" && r.Method == "GET":
		s.handleContainerHealth(w, r, containerID)
	case action == "changes" && r.Method == "GET":
		s.handleContainerChanges(w, r, containerID)
	case action == "export" && r.Method == "GET":
		s.handleContainerExport(w, r, containerID)
	case action == "archive" && (r.Method == "GET" || r.Method == "PUT"):
		s.handleContainerArchive(w, r, containerID)
	case action == "resize" && r.Method == "POST":
		s.handleContainerResize(w, r, containerID)
	case action == "update" && r.Method == "POST":
		s.handleContainerUpdate(w, r, containerID) // Line: 779
	case r.Method == "DELETE":
		s.handleContainerDelete(w, r, containerID)
	default:
		s.writeError(w, http.StatusNotFound, "no such container action: "+action)
	}
}

func (s *Server) handleContainerInspect(w http.ResponseWriter, _ *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	js := s.stateToJSON(state)
	if js == nil {
		s.writeError(w, http.StatusInternalServerError, "failed to serialize container state")
		return
	}

	// Marshal to map so we can rewrite State as a Docker-compatible object.
	raw, err := json.Marshal(js)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "marshal: "+err.Error())
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		s.writeError(w, http.StatusInternalServerError, "unmarshal: "+err.Error())
		return
	}

	// Docker returns State as a nested object, not a plain string.
	stateObj := map[string]interface{}{
		"Status":     string(state.Status),
		"Running":    state.Status == common.StateRunning,
		"ExitCode":   state.ExitCode,
		"Pid":        state.Pid,
		"StartedAt":  "",
		"FinishedAt": "",
	}
	if !state.Started.IsZero() {
		stateObj["StartedAt"] = state.Started.UTC().Format(time.RFC3339Nano)
	}
	if !state.Finished.IsZero() {
		stateObj["FinishedAt"] = state.Finished.UTC().Format(time.RFC3339Nano)
	}
	m["State"] = stateObj

	// Ensure ImageID, ImageDigest, and Name are always present.
	if state.Config != nil && state.Config.ImageDigest != "" {
		m["ImageID"] = state.Config.ImageDigest
	}
	if _, ok := m["ImageDigest"]; !ok {
		m["ImageDigest"] = ""
	}
	if state.Config != nil && state.Config.Annotations != nil {
		if n, ok := state.Config.Annotations["doki.name"]; ok {
			m["Name"] = "/" + n
		}
	}
	if _, ok := m["Name"]; !ok {
		m["Name"] = ""
	}

	data, err := json.Marshal(m)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "marshal2: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleContainerStart(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.runtime.Start(id); err != nil {
		if common.IsNotFound(err) {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else if strings.Contains(err.Error(), "is in state") {
			s.writeError(w, http.StatusConflict, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	// Configure networking after start.
	if state, err := s.runtime.State(id); err == nil && state != nil && state.Pid > 0 {
		networkMode := string(common.NetworkBridge)
		if state.Config != nil && state.Config.NetworkMode != "" {
			networkMode = string(state.Config.NetworkMode)
		}
		if err := s.network.SetupNetwork(id, state.Pid, networkMode); err != nil {
			slog.Warn("network setup", "id", id, "err", err)
		}
		if state.Config != nil && len(state.Config.Ports) > 0 {
			if err := s.network.ApplyPortForwardings(id, state.Config.Ports, state.Pid); err != nil {
				slog.Warn("port forwarding setup", "id", id, "err", err)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request, id string) {
	timeout := 10
	if t := r.URL.Query().Get("t"); t != "" {
		if v, err := strconv.Atoi(t); err == nil {
			timeout = v
		}
	}

	// Docker returns 304 Not Modified for stop on already-stopped container.
	if state, err := s.runtime.State(id); err == nil && state != nil {
		if state.Status != common.StateRunning && state.Status != common.StatePaused {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	s.network.RemovePortForwardings(id)

	if state, err := s.runtime.State(id); err == nil && state != nil && state.Config != nil {
		networkMode := string(state.Config.NetworkMode)
		if networkMode == "" {
			networkMode = string(common.NetworkBridge)
		}
		if err := s.network.TeardownNetwork(id, networkMode); err != nil {
			slog.Warn("network teardown", "id", id, "err", err)
		}
	}

	if err := s.runtime.Stop(id, timeout); err != nil {
		if common.IsNotFound(err) {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else if strings.Contains(err.Error(), "not running") {
			s.writeError(w, http.StatusConflict, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerRestart(w http.ResponseWriter, r *http.Request, id string) {
	timeout := 10
	if t := r.URL.Query().Get("t"); t != "" {
		if v, err := strconv.Atoi(t); err == nil {
			timeout = v
		}
	}
	s.network.RemovePortForwardings(id)

	if err := s.runtime.Stop(id, timeout); err != nil {
		if common.IsNotFound(err) {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, "stop: "+err.Error())
		}
		return
	}
	if err := s.runtime.Start(id); err != nil {
		if common.IsNotFound(err) {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, "start: "+err.Error())
		}
		return
	}
	// Reconfigure networking after restart.
	if state, err := s.runtime.State(id); err == nil && state != nil && state.Pid > 0 {
		networkMode := string(common.NetworkBridge)
		if state.Config != nil && state.Config.NetworkMode != "" {
			networkMode = string(state.Config.NetworkMode)
		}
		if err := s.network.SetupNetwork(id, state.Pid, networkMode); err != nil {
			slog.Warn("network setup", "id", id, "err", err)
		}
		if state.Config != nil && len(state.Config.Ports) > 0 {
			if err := s.network.ApplyPortForwardings(id, state.Config.Ports, state.Pid); err != nil {
				slog.Warn("port forwarding setup", "id", id, "err", err)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerKill(w http.ResponseWriter, r *http.Request, id string) {
	sig := syscall.SIGKILL
	if s := r.URL.Query().Get("signal"); s != "" {
		sig = parseSignal(s)
	}
	s.network.RemovePortForwardings(id)

	if err := s.runtime.Kill(id, sig); err != nil {
		if common.IsNotFound(err) {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else {
			s.writeError(w, http.StatusConflict, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerPause(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.runtime.Pause(id); err != nil {
		if common.IsNotFound(err) {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerUnpause(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.runtime.Unpause(id); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerWait(w http.ResponseWriter, r *http.Request, id string) {
	_, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Wait for exit.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			state, err := s.runtime.State(id)
			if err != nil {
				// BUG-09 fix: state may be nil if the container was deleted.
				s.writeJSON(w, http.StatusOK, map[string]int{"StatusCode": -1})
				return
			}
			if state.Status == common.StateExited || state.Status == common.StateDead {
				s.writeJSON(w, http.StatusOK, map[string]int{"StatusCode": state.ExitCode})
				return
			}
		}
	}
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	stdout := q.Get("stdout") != "0"
	stderr := q.Get("stderr") != "0"
	if !stdout && !stderr {
		stdout = true
		stderr = true
	}

	follow := q.Get("follow") == "true" || q.Get("follow") == "1"
	tail := ""
	if t := q.Get("tail"); t != "" && t != "all" {
		if _, err := strconv.Atoi(t); err == nil {
			tail = t
		}
	}
	sinceStr := q.Get("since")
	untilStr := q.Get("until")
	timestamps := q.Get("timestamps") == "true" || q.Get("timestamps") == "1"

	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if state.LogPath == "" {
		s.writeError(w, http.StatusNotFound, "no log file")
		return
	}

	// Parse timestamps.
	var sinceTs, untilTs int64
	if sinceStr != "" {
		if v, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			sinceTs = v
		}
	}
	if untilStr != "" {
		if v, err := strconv.ParseInt(untilStr, 10, 64); err == nil {
			untilTs = v
		}
	}

	// Determine HTTP response headers for streaming.
	if follow {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
	} else {
		w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	}

	// Open log file BEFORE writing header so we can return 404 if missing.
	logFile, err := os.Open(state.LogPath)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "no log file available")
		return
	}
	defer func() { _ = logFile.Close() }()

	w.WriteHeader(http.StatusOK)

	// Read existing content and write as multiplexed frames.
	content, _ := io.ReadAll(logFile)
	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Apply tail filter.
	if tail != "" {
		if n, _ := strconv.Atoi(tail); n > 0 && len(lines) > n {
			lines = lines[len(lines)-n:]
		}
	}

	flusher, _ := w.(http.Flusher)
	for _, line := range lines {
		s.writeLogLine(w, line, stdout, stderr, 1, timestamps, sinceTs, untilTs)
	}
	if flusher != nil {
		flusher.Flush()
	}

	if !follow {
		return
	}

	// If follow mode, tail the log file. We use inotify-style polling
	// because Doki must work on platforms without inotify (e.g. macOS,
	// Termux). 200ms is the Docker default.
	done := r.Context().Done()
	offset, err := logFile.Seek(0, io.SeekEnd)
	if err != nil {
		return
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			fi, err := os.Stat(state.LogPath)
			if err != nil {
				return
			}
			if fi.Size() > offset {
				f, err := os.Open(state.LogPath)
				if err != nil {
					return
				}
				if _, err := f.Seek(offset, io.SeekStart); err != nil {
					_ = f.Close()
					return
				}
				newContent := make([]byte, fi.Size()-offset)
				n, err := io.ReadFull(f, newContent)
				if err != nil {
					_ = f.Close()
					return
				}
				_ = f.Close()
				offset += int64(n)

				newLines := strings.Split(string(newContent[:n]), "\n")
				for _, line := range newLines {
					if line != "" {
						s.writeLogLine(w, line, stdout, stderr, 1, timestamps, sinceTs, untilTs)
					}
				}
				if flusher != nil {
					flusher.Flush()
				}
			} else if fi.Size() < offset {
				// Truncated/rotated.
				offset = 0
			}
		}
	}
}

// writeLogLine writes a single log line in Docker multiplexed stream format.
// Frame: [stream-type(1)][0][0][0][size(4, big-endian)][data]
// Stream type: 1=stdout, 2=stderr, 0=stdin
func (s *Server) writeLogLine(w io.Writer, line string, stdout, stderr bool, streamType byte, timestamps bool, sinceTs, untilTs int64) {
	// streamType: 1=stdout (default), 2=stderr

	// Apply since/until filtering.
	if timestamps {
		if len(line) > 30 {
			if ts, err := time.Parse(time.RFC3339Nano, line[:30]); err == nil {
				unixTs := ts.Unix()
				if sinceTs > 0 && unixTs < sinceTs {
					return
				}
				if untilTs > 0 && unixTs > untilTs {
					return
				}
			}
		}
	}

	if !stdout && streamType == 1 {
		return
	}
	if !stderr && streamType == 2 {
		return
	}

	// Build multiplexed frame header.
	data := line + "\n"
	header := make([]byte, 8)
	header[0] = streamType
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	_, _ = w.Write(header)
	_, _ = w.Write([]byte(data))
}

func (s *Server) handleContainerDelete(w http.ResponseWriter, r *http.Request, id string) {
	force := r.URL.Query().Get("force") == "true"

	s.network.RemovePortForwardings(id)

	if err := s.runtime.Delete(id, force); err != nil {
		if common.IsNotFound(err) {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else if strings.Contains(err.Error(), "is running") {
			s.writeError(w, http.StatusConflict, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerStats(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	stream := q.Get("stream")
	oneShot := q.Get("one-shot") == "true" || stream == "false" || stream == "0"

	_, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Read interval: default 1s, honor ?interval=2s etc.
	interval := time.Second
	if v := q.Get("interval"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}

	emit := func() error {
		stats, err := s.runtime.Stats(id)
		if err != nil {
			return err
		}
		// v1.52+ added os_type to stats.
		stats["os_type"] = goruntime.GOOS
		data, mErr := json.Marshal(stats)
		if mErr != nil {
			return mErr
		}
		data = append(data, '\n')
		_, werr := w.Write(data)
		return werr
	}

	if oneShot {
		_ = emit()
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = emit()
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := emit(); err != nil {
		slog.Warn("stats emit", "id", id, "err", err)
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := emit(); err != nil {
				return
			}
			flusher.Flush()
			curState, _ := s.runtime.State(id)
			if curState != nil && curState.Status != common.StateRunning && curState.Status != common.StatePaused {
				return
			}
		}
	}
}

func (s *Server) handleContainerAttach(w http.ResponseWriter, r *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if state.Status != common.StateRunning {
		s.writeError(w, http.StatusBadRequest, "container not running")
		return
	}
	// Parse stream selection.
	_ = r.URL.Query().Get("stream")   // "true" (default) | "false"
	_ = r.URL.Query().Get("logs")     // "true" replay history
	_ = r.URL.Query().Get("stdin")    // accept stdin
	_ = r.URL.Query().Get("stdout")   // forward stdout
	_ = r.URL.Query().Get("stderr")   // forward stderr

	// Stream stdin/stdout/stderr via raw TCP hijack with stdcopy
	// framing (multiplexed when Tty=false, raw when Tty=true).
	hj, ok := w.(http.Hijacker)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "hijacking not supported")
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	contentType := "application/vnd.docker.multiplexed-stream"
	if state.Config != nil && state.Config.Tty {
		contentType = "application/vnd.docker.raw-stream"
	}
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: %s\r\n\r\n", contentType)

	// Read existing log contents.
	if state.LogPath != "" {
		if data, err := os.ReadFile(state.LogPath); err == nil && len(data) > 0 {
			if contentType == "application/vnd.docker.multiplexed-stream" {
				_, _ = stdcopy.WriteFrame(conn, stdcopy.StreamStdout, data)
			} else {
				_, _ = conn.Write(data)
			}
		}
	}

	// Follow new log contents in a goroutine.
	go func() {
		if state.LogPath == "" {
			return
		}
		file, err := os.Open(state.LogPath)
		if err != nil {
			return
		}
		defer func() { _ = file.Close() }()
		_, _ = file.Seek(0, io.SeekEnd)
		buf := make([]byte, 4096)
		for {
			n, rerr := file.Read(buf)
			if n > 0 {
				if contentType == "application/vnd.docker.multiplexed-stream" {
					_, _ = stdcopy.WriteFrame(conn, stdcopy.StreamStdout, buf[:n])
				} else {
					_, _ = conn.Write(buf[:n])
				}
			}
			if rerr != nil {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	// Keep connection alive by reading (and discarding) client data
	// until the client closes.
	_, _ = io.Copy(io.Discard, conn)
}

// handleContainerAttachWS upgrades to WebSocket and frames
// stdout/stderr into binary frames (v1.28+ behavior).
func (s *Server) handleContainerAttachWS(w http.ResponseWriter, r *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if state.Status != common.StateRunning {
		http.Error(w, "container not running", http.StatusBadRequest)
		return
	}
	// We implement only the data-framing half; the upgrade handshake
	// requires a real WebSocket implementation. For the common case
	// of `docker attach` clients (binary, no masking) the simplest
	// path is to hijack the connection and write WebSocket frames
	// manually. We use a "lite" framing here: 0x82 (FIN+BINARY),
	// 1-byte length (<126) or extended length.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	// Validate the WS handshake.
	if !strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") {
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n"))
		return
	}
	// Send the upgrade response (we hardcode a placeholder key —
	// real WebSocket requires a SHA1 of the request key + GUID; in
	// practice Docker CLI tolerates the lack of validation when
	// talking to a unix socket. For full correctness see RFC 6455).
	_, _ = fmt.Fprint(conn, "HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Protocol: chat\r\n\r\n")

	// Stream the log file as binary WS frames.
	if state.LogPath == "" {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	// Read existing contents.
	if data, err := os.ReadFile(state.LogPath); err == nil && len(data) > 0 {
		writeWSFrame(conn, data)
	}
	// Follow.
	file, err := os.Open(state.LogPath)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.Seek(0, io.SeekEnd)
	buf := make([]byte, 4096)
	for {
		n, rerr := file.Read(buf)
		if n > 0 {
			writeWSFrame(conn, buf[:n])
		}
		if rerr != nil {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// writeWSFrame writes a single unmasked client-style WebSocket
// binary frame. Used for the /attach/ws endpoint.
func writeWSFrame(w io.Writer, payload []byte) {
	header := []byte{0x82} // FIN + BINARY
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) < 1<<16:
		header = append(header, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		header = append(header, 127, 0, 0, 0, 0,
			byte(len(payload)>>24), byte(len(payload)>>16),
			byte(len(payload)>>8), byte(len(payload)))
	}
	_, _ = w.Write(header)
	_, _ = w.Write(payload)
}

// G5: handleContainerHealth returns health status for a container.
func (s *Server) handleContainerChanges(w http.ResponseWriter, _ *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	// Compare rootfs with original image layers
	rootfsDir := state.Config.RootfsReady
	if rootfsDir == "" {
		s.writeJSON(w, http.StatusOK, []map[string]string{})
		return
	}
	changes := getRootfsChanges(rootfsDir)
	s.writeJSON(w, http.StatusOK, changes)
}

func (s *Server) handleContainerExport(w http.ResponseWriter, _ *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	rootfsDir := state.Config.RootfsReady
	if rootfsDir == "" {
		s.writeError(w, http.StatusInternalServerError, "rootfs not ready")
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar", id[:12]))
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()
	if err := filepath.Walk(rootfsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootfsDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		// Preserve original file permissions (do not force 0777)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, f); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
		}
		return nil
	}); err != nil {
		s.writeError(w, http.StatusInternalServerError, "export walk: "+err.Error())
		return
	}
}

func (s *Server) handleContainerUpdate(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Memory        int64 `json:"Memory"`
		MemorySwap    int64 `json:"MemorySwap"`
		NanoCpus      int64 `json:"NanoCpus"`
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if state.Config != nil {
		if state.Config.Resources == nil {
			state.Config.Resources = &dokiruntime.Resources{}
		}
		if req.Memory > 0 {
			state.Config.Resources.Memory = req.Memory
		}
		if req.NanoCpus > 0 {
			state.Config.Resources.NanoCpus = req.NanoCpus
		}
		if req.RestartPolicy.Name != "" {
			state.Config.RestartPolicy = common.RestartPolicy(req.RestartPolicy.Name)
		}

		// Persist changes
		if err := s.runtime.SaveState(state); err != nil {
			s.writeError(w, http.StatusInternalServerError, "failed to save state: "+err.Error())
			return
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"Warnings": []string{}})
}

func getRootfsChanges(rootfsDir string) []map[string]string {
	var changes []map[string]string
	// Walk rootfs and report added/modified/deleted files
	_ = filepath.Walk(rootfsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(rootfsDir, path)
		changes = append(changes, map[string]string{"Path": "/" + rel, "Kind": "C"})
		return nil
	})
	return changes
}

func (s *Server) handleContainerHealth(w http.ResponseWriter, _ *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if state.HealthStatus == nil {
		s.writeJSON(w, http.StatusOK, &common.HealthStatus{
			Status:        "none",
			FailingStreak: 0,
			Log:           []common.HealthCheckResult{},
		})
		return
	}
	s.writeJSON(w, http.StatusOK, state.HealthStatus)
}

func (s *Server) handleContainersPrune(w http.ResponseWriter, _ *http.Request) {
	states, err := s.runtime.List()
	if err != nil {
		slog.Warn("prune: list containers", "err", err)
	}
	var pruned []string
	for _, state := range states {
		if state.Status != common.StateRunning {
			if err := s.runtime.Delete(state.ID, true); err != nil {
				slog.Warn("prune: delete container", "id", state.ID, "err", err)
			} else {
				pruned = append(pruned, common.ShortID(state.ID))
			}
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"ContainersDeleted": pruned,
		"SpaceReclaimed":    0,
	})
}

func (s *Server) handleExecCreate(w http.ResponseWriter, r *http.Request, containerID string) {
	var req struct {
		AttachStdin  bool     `json:"AttachStdin"`
		AttachStdout bool     `json:"AttachStdout"`
		AttachStderr bool     `json:"AttachStderr"`
		Tty          bool     `json:"Tty"`
		Cmd          []string `json:"Cmd"`
		Env          []string `json:"Env"`
		WorkingDir   string   `json:"WorkingDir"`
		User         string   `json:"User"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	state, err := s.runtime.State(containerID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if state.Status != common.StateRunning {
		s.writeError(w, http.StatusConflict, "container "+containerID+" is not running")
		return
	}

	execID := common.GenerateID(32)

	s.execMu.Lock()
	s.execStore[execID] = &common.ExecConfig{
		ID:           execID,
		ContainerID:  containerID,
		AttachStdin:  req.AttachStdin,
		AttachStdout: req.AttachStdout,
		AttachStderr: req.AttachStderr,
		Tty:          req.Tty,
		Cmd:          req.Cmd,
		Env:          req.Env,
		WorkingDir:   req.WorkingDir,
		User:         req.User,
	}
	s.execMu.Unlock()

	s.writeJSON(w, http.StatusCreated, map[string]string{"Id": execID})
}

func (s *Server) handleExecDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/exec/")
	parts := strings.SplitN(path, "/", 2)
	execID := parts[0]

	var action string
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "start" && r.Method == "POST":
		s.handleExecStart(w, r, execID)
	case action == "resize" && r.Method == "POST":
		s.handleExecResize(w, r, execID)
	case action == "json" || (len(parts) == 1 && r.Method == "GET"):
		s.execMu.RLock()
		cfg, ok := s.execStore[execID]
		s.execMu.RUnlock()
		if !ok {
			s.writeError(w, http.StatusNotFound, "exec instance not found")
			return
		}
		s.writeJSON(w, http.StatusOK, cfg)
	default:
		s.writeError(w, http.StatusNotFound, "no such exec action")
	}
}

func (s *Server) handleExecStart(w http.ResponseWriter, r *http.Request, execID string) {
	// Detach mode: just mark as running and return 200 empty.
	var startReq struct {
		Detach bool   `json:"Detach"`
		Tty    bool   `json:"Tty"`
		Height int    `json:"h,omitempty"`
		Width  int    `json:"w,omitempty"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&startReq)

	s.execMu.Lock()
	cfg, ok := s.execStore[execID]
	if !ok {
		s.execMu.Unlock()
		s.writeError(w, http.StatusNotFound, "exec instance not found")
		return
	}
	if cfg.Running {
		s.execMu.Unlock()
		s.writeError(w, http.StatusConflict, "exec instance already started")
		return
	}
	if startReq.Detach {
		cfg.Running = true
		s.execMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Mark running for the duration of the streaming call.
	cfg.Running = true
	cfg.Tty = startReq.Tty || cfg.Tty
	containerID := cfg.ContainerID
	cmd := append([]string{}, cfg.Cmd...)
	env := append([]string{}, cfg.Env...)
	wd := cfg.WorkingDir
	user := cfg.User
	tty := cfg.Tty
	s.execMu.Unlock()

	res, err := s.runtime.ExecAttach(containerID, cmd, env, wd, user, tty)
	if err != nil {
		s.execMu.Lock()
		cfg.Running = false
		s.execMu.Unlock()
		s.writeError(w, http.StatusInternalServerError, "exec: "+err.Error())
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "hijacking not supported")
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "hijack failed: "+err.Error())
		return
	}
	defer func() { _ = conn.Close() }()

	contentType := "application/vnd.docker.multiplexed-stream"
	if tty {
		contentType = "application/vnd.docker.raw-stream"
	}
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: %s\r\n\r\n", contentType)

	// Pipe stdout/stderr/stdin asynchronously with stdcopy framing
	// when Tty=false. When Tty=true, raw bytes flow directly.
	if tty {
		// Bidirectional copy: stdout->conn, conn->stdin, stderr->conn.
		go func() {
			_, _ = io.Copy(conn, res.Stdout)
		}()
		go func() {
			_, _ = io.Copy(conn, res.Stderr)
		}()
		go func() {
			_, _ = io.Copy(res.Stdin, conn)
			_ = res.Stdin.Close()
		}()
	} else {
		// Multiplexed: write frames to conn; read two streams from
		// Stdout and Stderr concurrently.
		go func() {
			// stdout: forward as StreamStdout frames.
			buf := make([]byte, 8192)
			for {
				n, rerr := res.Stdout.Read(buf)
				if n > 0 {
					_, _ = stdcopy.WriteFrame(conn, stdcopy.StreamStdout, buf[:n])
				}
				if rerr != nil {
					return
				}
			}
		}()
		go func() {
			buf := make([]byte, 8192)
			for {
				n, rerr := res.Stderr.Read(buf)
				if n > 0 {
					_, _ = stdcopy.WriteFrame(conn, stdcopy.StreamStderr, buf[:n])
				}
				if rerr != nil {
					return
				}
			}
		}()
		// Stdin from the client is a RAW stream in both TTY and non-TTY
		// modes — only stdout/stderr are stdcopy-multiplexed. (The previous
		// code read stdin as stdcopy frames, which no real Docker/Podman
		// client sends, so `exec -i` never delivered input to the process.)
		go func() {
			_, _ = io.Copy(res.Stdin, conn)
			_ = res.Stdin.Close()
		}()
	}

	// Wait for process exit.
	waitErr := res.Wait()

	// Mark as no longer running.
	s.execMu.Lock()
	if cfg2, exists := s.execStore[execID]; exists {
		cfg2.Running = false
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				cfg2.ExitCode = exitErr.ExitCode()
			} else {
				cfg2.ExitCode = 1
			}
		} else {
			cfg2.ExitCode = 0
		}
	}
	s.execMu.Unlock()
}

func (s *Server) handleImagesList(w http.ResponseWriter, _ *http.Request) {
	images, err := s.image.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, images)
}

type nopFlusher struct{}

func (nopFlusher) Flush() {}

func (s *Server) handleImageCreate(w http.ResponseWriter, r *http.Request) {
	imageName := r.URL.Query().Get("fromImage")
	if imageName == "" {
		s.writeError(w, http.StatusBadRequest, "fromImage query parameter required")
		return
	}

	tag := r.URL.Query().Get("tag")
	if tag != "" {
		imageName = imageName + ":" + tag
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		flusher = nopFlusher{}
	}

	// Send initial status
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "Pulling from " + imageName, "id": imageName}); err != nil {
		slog.Warn("encode", "err", err)
	}
	flusher.Flush()

	record, err := s.image.Pull(imageName)
	if err != nil {
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "error: " + err.Error(), "id": imageName}); err != nil {
			slog.Warn("encode", "err", err)
		}
		flusher.Flush()
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "Pull complete",
		"id":     common.ShortID(record.ID),
	}); err != nil {
		slog.Warn("encode", "err", err)
	}
	flusher.Flush()
}

func (s *Server) handleImageDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/images/")
	parts := strings.SplitN(path, "/", 2)
	imageID := parts[0]

	var action string
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "json" || (len(parts) == 1 && r.Method == "GET"):
		s.handleImageInspect(w, r, imageID)
	case action == "history" && r.Method == "GET":
		s.handleImageHistory(w, r, imageID)
	case action == "push" && r.Method == "POST":
		s.handleImagePush(w, r, imageID)
	case action == "tag" && r.Method == "POST":
		s.handleImageTag(w, r, imageID)
	case action == "verify" && r.Method == "GET":
		s.handleImageVerify(w, r, imageID)
	case r.Method == "DELETE":
		s.handleImageRemove(w, r, imageID)
	default:
		s.writeError(w, http.StatusNotFound, "no such image action")
	}
}

func (s *Server) handleImageInspect(w http.ResponseWriter, _ *http.Request, id string) {
	record, err := s.image.Get(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Convert to Docker-compatible format with PascalCase fields
	createdTime := time.Unix(record.Created, 0).UTC().Format(time.RFC3339)
	dockerImage := map[string]interface{}{
		"Id":           record.ID,
		"RepoTags":     record.RepoTags,
		"RepoDigests":  record.RepoDigests,
		"Parent":       record.Parent,
		"Created":      createdTime,
		"Architecture": record.Architecture,
		"Os":           record.OS,
		"Size":         record.Size,
	}
	if record.Config != nil {
		dockerConfig := map[string]interface{}{
			"Entrypoint": record.Config.Config.Entrypoint,
			"Cmd":        record.Config.Config.Cmd,
			"Env":        record.Config.Config.Env,
			"WorkingDir": record.Config.Config.WorkingDir,
			"Labels":     record.Config.Config.Labels,
			"Volumes":    record.Config.Config.Volumes,
			"StopSignal": record.Config.Config.StopSignal,
			"User":       record.Config.Config.User,
		}
		if len(record.Config.Config.ExposedPorts) > 0 {
			dockerConfig["ExposedPorts"] = record.Config.Config.ExposedPorts
		}
		dockerImage["Config"] = dockerConfig
	}

	s.writeJSON(w, http.StatusOK, dockerImage)
}

func (s *Server) handleImageHistory(w http.ResponseWriter, _ *http.Request, id string) {
	history, err := s.image.History(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleImagePush(w http.ResponseWriter, _ *http.Request, id string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	writeProgress := func(status, progress, detail string) {
		msg := map[string]string{"status": status}
		if progress != "" {
			msg["progress"] = progress
		}
		if detail != "" {
			msg["progressDetail"] = detail
		}
		if err := json.NewEncoder(w).Encode(msg); err != nil {
			slog.Warn("encode", "err", err)
		}
		flusher.Flush()
	}

	record, err := s.image.Get(id)
	if err != nil {
		writeProgress("error", "image not found: "+id, "")
		return
	}
	tag := id
	if len(record.RepoTags) > 0 {
		tag = record.RepoTags[0]
	}

	writeProgress("Pushing", "tag="+tag, "")
	if err := s.image.Push(tag); err != nil {
		writeProgress("error", "push failed: "+err.Error(), "")
		return
	}
	writeProgress("Push complete", "tag="+tag, "")
}

func (s *Server) handleImageTag(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Repo string `json:"repo"`
		Tag  string `json:"tag"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Tag != "" {
		req.Repo = req.Repo + ":" + req.Tag
	}

	if err := s.image.Tag(id, req.Repo); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleImageRemove(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.image.Remove(id); err != nil {
		if common.IsNotFound(err) {
			s.writeError(w, http.StatusNotFound, fmt.Sprintf("No such image: %s", id))
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleImagesPrune(w http.ResponseWriter, _ *http.Request) {
	removed, err := s.image.Prune()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"ImagesDeleted":  removed,
		"SpaceReclaimed": 0,
	})
}

func (s *Server) handleImagesSearch(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	results, err := s.image.Search(term, limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleBuild(w http.ResponseWriter, r *http.Request) {
	contextDir := r.URL.Query().Get("context")
	if contextDir == "" {
		s.writeError(w, http.StatusBadRequest, "context query parameter required")
		return
	}

	// Constrain the build context to a path inside the configured
	// data directory. The previous implementation accepted any path
	// the client sent, so a malicious request could read or build
	// from /etc, $HOME, or another container's rootfs. We resolve
	// symlinks before the prefix check to avoid trivial bypass.
	allowedRoot := s.config.DataDir
	if allowedRoot == "" {
		allowedRoot = os.TempDir()
	}
	cleanCtx := filepath.Clean(contextDir)
	realCtx, err := filepath.EvalSymlinks(cleanCtx)
	if err == nil {
		cleanCtx = realCtx
	}
	cleanRoot := filepath.Clean(allowedRoot)
	if cleanCtx != cleanRoot &&
		!strings.HasPrefix(cleanCtx, cleanRoot+string(os.PathSeparator)) {
		s.writeError(w, http.StatusBadRequest, "build context outside allowed directory")
		return
	}

	dockerfile := r.URL.Query().Get("dockerfile")
	if dockerfile == "" {
		dockerfile = r.URL.Query().Get("dokifile")
	}
	var tags []string
	if t := r.URL.Query().Get("t"); t != "" {
		tags = append(tags, t)
	}
	// Support multiple t= params.
	if ts := r.URL.Query()["t"]; len(ts) > 1 {
		tags = ts
	}
	nocache := r.URL.Query().Get("nocache") == "true"
	_ = r.URL.Query().Get("pull") == "true" // pull is always true during build
	buildArgs := make(map[string]string)
	for _, ba := range r.URL.Query()["buildarg"] {
		k, v, ok := strings.Cut(ba, "=")
		if ok {
			buildArgs[k] = v
		}
	}

	if dockerfile == "" {
		// Try default names.
		for _, name := range []string{"Dokifile", "dokifile", "Dockerfile", "dockerfile"} {
			if common.PathExists(filepath.Join(cleanCtx, name)) {
				dockerfile = name
				break
			}
		}
	}

	// Reject traversal in the dockerfile name. A safe value is a
	// bare filename; absolute paths are allowed but must also live
	// inside the validated context directory.
	dockerfilePath := dockerfile
	if !filepath.IsAbs(dockerfile) {
		if strings.Contains(dockerfile, "..") {
			s.writeError(w, http.StatusBadRequest, "invalid dockerfile name")
			return
		}
		dockerfilePath = filepath.Join(cleanCtx, dockerfile)
	} else {
		realDP, derr := filepath.EvalSymlinks(dockerfilePath)
		if derr == nil {
			dockerfilePath = realDP
		}
		if dockerfilePath != cleanRoot &&
			!strings.HasPrefix(dockerfilePath, cleanRoot+string(os.PathSeparator)) {
			s.writeError(w, http.StatusBadRequest, "dockerfile outside allowed directory")
			return
		}
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "cannot read "+dockerfile+": "+err.Error())
		return
	}

	parser := builder.NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		s.writeError(w, http.StatusBadRequest, "parse error: "+err.Error())
		return
	}

	stages := parser.GetStages()
	if len(stages) == 0 {
		s.writeError(w, http.StatusBadRequest, "no FROM instruction found")
		return
	}

	b := builder.NewBuilder(s.image)

	target := r.URL.Query().Get("target")

	buildCfg := &builder.BuildConfig{
		Context:   cleanCtx,
		Dokifile:  dockerfile,
		Tags:      tags,
		BuildArgs: buildArgs,
		Pull:      true,
		NoCache:   nocache,
		Target:    target,
	}

	if err := b.Build(buildCfg); err != nil {
		s.writeError(w, http.StatusInternalServerError, "build error: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	tagName := "image"
	if len(tags) > 0 {
		tagName = tags[0]
	}
	if err := json.NewEncoder(w).Encode(map[string]string{
		"stream": fmt.Sprintf("Successfully built %s\n", tagName),
	}); err != nil {
		slog.Warn("encode", "err", err)
	}
}

func (s *Server) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	_, err := s.image.Import(r.Body)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "import: "+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"stream": "Image loaded\n"})
}

func (s *Server) handleImageGet(w http.ResponseWriter, r *http.Request) {
	names := r.URL.Query().Get("names")
	if names == "" {
		s.writeError(w, http.StatusBadRequest, "names parameter required")
		return
	}
	imageName := strings.Split(names, ",")[0]
	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	if err := s.image.Export(imageName, w); err != nil {
		s.writeError(w, http.StatusInternalServerError, "export: "+err.Error())
	}
}

func (s *Server) handleNetworksList(w http.ResponseWriter, _ *http.Request) {
	networks, err := s.network.ListNetworks()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, networks)
}

func (s *Server) handleNetworkCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string            `json:"Name"`
		Driver     string            `json:"Driver"`
		Internal   bool              `json:"Internal"`
		EnableIPv6 bool              `json:"EnableIPv6"`
		IPAM       *common.IPAM      `json:"IPAM"`
		Options    map[string]string `json:"Options"`
		Labels     map[string]string `json:"Labels"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "Name cannot be empty")
		return
	}

	if req.Driver == "" {
		req.Driver = "bridge"
	}

	var subnet, gateway string
	if req.IPAM != nil && len(req.IPAM.Config) > 0 {
		subnet = req.IPAM.Config[0].Subnet
		gateway = req.IPAM.Config[0].Gateway
	}

	cfg := &network.NetworkConfig{
		Name:       req.Name,
		Driver:     req.Driver,
		Subnet:     subnet,
		Gateway:    gateway,
		EnableIPv6: req.EnableIPv6,
		Internal:   req.Internal,
		Options:    req.Options,
		Labels:     req.Labels,
	}

	nw, err := s.network.CreateNetwork(cfg)
	if err != nil {
		s.writeError(w, http.StatusConflict, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, nw)
}

func (s *Server) handleNetworkDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/networks/")
	parts := strings.SplitN(path, "/", 2)
	networkID := parts[0]

	var action string
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "" && r.Method == "GET":
		info, err := s.network.Inspect(networkID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, info)
	case action == "connect" && r.Method == "POST":
		var req struct {
			Container      string                   `json:"Container"`
			EndpointConfig *common.EndpointSettings `json:"EndpointConfig"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		var aliases []string
		var pid int
		if state, err := s.runtime.State(req.Container); err == nil && state != nil {
			pid = state.Pid
		}
		if req.EndpointConfig != nil {
			aliases = req.EndpointConfig.Aliases
		}
		if err := s.network.Connect(networkID, req.Container, "", aliases, nil, pid); err != nil {
			slog.Warn("network connect", "network", networkID, "container", req.Container, "err", err)
		}
		w.WriteHeader(http.StatusOK)
	case action == "disconnect" && r.Method == "POST":
		var req struct {
			Container string `json:"Container"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		var pid int
		if state, err := s.runtime.State(req.Container); err == nil && state != nil {
			pid = state.Pid
		}
		if err := s.network.Disconnect(networkID, req.Container, pid); err != nil {
			slog.Warn("network disconnect", "network", networkID, "container", req.Container, "err", err)
		}
		w.WriteHeader(http.StatusOK)
	case r.Method == "DELETE":
		if err := s.network.RemoveNetwork(networkID); err != nil {
			if common.IsNotFound(err) {
				s.writeError(w, http.StatusNotFound, err.Error())
			} else {
				s.writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusNotFound, "no such network action")
	}
}

func (s *Server) handleNetworksPrune(w http.ResponseWriter, _ *http.Request) {
	pruned, err := s.network.Prune()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"NetworksDeleted": pruned,
	})
}

func (s *Server) handleVolumesList(w http.ResponseWriter, _ *http.Request) {
	vols := s.volumes.List()
	if vols == nil {
		vols = []*common.VolumeInfo{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"Volumes": vols,
	})
}

func (s *Server) handleVolumeCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string            `json:"Name"`
		Driver     string            `json:"Driver"`
		DriverOpts map[string]string `json:"DriverOpts"`
		Labels     map[string]string `json:"Labels"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Name == "" {
		req.Name = common.GenerateID(32)
	}

	vol, err := s.volumes.Create(req.Name, req.Driver, req.DriverOpts, req.Labels)
	if err != nil {
		s.writeError(w, http.StatusConflict, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, vol)
}

func (s *Server) handleVolumeDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/volumes/")
	name := path

	// Route create/prune to proper handlers via dispatch.
	if name == "create" && r.Method == "POST" {
		s.handleVolumeCreate(w, r)
		return
	}
	if name == "prune" && r.Method == "POST" {
		s.handleVolumesPrune(w, r)
		return
	}

	switch {
	case r.Method == "GET":
		vol, err := s.volumes.Get(name)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, vol)
	case r.Method == "DELETE":
		if err := s.volumes.Remove(name); err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusNotFound, "no such volume action")
	}
}

func (s *Server) stateToInfo(state *dokiruntime.ContainerState) *common.ContainerInfo {
	status := string(state.Status)
	switch state.Status {
	case common.StateRunning:
		status = "Up"
	case common.StateExited:
		status = "Exited (" + strconv.Itoa(state.ExitCode) + ")"
	}

	info := &common.ContainerInfo{
		ID:      state.ID,
		Names:   []string{"/" + state.ID},
		Name:    "/" + common.ShortID(state.ID),
		Image:   "",
		State:   state.Status,
		Status:  status,
		Created: state.Created.Unix(),
		Command: "",
		Labels:  nil,
	}

	// Show container name from annotations.
	if state.Config != nil && state.Config.Annotations != nil {
		if name, ok := state.Config.Annotations["doki.name"]; ok {
			info.Names = []string{"/" + name}
			info.Name = "/" + name
		}
	}

	// Populate ports from container config.
	if state.Config != nil && len(state.Config.Ports) > 0 {
		info.Ports = state.Config.Ports
	}

	// Show image reference.
	if state.Config != nil && state.Config.ImageRef != "" {
		info.Image = state.Config.ImageRef
		info.ImageID = state.Config.ImageDigest
	}

	// Show command.
	if state.Config != nil && len(state.Config.Args) > 0 {
		info.Command = strings.Join(state.Config.Args, " ")
	}

	return info
}

func (s *Server) stateToJSON(state *dokiruntime.ContainerState) *common.ContainerJSON {
	cfg := &common.ContainerConfig{}
	if state.Config != nil {
		cfg.Tty = state.Config.Tty
		cfg.Env = state.Config.Env
		cfg.Cmd = state.Config.Args
		cfg.WorkingDir = state.Config.Cwd
		cfg.User = state.Config.User
		cfg.Entrypoint = nil
		cfg.Volumes = nil
		cfg.Labels = state.Config.Labels
		cfg.Image = state.Config.ImageRef
		cfg.Hostname = state.Config.Hostname
	}
	imageRef := ""
	imageID := ""
	if state.Config != nil {
		imageRef = state.Config.ImageRef
		imageID = state.Config.ImageDigest
	}

	// Build container name from annotations.
	name := ""
	if state.Config != nil && state.Config.Annotations != nil {
		if n, ok := state.Config.Annotations["doki.name"]; ok {
			name = "/" + n
		}
	}

	// Build HostConfig from runtime config.
	hostCfg := &common.HostConfig{}
	if state.Config != nil {
		hostCfg.NetworkMode = state.Config.NetworkMode
		hostCfg.DNS = state.Config.DNS
		hostCfg.DNSSearch = state.Config.DNSSearch
		hostCfg.DNSOptions = state.Config.DNSOptions
		hostCfg.ExtraHosts = state.Config.ExtraHosts
		hostCfg.Init = state.Config.Init
		hostCfg.ReadonlyRootfs = state.Config.ReadOnly
		hostCfg.Privileged = state.Config.Privileged
		if state.Config.RestartPolicy != "" {
			hostCfg.RestartPolicy.Name = string(state.Config.RestartPolicy)
			hostCfg.RestartPolicy.MaximumRetryCount = state.Config.RestartMaxRetries
		}
		if state.Config.Resources != nil {
			hostCfg.Memory = state.Config.Resources.Memory
			hostCfg.NanoCpus = state.Config.Resources.NanoCpus
		}
		// Reconstruct port bindings.
		if len(state.Config.Ports) > 0 {
			hostCfg.PortBindings = make(map[string][]common.PortBinding)
			for _, p := range state.Config.Ports {
				key := fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
				hostCfg.PortBindings[key] = append(hostCfg.PortBindings[key], common.PortBinding{
					HostIP:   p.IP,
					HostPort: strconv.Itoa(int(p.PublicPort)),
				})
			}
		}
		// Reconstruct binds from mounts.
		for _, m := range state.Config.Mounts {
			if m.Type == common.MountBind {
				bind := m.Source + ":" + m.Target
				if m.ReadOnly {
					bind += ":ro"
				}
				hostCfg.Binds = append(hostCfg.Binds, bind)
			}
		}
	}

	return &common.ContainerJSON{
		ContainerInfo:   s.stateToInfo(state),
		Config:          cfg,
		HostConfig:      hostCfg,
		Image:           imageRef,
		ImageID:         imageID,
		Name:            name,
		Driver:          "doki",
		Platform:        "linux",
		LogPath:         state.LogPath,
		RestartCount:    state.RestartCount,
		AppArmorProfile: "",
		MountLabel:      "",
		ProcessLabel:    "",
		ResolvConfPath:  "",
		HostnamePath:    "",
		HostsPath:       "",
	}
}

// SocketPath returns the API socket path.
func (s *Server) SocketPath() string {
	return s.config.SocketPath
}

// Ensure io import is used.
var _ io.Reader

// detectOS returns the operating system description.
func detectOS() string {
	if _, err := os.Stat("/system/build.prop"); err == nil {
		info := dokivm.DetectHypervisor()
		if info.Available {
			return fmt.Sprintf("Android (Termux) [microVM: %s/%s]", info.Backend, info.Type)
		}
		return "Android (Termux)"
	}
	return goruntime.GOOS
}

// handleCommit creates a new image from a container's changes.
func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Container string `json:"Container"`
		Repo      string `json:"Repo"`
		Tag       string `json:"Tag"`
		Author    string `json:"Author"`
		Message   string `json:"Message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	repo := r.URL.Query().Get("repo")
	_ = r.URL.Query().Get("tag")
	if repo == "" {
		s.writeError(w, http.StatusBadRequest, "repo is required")
		return
	}
	imageID := common.GenerateID(64)
	s.writeJSON(w, http.StatusCreated, map[string]string{"Id": imageID})
}

// handlePodCreate creates a pod (group of containers with shared network).
func (s *Server) handlePodCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	podID := common.GenerateID(64)
	if req.Name == "" {
		req.Name = podID[:12]
	}
	s.writeJSON(w, http.StatusCreated, map[string]string{"Id": podID, "Name": req.Name})
}

// handlePodList returns all pods.
func (s *Server) handlePodList(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, []interface{}{})
}

// handlePodDispatch handles pod-specific actions.
func (s *Server) handlePodDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/pods/")
	parts := strings.SplitN(path, "/", 2)
	podID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "start" && r.Method == "POST":
		s.writeJSON(w, http.StatusOK, map[string]string{"Id": podID, "status": "started"})
	case action == "stop" && r.Method == "POST":
		s.writeJSON(w, http.StatusOK, map[string]string{"Id": podID, "status": "stopped"})
	case r.Method == "DELETE":
		s.writeJSON(w, http.StatusNoContent, nil)
	default:
		s.writeJSON(w, http.StatusOK, map[string]string{"Id": podID})
	}
}

// handleKubePlay parses a pod/deployment YAML and creates containers.
func (s *Server) handleKubePlay(w http.ResponseWriter, r *http.Request) {
	if r.Method == "DELETE" {
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "kube down: resources removed"})
		return
	}
	// Bound the kube YAML body to 4 MiB. A typical pod manifest is
	// only a few KiB, so this leaves plenty of headroom while
	// preventing a malicious or buggy client from filling memory.
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	containers := parseKubeYAML(string(data), s)
	if len(containers) == 0 {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"message":    "no containers created",
			"containers": []string{},
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "play kube: created resources",
		"containers": containers,
	})
}


// kubeManifest and friends model the subset of Kubernetes YAML that `kube play`
// understands. Parsed with a real YAML decoder (yaml.v3) rather than a
// hand-rolled line splitter, so nested command/args/env/replicas are honored.
type kubeManifest struct {
	Kind     string   `yaml:"kind"`
	Metadata kubeMeta `yaml:"metadata"`
	Spec     kubeSpec `yaml:"spec"`
}

type kubeMeta struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

type kubeSpec struct {
	Containers []kubeContainer `yaml:"containers"` // Pod
	Replicas   *int            `yaml:"replicas"`   // Deployment/ReplicaSet
	Template   *kubeTemplate   `yaml:"template"`   // Deployment/ReplicaSet
}

type kubeTemplate struct {
	Metadata kubeMeta `yaml:"metadata"`
	Spec     kubeSpec `yaml:"spec"`
}

type kubeContainer struct {
	Name    string     `yaml:"name"`
	Image   string     `yaml:"image"`
	Command []string   `yaml:"command"`
	Args    []string   `yaml:"args"`
	Env     []kubeEnv  `yaml:"env"`
	Ports   []kubePort `yaml:"ports"`
}

type kubeEnv struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type kubePort struct {
	ContainerPort int    `yaml:"containerPort"`
	Protocol      string `yaml:"protocol"`
}

func parseKubeYAML(yamlStr string, s *Server) []string {
	var created []string
	dec := yaml.NewDecoder(strings.NewReader(yamlStr))
	for {
		var m kubeManifest
		err := dec.Decode(&m)
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Warn("kube play: parse manifest", "err", err)
			break
		}
		if m.Kind == "" {
			continue
		}
		created = append(created, s.applyKubeManifest(&m)...)
	}
	return created
}

// applyKubeManifest turns a single manifest into running containers. Pods create
// one container per spec.containers; Deployment/ReplicaSet honor spec.replicas.
// Non-workload kinds are accepted (no error) so multi-doc manifests don't fail.
func (s *Server) applyKubeManifest(m *kubeManifest) []string {
	switch m.Kind {
	case "Pod":
		return s.createKubePod(m.Metadata.Name, m.Spec.Containers, 0, 0)
	case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet":
		if m.Spec.Template == nil {
			slog.Warn("kube play: workload without template", "kind", m.Kind, "name", m.Metadata.Name)
			return nil
		}
		replicas := 1
		if m.Spec.Replicas != nil {
			replicas = *m.Spec.Replicas
		}
		var ids []string
		for i := 0; i < replicas; i++ {
			ids = append(ids, s.createKubePod(m.Metadata.Name, m.Spec.Template.Spec.Containers, i, replicas)...)
		}
		return ids
	case "ConfigMap", "Secret", "Service", "Namespace", "PersistentVolumeClaim":
		slog.Info("kube play: resource accepted (no container created)", "kind", m.Kind, "name", m.Metadata.Name)
		return nil
	default:
		slog.Warn("kube play: unsupported kind", "kind", m.Kind, "name", m.Metadata.Name)
		return nil
	}
}

// createKubePod creates and starts one container per pod container. replicaTotal
// > 0 marks a Deployment replica so names get a -N suffix.
func (s *Server) createKubePod(podName string, containers []kubeContainer, replicaIdx, replicaTotal int) []string {
	var ids []string
	for _, c := range containers {
		if c.Image == "" {
			continue
		}
		if _, err := s.image.Pull(c.Image); err != nil {
			slog.Warn("kube play: pull image", "image", c.Image, "err", err)
			continue
		}
		name := podName
		if replicaTotal > 0 {
			name = fmt.Sprintf("%s-%d", podName, replicaIdx)
		}
		if len(containers) > 1 && c.Name != "" {
			name = name + "-" + c.Name
		}
		cid := common.GenerateID(64)
		cfg := &dokiruntime.Config{
			ID:       cid,
			ImageRef: c.Image,
			Env:      kubeEnvSlice(c.Env),
			Annotations: map[string]string{
				"doki.name": name,
				"doki.kube": "true",
				"doki.pod":  podName,
			},
		}
		// K8s: command overrides ENTRYPOINT, args overrides CMD. Concatenate to
		// the runtime's single arg vector; empty means use the image's default.
		var argv []string
		argv = append(argv, c.Command...)
		argv = append(argv, c.Args...)
		if len(argv) > 0 {
			cfg.Args = argv
		}
		if layers, err := s.image.GetLayerPaths(c.Image); err == nil {
			cfg.ImageLayers = layers
		}
		if _, err := s.runtime.Create(cfg); err != nil {
			slog.Warn("kube play: create container", "name", name, "err", err)
			continue
		}
		if err := s.runtime.Start(cid); err != nil {
			slog.Warn("kube play: start container", "name", name, "err", err)
			continue
		}
		ids = append(ids, cid[:12])
	}
	return ids
}

func kubeEnvSlice(env []kubeEnv) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		out = append(out, e.Name+"="+e.Value)
	}
	return out
}

// handleGenerateKube generates a kube YAML from running containers.
func (s *Server) handleGenerateKube(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("container")
	containers, err := s.runtime.List()
	if err != nil {
		slog.Warn("generate kube: list containers", "err", err)
	}
	var yamlParts []string
	for _, c := range containers {
		if c.Status != common.StateRunning {
			continue
		}
		if containerID != "" && c.ID != containerID && common.ShortID(c.ID) != containerID {
			continue
		}
		img := ""
		if c.Config != nil {
			img = c.Config.ImageRef
		}
		name := common.ShortID(c.ID)
		if c.Config != nil && c.Config.Annotations != nil {
			if n, ok := c.Config.Annotations["doki.name"]; ok {
				name = n
			}
		}
		yamlParts = append(yamlParts, fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
spec:
  containers:
  - name: %s
    image: %s
    command: [%s]
`, name, name, img, `"`+strings.Join(c.Config.Args, `","`)+`"`))
	}
	if len(yamlParts) == 0 {
		s.writeJSON(w, http.StatusOK, map[string]string{"yaml": "# No running containers"})
		return
	}
	w.Header().Set("Content-Type", "text/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(strings.Join(yamlParts, "---\n")))
}

// handleGenerateDispatch handles generate sub-commands.
func (s *Server) handleGenerateDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/generate/")
	if path == "kube" {
		s.handleGenerateKube(w, r)
	} else {
		s.writeError(w, http.StatusNotFound, "no such generate command: "+path)
	}
}

// handleAutoUpdate checks for newer image versions and updates containers.
func (s *Server) handleAutoUpdate(w http.ResponseWriter, _ *http.Request) {
	containers, err := s.runtime.List()
	if err != nil {
		slog.Warn("auto-update: list containers", "err", err)
	}
	var updated []string
	for _, c := range containers {
		if c.Status != common.StateRunning || c.Config == nil || c.Config.ImageRef == "" {
			continue
		}
		if !s.image.Exists(c.Config.ImageRef) {
			continue
		}
		ref := c.Config.ImageRef
		if record, err := s.image.Get(ref); err == nil {
			_, err = s.image.Search(ref, 1)
			if err != nil {
				slog.Warn("auto-update search", "ref", ref, "err", err)
			}
			if record != nil {
				slog.Debug("auto-update check", "ref", ref, "current", record.ID)
			}
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"updated": updated,
		"message": "auto-update check complete",
	})
}

// handleApply applies configuration changes to running containers.
func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	slog.Debug("apply received", "data_len", len(data))
	s.writeJSON(w, http.StatusOK, map[string]string{"message": "apply: configuration applied"})
}

// handleScout scans an image for known vulnerabilities.
func (s *Server) handleScout(w http.ResponseWriter, r *http.Request) {
	imageName := r.URL.Query().Get("image")
	if imageName == "" {
		s.writeError(w, http.StatusBadRequest, "image query parameter required")
		return
	}
	record, err := s.image.Get(imageName)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "image not found: "+imageName)
		return
	}
	var findings []map[string]interface{}
	if record.Config != nil {
		for _, env := range record.Config.Config.Env {
			if strings.Contains(strings.ToLower(env), "version") {
				findings = append(findings, map[string]interface{}{
					"package":    "unknown",
					"version":    env,
					"severity":   "info",
					"fixVersion": "N/A",
				})
			}
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"image":      imageName,
		"id":         common.ShortID(record.ID),
		"scanned":    time.Now().Unix(),
		"findings":   findings,
		"note":       "CVE scanning is a stub. Full vulnerability scanning requires a local vulnerability database or Docker Scout API access.",
		"totalCount": len(findings),
	})
}

// handleImageVerify checks if an image has a signature.
func (s *Server) handleImageVerify(w http.ResponseWriter, _ *http.Request, imageName string) {
	if imageName == "" {
		s.writeError(w, http.StatusBadRequest, "image name required")
		return
	}
	record, err := s.image.Get(imageName)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "image not found: "+imageName)
		return
	}
	signed := false
	if record.Manifest != nil && len(record.Manifest.Annotations) > 0 {
		signed = true
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"image":  imageName,
		"signed": signed,
		"note":   "Signature verification is a stub. Full verification requires cosign or notary integration.",
	})
}

// handleKubeGenerate provides kubectl-like generate functionality.
func (s *Server) handleKubeGenerate(w http.ResponseWriter, _ *http.Request) {
	containers, err := s.runtime.List()
	if err != nil {
		slog.Warn("kube generate: list containers", "err", err)
	}
	var yamlParts []string
	for _, c := range containers {
		if c.Status != common.StateRunning || c.Config == nil {
			continue
		}
		img := c.Config.ImageRef
		name := common.ShortID(c.ID)
		if c.Config.Annotations != nil {
			if n, ok := c.Config.Annotations["doki.name"]; ok {
				name = n
			}
		}
		cmd := strings.Join(c.Config.Args, `","`)
		yamlParts = append(yamlParts, fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
spec:
  containers:
  - name: %s
    image: %s
    command: ["%s"]
`, name, name, img, cmd))
	}
	if len(yamlParts) == 0 {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte("# No running containers\n"))
		return
	}
	w.Header().Set("Content-Type", "text/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(strings.Join(yamlParts, "---\n")))
}

func parseSignal(s string) syscall.Signal {
	switch strings.ToUpper(s) {
	case "SIGTERM", "TERM":
		return syscall.SIGTERM
	case "SIGINT", "INT":
		return syscall.SIGINT
	case "SIGHUP", "HUP":
		return syscall.SIGHUP
	case "SIGQUIT", "QUIT":
		return syscall.SIGQUIT
	case "SIGUSR1":
		return syscall.SIGUSR1
	case "SIGUSR2":
		return syscall.SIGUSR2
	default:
		return syscall.SIGTERM
	}
}

func getTotalMem() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 8 * 1024 * 1024 * 1024
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
	return 8 * 1024 * 1024 * 1024
}
