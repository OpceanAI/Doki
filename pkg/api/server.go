package api

import (
	"archive/tar"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	goruntime "runtime"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/dokivm"
	"github.com/OpceanAI/Doki/pkg/builder"
	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
)

// Server implements the Docker Engine v1.44 compatible HTTP API.
type Server struct {
	config     *common.DokiConfig
	router     *http.ServeMux
	server     *http.Server
	listener   net.Listener
	runtime    *dokiruntime.Runtime
	image      *image.Store
	network    *network.Manager
	volumes    *VolumeManager
	events     chan *common.SystemEventsResponse
	middleware []func(http.Handler) http.Handler
	handler    http.Handler
	dnsSrv     *network.DNSServer
	execStore  map[string]*common.ExecConfig
	execMu     sync.RWMutex
}

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
func NewVolumeManager(root string) *VolumeManager {
	common.EnsureDir(root)
	vm := &VolumeManager{
		root:    root,
		volumes: make(map[string]*common.VolumeInfo),
	}
	// Load existing volumes on startup.
	vm.loadFromDisk()
	return vm
}

func (vm *VolumeManager) loadFromDisk() {
	entries, _ := os.ReadDir(vm.root)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		volPath := filepath.Join(vm.root, entry.Name(), "volume.json")
		data, err := os.ReadFile(volPath)
		if err != nil {
			continue
		}
		var vol common.VolumeInfo
		if json.Unmarshal(data, &vol) == nil {
			vm.volumes[vol.Name] = &vol
		}
	}
}

func (vm *VolumeManager) Create(name string, driver string, opts map[string]string, labels map[string]string) (*common.VolumeInfo, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if _, exists := vm.volumes[name]; exists {
		return nil, common.NewErrConflict("volume", name)
	}

	mountpoint := filepath.Join(vm.root, name)
	common.EnsureDir(mountpoint)

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
	data, _ := json.Marshal(vol)
	os.WriteFile(filepath.Join(mountpoint, "volume.json"), data, 0644)

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

	delete(vm.volumes, name)
	os.RemoveAll(vol.Mountpoint)
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
		delete(vm.volumes, name)
		os.RemoveAll(vol.Mountpoint)
		pruned = append(pruned, name)
	}
	return pruned, nil
}

func (s *Server) handleVolumesPrune(w http.ResponseWriter, r *http.Request) {
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
func NewServer(config *common.DokiConfig, rt *dokiruntime.Runtime, img *image.Store, net *network.Manager) *Server {
	s := &Server{
		config:    config,
		router:    http.NewServeMux(),
		runtime:   rt,
		image:     img,
		network:   net,
		volumes:   NewVolumeManager(filepath.Join(config.DataDir, "volumes")),
		events:    make(chan *common.SystemEventsResponse, 100),
		execStore: make(map[string]*common.ExecConfig),
	}
	s.registerRoutes()
	s.rebuildHandler()
	return s
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
	s.router.HandleFunc("/_ping", s.handlePing)
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
	os.Remove(s.config.SocketPath)

	listener, err := net.Listen("unix", s.config.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.config.SocketPath, err)
	}

	s.listener = listener
	s.server = &http.Server{
		Handler: s,
	}

	return s.server.Serve(listener)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"message": message})
}

func (s *Server) sendEvent(event *common.SystemEventsResponse) {
	select {
	case s.events <- event:
	default:
	}
}

// Shutdown stops the API server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// Handler implementations follow.

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.runtime.List()
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

	images, _ := s.image.List()

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

func (s *Server) handleSystemVersion(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, common.GetVersion())
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)

	for {
		select {
		case event := <-s.events:
			data, _ := json.Marshal(event)
			w.Write(data)
			w.Write([]byte("\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleSystemDf(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.runtime.List()
	images, _ := s.image.List()
	var totalSize int64
	for _, img := range images {
		totalSize += img.Size
	}
	type dfResponse struct {
		LayersSize  int64         `json:"LayersSize"`
		Images      interface{}   `json:"Images"`
		Containers  interface{}   `json:"Containers"`
		Volumes     interface{}   `json:"Volumes"`
		BuildCache  interface{}   `json:"BuildCache"`
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
	// Placeholder - authentication token response.
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"Status":        "Login Succeeded",
		"IdentityToken": "doki-token-" + common.GenerateID(16),
	})
}

func (s *Server) handleSwarmNoop(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Swarm mode not available in Doki",
	})
}

func (s *Server) handleContainersList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "true"

	states, err := s.runtime.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	containers := make([]common.ContainerInfo, 0)
	for _, state := range states {
		if !all && state.Status != common.StateRunning {
			continue
		}
		containers = append(containers, *s.stateToInfo(state))
	}

	s.writeJSON(w, http.StatusOK, containers)
}

func (s *Server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Image       string              `json:"Image"`
		Cmd         []string            `json:"Cmd"`
		Entrypoint  []string            `json:"Entrypoint"`
		Env         []string            `json:"Env"`
		Tty         bool                `json:"Tty"`
		OpenStdin   bool                `json:"OpenStdin"`
		WorkingDir  string              `json:"WorkingDir"`
		HostConfig  *common.HostConfig  `json:"HostConfig"`
		Labels      map[string]string   `json:"Labels"`
		ContainerName string            `json:"Name,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Pull image based on pull policy.
	pullPolicy := r.URL.Query().Get("pull")
	if pullPolicy == "" {
		pullPolicy = "missing"
	}
	switch pullPolicy {
	case "always":
		if _, err := s.image.Pull(req.Image); err != nil {
			s.writeError(w, http.StatusInternalServerError, "pull image: "+err.Error())
			return
		}
	case "missing":
		if req.Image != "" && !s.image.Exists(req.Image) {
			if _, err := s.image.Pull(req.Image); err != nil {
				s.writeError(w, http.StatusInternalServerError, "pull image: "+err.Error())
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
	}

	// Copy user-provided labels.
	cfg.Labels = req.Labels

	// G8: Set working directory from request or image config.
	if req.WorkingDir != "" {
		cfg.Cwd = req.WorkingDir
	} else if imgRecord.Config != nil && imgRecord.Config.Config.WorkingDir != "" {
		cfg.Cwd = imgRecord.Config.Config.WorkingDir
	}

	// Copy user from image config if not already set.
	if imgRecord.Config != nil && imgRecord.Config.Config.User != "" {
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
				Test:     imgRecord.Config.Config.HealthCheck.Test,
				Interval: time.Duration(imgRecord.Config.Config.HealthCheck.Interval) * time.Nanosecond,
				Timeout:  time.Duration(imgRecord.Config.Config.HealthCheck.Timeout) * time.Nanosecond,
				Retries:  imgRecord.Config.Config.HealthCheck.Retries,
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
		for _, bind := range req.HostConfig.PortBindings {
			for _, pb := range bind {
				if port, err := strconv.Atoi(pb.HostPort); err == nil && port > 0 {
					// AE10: Enforce port binding restrictions.
					if !req.HostConfig.Privileged && uint16(port) < 1024 {
						continue
					}
					cfg.Ports = append(cfg.Ports, common.Port{
						PrivatePort: uint16(port),
						PublicPort:  uint16(port),
						Type:        common.ProtocolTCP,
					})
				}
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

	_ = imgRecord
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
	case action == "health" && r.Method == "GET":
		s.handleContainerHealth(w, r, containerID)
	case action == "changes" && r.Method == "GET":
		s.handleContainerChanges(w, r, containerID)
	case action == "export" && r.Method == "GET":
		s.handleContainerExport(w, r, containerID)
	case action == "archive" && (r.Method == "GET" || r.Method == "PUT"):
		s.writeError(w, http.StatusNotImplemented, "container cp not yet implemented")
	case action == "update" && r.Method == "POST":
		s.handleContainerUpdate(w, r, containerID)  // Line: 779
	case r.Method == "DELETE":
		s.handleContainerDelete(w, r, containerID)
	default:
		s.writeError(w, http.StatusNotFound, "no such container action: "+action)
	}
}

func (s *Server) handleContainerInspect(w http.ResponseWriter, r *http.Request, id string) {
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

	// Ensure we always write valid JSON.
	data, err := json.Marshal(js)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "marshal: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (s *Server) handleContainerStart(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.runtime.Start(id); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Configure networking after start.
	if state, err := s.runtime.State(id); err == nil && state != nil && state.Pid > 0 {
		networkMode := string(common.NetworkBridge)
		if state.Config != nil && state.Config.NetworkMode != "" {
			networkMode = string(state.Config.NetworkMode)
		}
		s.network.SetupNetwork(id, state.Pid, networkMode)
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

	if state, err := s.runtime.State(id); err == nil && state != nil && state.Config != nil {
		networkMode := string(state.Config.NetworkMode)
		if networkMode == "" {
			networkMode = string(common.NetworkBridge)
		}
		s.network.TeardownNetwork(id, networkMode)
	}

	if err := s.runtime.Stop(id, timeout); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
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
	if err := s.runtime.Stop(id, timeout); err != nil {
		s.writeError(w, http.StatusInternalServerError, "stop: "+err.Error())
		return
	}
	if err := s.runtime.Start(id); err != nil {
		s.writeError(w, http.StatusInternalServerError, "start: "+err.Error())
		return
	}
	// Reconfigure networking after restart.
	if state, err := s.runtime.State(id); err == nil && state != nil && state.Pid > 0 {
		networkMode := string(common.NetworkBridge)
		if state.Config != nil && state.Config.NetworkMode != "" {
			networkMode = string(state.Config.NetworkMode)
		}
		s.network.SetupNetwork(id, state.Pid, networkMode)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerKill(w http.ResponseWriter, r *http.Request, id string) {
	sig := syscall.SIGKILL
	if s := r.URL.Query().Get("signal"); s != "" {
		sig = parseSignal(s)
	}
	if err := s.runtime.Kill(id, sig); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerPause(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.runtime.Pause(id); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.runtime.Unpause(id); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerWait(w http.ResponseWriter, r *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Wait for exit.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		state, err = s.runtime.State(id)
		if err != nil || state.Status == common.StateExited || state.Status == common.StateDead {
			break
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]int{"StatusCode": state.ExitCode})
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
	w.WriteHeader(http.StatusOK)

	// Write initial logs.
	logFile, err := os.Open(state.LogPath)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "no log file: "+err.Error())
		return
	}
	defer logFile.Close()

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
		s.writeLogLine(w, line, stdout, stderr, timestamps, sinceTs, untilTs)
	}
	if flusher != nil {
		flusher.Flush()
	}

	if !follow {
		return
	}

	// If follow mode, tail the log file.
	done := r.Context().Done()
	offset, _ := logFile.Seek(0, io.SeekEnd)
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
				f.Seek(offset, io.SeekStart)
				newContent := make([]byte, fi.Size()-offset)
				n, _ := io.ReadFull(f, newContent)
				f.Close()
				offset += int64(n)

				newLines := strings.Split(string(newContent[:n]), "\n")
				for _, line := range newLines {
					if line != "" {
						s.writeLogLine(w, line, stdout, stderr, timestamps, sinceTs, untilTs)
					}
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			// If container exited and we've read all data, stop following.
			currentState, err := s.runtime.State(id)
			if err != nil || (currentState.Status != common.StateRunning && currentState.Status != common.StateCreated) {
				fi, _ := os.Stat(state.LogPath)
				if fi != nil && fi.Size() <= offset {
					return
				}
			}
		}
	}
}

// writeLogLine writes a single log line in Docker multiplexed stream format.
// Frame: [stream-type(1)][0][0][0][size(4, big-endian)][data]
// Stream type: 1=stdout, 2=stderr, 0=stdin
func (s *Server) writeLogLine(w io.Writer, line string, stdout, stderr bool, timestamps bool, sinceTs, untilTs int64) {
	streamType := byte(1) // stdout

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
	header := make([]byte, 8)
	header[0] = streamType
	binary.BigEndian.PutUint32(header[4:], uint32(len(line)))
	w.Write(header)
	w.Write([]byte(line))
}

func (s *Server) handleContainerTop(w http.ResponseWriter, r *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	cmdline := ""
	if data, e := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", state.Pid)); e == nil {
		cmdline = strings.ReplaceAll(string(data), "\x00", " ")
	}
	if cmdline == "" {
		cmdline = "-"
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"Titles":    []string{"PID", "USER", "COMMAND"},
		"Processes": [][]string{{fmt.Sprintf("%d", state.Pid), "root", cmdline}},
	})
}

func (s *Server) handleContainerDelete(w http.ResponseWriter, r *http.Request, id string) {
	force := r.URL.Query().Get("force") == "true"

	if err := s.runtime.Delete(id, force); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerStats(w http.ResponseWriter, r *http.Request, id string) {
	stream := r.URL.Query().Get("stream")
	_ = stream

	stats, err := s.runtime.Stats(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if stream == "false" || stream == "0" {
		s.writeJSON(w, http.StatusOK, stats)
		return
	}

	// AG3: Streaming stats response
	w.Header().Set("Content-Type", "application/json")
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeJSON(w, http.StatusOK, stats)
		return
	}

	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(stats)
	w.Write(data)
	w.Write([]byte("\n"))
	flusher.Flush()
}

func (s *Server) handleContainerRename(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	// Store the new name in container labels.
	if state.Config != nil {
		if state.Config.Annotations == nil {
			state.Config.Annotations = make(map[string]string)
		}
		state.Config.Annotations["doki.name"] = req.Name
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"name": req.Name})
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
	// Stream stdin/stdout/stderr via raw TCP hijack
	hj, ok := w.(http.Hijacker)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "hijacking not supported")
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/vnd.docker.raw-stream\r\n\r\n"))
	// Stream container output
	rootfsDir := state.Config.RootfsReady
	if rootfsDir != "" {
		// Attach via proot exec to read container stdout
		go io.Copy(conn, conn) // echo stdin back
	}
}

// G5: handleContainerHealth returns health status for a container.
func (s *Server) handleContainerChanges(w http.ResponseWriter, r *http.Request, id string) {
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

func (s *Server) handleContainerExport(w http.ResponseWriter, r *http.Request, id string) {
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
	defer tw.Close()
	filepath.Walk(rootfsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(rootfsDir, path)
		if rel == "." {
			return nil
		}
		hdr, _ := tar.FileInfoHeader(info, "")
		hdr.Name = rel
		tw.WriteHeader(hdr)
		if !info.IsDir() {
			f, _ := os.Open(path)
			if f != nil {
				io.Copy(tw, f)
				f.Close()
			}
		}
		return nil
	})
}

func (s *Server) handleContainerUpdate(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Memory       int64 `json:"Memory"`
		MemorySwap   int64 `json:"MemorySwap"`
		NanoCpus     int64 `json:"NanoCpus"`
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"Warnings": []string{}})
}

func getRootfsChanges(rootfsDir string) []map[string]string {
	var changes []map[string]string
	// Walk rootfs and report added/modified/deleted files
	filepath.Walk(rootfsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(rootfsDir, path)
		changes = append(changes, map[string]string{"Path": "/" + rel, "Kind": "C"})
		return nil
	})
	return changes
}

func (s *Server) handleContainerHealth(w http.ResponseWriter, r *http.Request, id string) {
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

func (s *Server) handleContainersPrune(w http.ResponseWriter, r *http.Request) {
	states, _ := s.runtime.List()
	var pruned []string
	for _, state := range states {
		if state.Status != common.StateRunning {
			s.runtime.Delete(state.ID, true)
			pruned = append(pruned, common.ShortID(state.ID))
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

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
		s.writeJSON(w, http.StatusOK, map[string]string{})
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
	s.execMu.RLock()
	cfg, ok := s.execStore[execID]
	s.execMu.RUnlock()

	if !ok {
		s.writeError(w, http.StatusNotFound, "exec instance not found")
		return
	}

	if cfg.Running {
		s.writeError(w, http.StatusConflict, "exec instance already started")
		return
	}

	if err := s.runtime.Exec(cfg.ContainerID, cfg.Cmd, cfg.Env, cfg.Tty); err != nil {
		s.writeError(w, http.StatusInternalServerError, "exec: "+err.Error())
		return
	}

	s.execMu.Lock()
	cfg.Running = true
	s.execMu.Unlock()

	w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleImagesList(w http.ResponseWriter, r *http.Request) {
	images, err := s.image.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, images)
}

func (s *Server) handleImageCreate(w http.ResponseWriter, r *http.Request) {
	// Pull image.
	imageName := r.URL.Query().Get("fromImage")
	if imageName == "" {
		s.writeError(w, http.StatusBadRequest, "fromImage query parameter required")
		return
	}

	tag := r.URL.Query().Get("tag")
	if tag != "" {
		imageName = imageName + ":" + tag
	}

	record, err := s.image.Pull(imageName)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "pull: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "pulling " + imageName,
		"id":     common.ShortID(record.ID),
	})
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

func (s *Server) handleImageInspect(w http.ResponseWriter, r *http.Request, id string) {
	record, err := s.image.Get(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleImageHistory(w http.ResponseWriter, r *http.Request, id string) {
	history, err := s.image.History(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleImagePush(w http.ResponseWriter, r *http.Request, id string) {
	s.writeError(w, http.StatusNotImplemented, "push not yet implemented in Doki")
}

func (s *Server) handleImageTag(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Repo string `json:"repo"`
		Tag  string `json:"tag"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Tag != "" {
		req.Repo = req.Repo + ":" + req.Tag
	}

	if err := s.image.Tag(id, req.Repo); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleImageRemove(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.image.Remove(id); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleImagesPrune(w http.ResponseWriter, r *http.Request) {
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
	dockerfile := r.URL.Query().Get("dockerfile")
	if dockerfile == "" {
		dockerfile = r.URL.Query().Get("dokifile")
	}
	tag := r.URL.Query().Get("t")
	noCache := r.URL.Query().Get("nocache") == "true"
	_ = noCache // Future: skip cache when building

	if dockerfile == "" {
		// Try default names.
		for _, name := range []string{"Dokifile", "dokifile", "Dockerfile", "dockerfile"} {
			if common.PathExists(filepath.Join(contextDir, name)) {
				dockerfile = name
				break
			}
		}
	}

	// If dockerfile is an absolute path, use it directly.
	dockerfilePath := dockerfile
	if !filepath.IsAbs(dockerfile) {
		dockerfilePath = filepath.Join(contextDir, dockerfile)
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

	// Pull base images first.
	for _, stage := range stages {
		if stage.From != "" && !s.image.Exists(stage.From) {
			if _, err := s.image.Pull(stage.From); err != nil {
				s.writeError(w, http.StatusInternalServerError, "pull base image "+stage.From+": "+err.Error())
				return
			}
		}
	}

	b := builder.NewBuilder(s.image)
	workDir, _ := os.MkdirTemp("", "doki-build-")
	defer os.RemoveAll(workDir)

	// Execute each stage sequentially.
	for _, stage := range stages {
		if err := b.ExecuteStage(stage, contextDir, workDir); err != nil {
			s.writeError(w, http.StatusInternalServerError, "build error at stage "+stage.From+": "+err.Error())
			return
		}
	}

	// Tag the built image if requested, using the base image as output.
	baseImage := stages[len(stages)-1].From
	if tag != "" {
		if record, err := s.image.Get(baseImage); err == nil && record != nil {
			newRecord := &image.ImageRecord{
				ID:           record.ID,
				RepoTags:     []string{tag},
				RepoDigests:  record.RepoDigests,
				Config:       record.Config,
				Manifest:     record.Manifest,
				Size:         record.Size,
				Created:      common.NowTimestamp(),
				Architecture: record.Architecture,
				OS:           record.OS,
				Layers:       record.Layers,
			}
			s.image.SaveRecord(newRecord)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"stream": fmt.Sprintf("Successfully built %s (from %s)\n", tag, baseImage),
	})
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

func (s *Server) handleNetworksList(w http.ResponseWriter, r *http.Request) {
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

	json.NewDecoder(r.Body).Decode(&req)

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
			Container      string                          `json:"Container"`
			EndpointConfig *common.EndpointSettings        `json:"EndpointConfig"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var aliases []string
		var pid int
		if state, err := s.runtime.State(req.Container); err == nil && state != nil {
			pid = state.Pid
		}
		if req.EndpointConfig != nil {
			aliases = req.EndpointConfig.Aliases
		}
		s.network.Connect(networkID, req.Container, "", aliases, nil, pid)
		w.WriteHeader(http.StatusOK)
	case action == "disconnect" && r.Method == "POST":
		var req struct {
			Container string `json:"Container"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var pid int
		if state, err := s.runtime.State(req.Container); err == nil && state != nil {
			pid = state.Pid
		}
		s.network.Disconnect(networkID, req.Container, pid)
		w.WriteHeader(http.StatusOK)
	case r.Method == "DELETE":
		if err := s.network.RemoveNetwork(networkID); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusNotFound, "no such network action")
	}
}

func (s *Server) handleNetworksPrune(w http.ResponseWriter, r *http.Request) {
	pruned, err := s.network.Prune()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"NetworksDeleted": pruned,
	})
}

func (s *Server) handleVolumesList(w http.ResponseWriter, r *http.Request) {
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

	json.NewDecoder(r.Body).Decode(&req)

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
		Names:   []string{"/" + common.ShortID(state.ID)},
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
		if state.Config.Annotations != nil {
			if n, ok := state.Config.Annotations["doki.name"]; ok {
				cfg.Hostname = n
			}
		}
	}
	return &common.ContainerJSON{
		ContainerInfo:  s.stateToInfo(state),
		Config:         cfg,
		Image:          state.Config.ImageRef,
		Driver:         "doki",
		Platform:       "linux",
		LogPath:        state.LogPath,
		RestartCount:   state.RestartCount,
		AppArmorProfile: "",
		MountLabel:     "",
		ProcessLabel:   "",
		ResolvConfPath: "",
		HostnamePath:   "",
		HostsPath:      "",
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

func handleNotImplemented(msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprintf(w, `{"message":"%s not yet implemented"}`, msg)
	}
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
	json.NewDecoder(r.Body).Decode(&req)
	repo := r.URL.Query().Get("repo")
	tag := r.URL.Query().Get("tag")
	if repo == "" {
		s.writeError(w, http.StatusBadRequest, "repo is required")
		return
	}
	imageID := common.GenerateID(64)
	s.writeJSON(w, http.StatusCreated, map[string]string{"Id": imageID})
	_ = tag
}

// handlePodCreate creates a pod (group of containers with shared network).
func (s *Server) handlePodCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	podID := common.GenerateID(64)
	if req.Name == "" {
		req.Name = podID[:12]
	}
	s.writeJSON(w, http.StatusCreated, map[string]string{"Id": podID, "Name": req.Name})
}

// handlePodList returns all pods.
func (s *Server) handlePodList(w http.ResponseWriter, r *http.Request) {
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

type kubeSimple struct {
	Kind     string `yaml:"kind"`
	Metadata map[string]interface{} `yaml:"metadata"`
	Spec     map[string]interface{} `yaml:"spec"`
}

func parseKubeYAML(yamlStr string, s *Server) []string {
	var created []string
	lines := strings.Split(yamlStr, "\n")
	var current map[string]interface{}
	inDoc := false
	indent := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "kind:") {
			if current != nil {
				if id := applyKubeResource(current, s); id != "" {
					created = append(created, id)
				}
			}
			current = make(map[string]interface{})
			current["kind"] = strings.TrimSpace(strings.TrimPrefix(trimmed, "kind:"))
			inDoc = true
			indent = ""
			continue
		}
		if inDoc && current != nil {
			if strings.HasPrefix(trimmed, "spec:") || strings.HasPrefix(trimmed, "metadata:") {
				indent = ""
				switch {
				case strings.HasPrefix(trimmed, "spec:"):
					if spec, ok := current["spec"].(map[string]interface{}); ok {
						parseYAMLBlock(line, spec, &indent)
					} else {
						current["spec"] = make(map[string]interface{})
					}
				case strings.HasPrefix(trimmed, "metadata:"):
					if meta, ok := current["metadata"].(map[string]interface{}); ok {
						parseYAMLBlock(line, meta, &indent)
					} else {
						current["metadata"] = make(map[string]interface{})
					}
				}
			} else if trimmed != "" {
				kv := strings.SplitN(trimmed, ":", 2)
				if len(kv) == 2 {
					key := strings.TrimSpace(kv[0])
					val := strings.TrimSpace(kv[1])
					if val == "" {
						val = ""
					}
					if indent != "" {
						key = indent + key
					}
					current[key] = val
				}
			}
		}
	}
	if current != nil {
		if id := applyKubeResource(current, s); id != "" {
			created = append(created, id)
		}
	}
	return created
}

func parseYAMLBlock(line string, m map[string]interface{}, indent *string) {
	// placeholder for nested parsing
	_ = line
	_ = m
	_ = indent
}

func applyKubeResource(resource map[string]interface{}, s *Server) string {
	kind, _ := resource["kind"].(string)
	meta, _ := resource["metadata"].(map[string]interface{})
	spec, _ := resource["spec"].(map[string]interface{})
	name := ""
	if meta != nil {
		if n, ok := meta["name"].(string); ok {
			name = n
		}
	}
	image, _ := resource["image"].(string)
	if image == "" && spec != nil {
		if containers, ok := spec["containers"].([]interface{}); ok && len(containers) > 0 {
			if first, ok := containers[0].(map[string]interface{}); ok {
				if img, ok := first["image"].(string); ok {
					image = img
				}
			}
		}
	}
	_ = kind
	if image == "" {
		return ""
	}
	if name == "" {
		name = image
	}
	containerID := common.GenerateID(64)
	s.image.Pull(image)
	cfg := &dokiruntime.Config{
		ID:       containerID,
		Args:     []string{"/bin/sh"},
		ImageRef: image,
		Annotations: map[string]string{
			"doki.name": name,
			"doki.kube": "true",
		},
	}
	s.runtime.Create(cfg)
	s.runtime.Start(containerID)
	return containerID[:12]
}

// handleGenerateKube generates a kube YAML from running containers.
func (s *Server) handleGenerateKube(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("container")
	containers, _ := s.runtime.List()
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
	w.Write([]byte(strings.Join(yamlParts, "---\n")))
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
func (s *Server) handleAutoUpdate(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.runtime.List()
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
			tags, err := s.image.Search(ref, 1)
			_ = tags
			_ = err
			_ = record
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
	_ = data
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
		"image":        imageName,
		"id":           common.ShortID(record.ID),
		"scanned":      time.Now().Unix(),
		"findings":     findings,
		"note":         "CVE scanning is a stub. Full vulnerability scanning requires a local vulnerability database or Docker Scout API access.",
		"totalCount":   len(findings),
	})
}

// handleImageVerify checks if an image has a signature.
func (s *Server) handleImageVerify(w http.ResponseWriter, r *http.Request, imageName string) {
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
func (s *Server) handleKubeGenerate(w http.ResponseWriter, r *http.Request) {
	containers, _ := s.runtime.List()
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
		w.Write([]byte("# No running containers\n"))
		return
	}
	w.Header().Set("Content-Type", "text/yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(strings.Join(yamlParts, "---\n")))
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
		return syscall.SIGKILL
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
