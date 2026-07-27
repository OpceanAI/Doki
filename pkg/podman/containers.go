package podman

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
)

// libpodContainer is the container shape podman's REST clients expect. It
// differs from Docker's: names are a list, state is under "State", and the
// timestamps are unix seconds.
type libpodContainer struct {
	ID         string            `json:"Id"`
	Names      []string          `json:"Names"`
	Image      string            `json:"Image"`
	ImageID    string            `json:"ImageID"`
	Command    []string          `json:"Command"`
	Created    int64             `json:"Created"`
	CreatedAt  string            `json:"CreatedAt"`
	StartedAt  int64             `json:"StartedAt"`
	ExitedAt   int64             `json:"ExitedAt"`
	ExitCode   int32             `json:"ExitCode"`
	Exited     bool              `json:"Exited"`
	State      string            `json:"State"`
	Status     string            `json:"Status"`
	Labels     map[string]string `json:"Labels"`
	Pid        int               `json:"Pid"`
	IsInfra    bool              `json:"IsInfra"`
	Namespaces map[string]string `json:"Namespaces"`
	Mounts     []string          `json:"Mounts"`
	PodName    string            `json:"PodName,omitempty"`
	Pod        string            `json:"Pod,omitempty"`
}

// containerName recovers the user-facing name the daemon stored in annotations,
// falling back to a short ID so a container is never nameless.
func containerName(state *dokiruntime.ContainerState) string {
	if state.Config != nil && state.Config.Annotations != nil {
		if n := state.Config.Annotations["doki.name"]; n != "" {
			return n
		}
	}
	if len(state.ID) > 12 {
		return state.ID[:12]
	}
	return state.ID
}

// libpodState maps Doki's internal container states onto podman's vocabulary.
// Podman says "configured" where Docker says "created", and "stopped" where
// Docker says "exited".
func libpodState(s common.ContainerState) string {
	switch s {
	case common.StateCreated:
		return "configured"
	case common.StateRunning:
		return "running"
	case common.StatePaused:
		return "paused"
	case common.StateExited:
		return "exited"
	default:
		return string(s)
	}
}

func toLibpodContainer(state *dokiruntime.ContainerState) libpodContainer {
	c := libpodContainer{
		ID:         state.ID,
		Names:      []string{containerName(state)},
		Created:    state.Created.Unix(),
		CreatedAt:  state.Created.Format(time.RFC3339),
		ExitCode:   int32(state.ExitCode),
		Exited:     state.Status == common.StateExited,
		State:      libpodState(state.Status),
		Status:     libpodState(state.Status),
		Pid:        state.Pid,
		Namespaces: map[string]string{},
		Mounts:     []string{},
		Labels:     map[string]string{},
		Command:    []string{},
	}
	if !state.Started.IsZero() {
		c.StartedAt = state.Started.Unix()
	}
	if !state.Finished.IsZero() {
		c.ExitedAt = state.Finished.Unix()
	}
	if state.Config != nil {
		c.Image = state.Config.ImageRef
		c.ImageID = state.Config.ImageDigest
		c.Command = state.Config.Args
		if state.Config.Labels != nil {
			c.Labels = state.Config.Labels
		}
		for _, m := range state.Config.Mounts {
			c.Mounts = append(c.Mounts, m.Target)
		}
	}
	return c
}

func (s *PodmanServer) handleContainersList(w http.ResponseWriter, r *http.Request) {
	if !s.requireRuntime(w) {
		return
	}
	states, err := s.runtime.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list containers failed")
		return
	}
	all := r.URL.Query().Get("all") == "true" || r.URL.Query().Get("all") == "1"
	out := make([]libpodContainer, 0, len(states))
	for _, st := range states {
		if !all && st.Status != common.StateRunning {
			continue
		}
		out = append(out, toLibpodContainer(st))
	}
	writeJSON(w, http.StatusOK, out)
}

// libpodCreateRequest is the subset of podman's SpecGenerator this shim
// understands. Fields it does not understand are ignored rather than rejected,
// because podman sends a very wide struct for even a trivial run.
type libpodCreateRequest struct {
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	Command    []string          `json:"command"`
	Entrypoint []string          `json:"entrypoint"`
	Env        map[string]string `json:"env"`
	WorkDir    string            `json:"work_dir"`
	User       string            `json:"user"`
	Hostname   string            `json:"hostname"`
	Labels     map[string]string `json:"labels"`
	Terminal   bool              `json:"terminal"`
	Stdin      bool              `json:"stdin"`
	Privileged bool              `json:"privileged"`
	Remove     bool              `json:"remove"`
	NetNS      struct {
		NSMode string `json:"nsmode"`
		Value  string `json:"value"`
	} `json:"netns"`
	PortMappings []struct {
		HostIP        string `json:"host_ip"`
		HostPort      uint16 `json:"host_port"`
		ContainerPort uint16 `json:"container_port"`
		Protocol      string `json:"protocol"`
	} `json:"portmappings"`
	Mounts []struct {
		Type        string   `json:"Type"`
		Source      string   `json:"Source"`
		Destination string   `json:"Destination"`
		Options     []string `json:"Options"`
	} `json:"mounts"`
	Volumes []struct {
		Name    string   `json:"Name"`
		Dest    string   `json:"Dest"`
		Options []string `json:"Options"`
	} `json:"volumes"`
	RestartPolicy  string   `json:"restart_policy"`
	CapAdd         []string `json:"cap_add"`
	CapDrop        []string `json:"cap_drop"`
	ResourceLimits *struct {
		CPU *struct {
			Shares *uint64 `json:"shares"`
			Period *uint64 `json:"period"`
			Quota  *int64  `json:"quota"`
			Cpus   string  `json:"cpus"`
		} `json:"cpu"`
		Memory *struct {
			Limit *int64 `json:"limit"`
			Swap  *int64 `json:"swap"`
		} `json:"memory"`
		Pids *struct {
			Limit int64 `json:"limit"`
		} `json:"pids"`
	} `json:"resource_limits"`
}

func (s *PodmanServer) handleContainersCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if !s.requireRuntime(w) || !s.requireImages(w) {
		return
	}
	var req libpodCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}

	if req.Name != "" {
		if states, err := s.runtime.List(); err == nil {
			for _, st := range states {
				if containerName(st) == req.Name {
					writeError(w, http.StatusConflict, "container name already in use: "+req.Name)
					return
				}
			}
		}
	}

	if !s.images.Exists(req.Image) {
		if _, err := s.images.Pull(req.Image); err != nil {
			writeError(w, http.StatusInternalServerError, "pull image failed")
			return
		}
	}
	imgRecord, err := s.images.Get(req.Image)
	if err != nil {
		writeError(w, http.StatusNotFound, "image not found: "+req.Image)
		return
	}

	var imgOCI *dokiruntime.ImageOCIConfig
	if imgRecord.Config != nil {
		imgOCI = &dokiruntime.ImageOCIConfig{
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
	}

	cfg := &dokiruntime.Config{
		ID:          common.GenerateID(64),
		Args:        dokiruntime.BuildCommand(req.Entrypoint, req.Command, imgOCI),
		Env:         envMapToSlice(req.Env),
		Tty:         req.Terminal,
		Interactive: req.Stdin,
		Privileged:  req.Privileged,
		Hostname:    req.Hostname,
		Labels:      req.Labels,
		ImageRef:    req.Image,
		ImageDigest: imgRecord.ID,
		ImageConfig: imgOCI,
		CapAdd:      req.CapAdd,
		CapDrop:     req.CapDrop,
	}

	cfg.Cwd = req.WorkDir
	if cfg.Cwd == "" && imgOCI != nil {
		cfg.Cwd = imgOCI.WorkingDir
	}
	cfg.User = req.User
	if cfg.User == "" && imgOCI != nil {
		cfg.User = imgOCI.User
	}
	if req.Name != "" {
		cfg.Annotations = map[string]string{"doki.name": req.Name}
	}
	if req.RestartPolicy != "" {
		cfg.RestartPolicy = common.RestartPolicy(req.RestartPolicy)
	}
	if req.NetNS.NSMode != "" {
		cfg.NetworkMode = common.NetworkMode(req.NetNS.NSMode)
		if req.NetNS.Value != "" {
			cfg.NetworkMode = common.NetworkMode(req.NetNS.Value)
		}
	}
	if layers, err := s.images.GetLayerPaths(req.Image); err == nil {
		cfg.ImageLayers = layers
	}

	for _, m := range req.Mounts {
		if m.Source == "" || m.Destination == "" {
			continue
		}
		// The same rule the Docker surface enforces: a bind of the host root or
		// a sensitive system directory is a host takeover, not a mount.
		if err := validateBindSource(m.Type, m.Source); err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		cfg.Mounts = append(cfg.Mounts, common.Mount{
			Type:     common.MountType(strings.ToLower(m.Type)),
			Source:   m.Source,
			Target:   m.Destination,
			ReadOnly: hasOption(m.Options, "ro"),
		})
	}
	for _, v := range req.Volumes {
		if v.Name == "" || v.Dest == "" {
			continue
		}
		cfg.Mounts = append(cfg.Mounts, common.Mount{
			Type:     common.MountVolume,
			Source:   v.Name,
			Target:   v.Dest,
			ReadOnly: hasOption(v.Options, "ro"),
		})
	}

	for _, p := range req.PortMappings {
		if p.ContainerPort == 0 || p.HostPort == 0 {
			continue
		}
		proto := common.PortProtocol(strings.ToLower(p.Protocol))
		if proto == "" {
			proto = common.ProtocolTCP
		}
		if !req.Privileged && p.HostPort < 1024 {
			continue
		}
		cfg.Ports = append(cfg.Ports, common.Port{
			PrivatePort: p.ContainerPort,
			PublicPort:  p.HostPort,
			Type:        proto,
			IP:          p.HostIP,
		})
	}

	if rl := req.ResourceLimits; rl != nil {
		cfg.Resources = &dokiruntime.Resources{}
		if rl.CPU != nil {
			if rl.CPU.Shares != nil {
				cfg.Resources.CPUShares = common.SafeInt64FromUint64(*rl.CPU.Shares)
			}
			if rl.CPU.Period != nil {
				cfg.Resources.CPUPeriod = common.SafeInt64FromUint64(*rl.CPU.Period)
			}
			if rl.CPU.Quota != nil {
				cfg.Resources.CPUQuota = *rl.CPU.Quota
			}
			cfg.Resources.CpusetCpus = rl.CPU.Cpus
		}
		if rl.Memory != nil {
			if rl.Memory.Limit != nil {
				cfg.Resources.Memory = *rl.Memory.Limit
			}
			if rl.Memory.Swap != nil {
				cfg.Resources.MemorySwap = *rl.Memory.Swap
			}
		}
		if rl.Pids != nil {
			cfg.Resources.PidsLimit = rl.Pids.Limit
		}
	}

	if _, err := s.runtime.Create(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "create container failed")
		return
	}
	s.publish("create", cfg.ID, req.Name)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"Id":       cfg.ID,
		"Warnings": []string{},
	})
}

func (s *PodmanServer) handleContainersPrune(w http.ResponseWriter, _ *http.Request) {
	if !s.requireRuntime(w) {
		return
	}
	states, err := s.runtime.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list containers failed")
		return
	}
	report := make([]map[string]interface{}, 0)
	for _, st := range states {
		if st.Status != common.StateExited && st.Status != common.StateCreated {
			continue
		}
		entry := map[string]interface{}{"Id": st.ID, "Size": 0}
		if err := s.runtime.Delete(st.ID, false); err != nil {
			entry["Err"] = "remove failed"
		} else {
			s.publish("remove", st.ID, containerName(st))
		}
		report = append(report, entry)
	}
	writeJSON(w, http.StatusOK, report)
}

// resolveContainer accepts either a full/partial ID or a user-assigned name,
// the way podman's clients address containers.
func (s *PodmanServer) resolveContainer(nameOrID string) (*dokiruntime.ContainerState, error) {
	if st, err := s.runtime.State(nameOrID); err == nil {
		return st, nil
	}
	states, err := s.runtime.List()
	if err != nil {
		return nil, fmt.Errorf("no such container: %s", nameOrID)
	}
	for _, st := range states {
		if containerName(st) == nameOrID || strings.HasPrefix(st.ID, nameOrID) {
			return st, nil
		}
	}
	return nil, fmt.Errorf("no such container: %s", nameOrID)
}

func (s *PodmanServer) handleContainersDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/containers/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}
	if !s.requireRuntime(w) {
		return
	}

	// A pod and a container are different objects; resolving a container ID
	// against the pod store (as this dispatcher used to) conflates the two.
	switch action {
	case "", "json":
		st, err := s.resolveContainer(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.inspectContainer(st))
		case http.MethodDelete:
			force := r.URL.Query().Get("force") == "true"
			if err := s.runtime.Delete(st.ID, force); err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			s.publish("remove", st.ID, containerName(st))
			writeJSON(w, http.StatusOK, []map[string]string{{"Id": st.ID}})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "logs":
		s.handleContainerLogs(w, r, id)
	case "exists":
		if _, err := s.resolveContainer(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.handleContainerAction(w, id, action, r)
	}
}

func (s *PodmanServer) inspectContainer(st *dokiruntime.ContainerState) map[string]interface{} {
	out := map[string]interface{}{
		"Id":      st.ID,
		"Name":    containerName(st),
		"Created": st.Created.Format(time.RFC3339Nano),
		"Path":    "",
		"Args":    []string{},
		"State": map[string]interface{}{
			"Status":     libpodState(st.Status),
			"Running":    st.Status == common.StateRunning,
			"Paused":     st.Status == common.StatePaused,
			"Pid":        st.Pid,
			"ExitCode":   st.ExitCode,
			"StartedAt":  st.Started.Format(time.RFC3339Nano),
			"FinishedAt": st.Finished.Format(time.RFC3339Nano),
		},
		"Image":        "",
		"ImageName":    "",
		"Mounts":       []interface{}{},
		"RestartCount": st.RestartCount,
	}
	if st.Config != nil {
		out["Image"] = st.Config.ImageDigest
		out["ImageName"] = st.Config.ImageRef
		if len(st.Config.Args) > 0 {
			out["Path"] = st.Config.Args[0]
			out["Args"] = st.Config.Args[1:]
		}
		out["Config"] = map[string]interface{}{
			"Hostname":   st.Config.Hostname,
			"User":       st.Config.User,
			"Env":        st.Config.Env,
			"Cmd":        st.Config.Args,
			"WorkingDir": st.Config.Cwd,
			"Labels":     st.Config.Labels,
			"Tty":        st.Config.Tty,
			"OpenStdin":  st.Config.Interactive,
		}
	}
	return out
}

func (s *PodmanServer) handleContainerLogs(w http.ResponseWriter, r *http.Request, nameOrID string) {
	st, err := s.resolveContainer(nameOrID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	tail := 0
	if v := r.URL.Query().Get("tail"); v != "" && v != "all" {
		if n, err := strconv.Atoi(v); err == nil {
			tail = n
		}
	}
	logs, err := s.runtime.GetLogs(st.ID, tail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read logs failed")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(logs))
}

func (s *PodmanServer) handleContainerAction(w http.ResponseWriter, nameOrID, action string, r *http.Request) {
	// A pod shares this URL space in podman, so keep honoring pod actions when
	// the identifier really is a pod.
	if s.podMgr.Exists(nameOrID) {
		switch action {
		case "start", "stop", "kill", "restart", "pause", "unpause":
			s.handlePodAction(w, nameOrID, action, r)
			return
		}
	}
	if !s.requireRuntime(w) {
		return
	}
	st, err := s.resolveContainer(nameOrID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	timeout := 10
	if v := r.URL.Query().Get("t"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			timeout = n
		}
	}

	switch action {
	case "start":
		err = s.runtime.Start(st.ID)
	case "stop":
		err = s.runtime.Stop(st.ID, timeout)
	case "restart":
		if err = s.runtime.Stop(st.ID, timeout); err == nil {
			err = s.runtime.Start(st.ID)
		}
	case "kill":
		sig := r.URL.Query().Get("signal")
		if sig == "" {
			sig = "SIGKILL"
		}
		err = s.runtime.Kill(st.ID, parseSignalName(sig))
	case "pause":
		err = s.runtime.Pause(st.ID)
	case "unpause":
		err = s.runtime.Unpause(st.ID)
	case "wait":
		s.handleContainerWait(w, st.ID)
		return
	default:
		writeError(w, http.StatusNotFound, "unsupported container action: "+action)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publish(action, st.ID, containerName(st))
	w.WriteHeader(http.StatusNoContent)
}

func (s *PodmanServer) handleContainerWait(w http.ResponseWriter, id string) {
	for {
		st, err := s.runtime.State(id)
		if err != nil {
			writeError(w, http.StatusNotFound, "no such container")
			return
		}
		if st.Status == common.StateExited {
			writeJSON(w, http.StatusOK, st.ExitCode)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// publish emits a container lifecycle event on the daemon's bus so that
// /libpod/events reports what actually happened.
func (s *PodmanServer) publish(action, id, name string) {
	if s.events == nil {
		return
	}
	s.events.PublishContainer(eventsAction(action), id, name, nil)
}

func envMapToSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func hasOption(opts []string, want string) bool {
	for _, o := range opts {
		if strings.EqualFold(o, want) {
			return true
		}
	}
	return false
}

func parseSignalName(s string) syscall.Signal {
	if n, err := strconv.Atoi(s); err == nil {
		return syscall.Signal(n)
	}
	switch strings.ToUpper(strings.TrimPrefix(strings.ToUpper(s), "SIG")) {
	case "HUP":
		return syscall.SIGHUP
	case "INT":
		return syscall.SIGINT
	case "QUIT":
		return syscall.SIGQUIT
	case "TERM":
		return syscall.SIGTERM
	case "USR1":
		return syscall.SIGUSR1
	case "USR2":
		return syscall.SIGUSR2
	default:
		return syscall.SIGKILL
	}
}
