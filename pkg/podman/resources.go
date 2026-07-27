package podman

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/events"
	"github.com/OpceanAI/Doki/pkg/network"
)

// validateBindSource applies the same host-path policy the Docker surface
// enforces. Bind sources must be absolute, traversal-free, and outside the
// system directories that would hand over the host.
func validateBindSource(mountType, source string) error {
	if !strings.EqualFold(mountType, "bind") {
		return nil
	}
	if !filepath.IsAbs(source) || filepath.Clean(source) != source {
		return fmt.Errorf("invalid bind mount source: must be an absolute path without traversal")
	}
	if common.IsSensitiveBindSource(source) {
		return fmt.Errorf("bind mount source not allowed: %s", source)
	}
	return nil
}

// eventsAction maps a libpod lifecycle verb onto the daemon's event vocabulary.
func eventsAction(action string) events.Action {
	switch action {
	case "create":
		return events.ActionCreate
	case "start":
		return events.ActionStart
	case "stop":
		return events.ActionStop
	case "kill":
		return events.ActionKill
	case "pause":
		return events.ActionPause
	case "unpause":
		return events.ActionUnpause
	case "restart":
		return events.ActionRestart
	case "remove":
		return events.ActionRemove
	default:
		return events.Action(action)
	}
}

// --- Images (P3) -----------------------------------------------------------

func (s *PodmanServer) handleImagesList(w http.ResponseWriter, _ *http.Request) {
	if !s.requireImages(w) {
		return
	}
	imgs, err := s.images.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list images failed")
		return
	}
	out := make([]map[string]interface{}, 0, len(imgs))
	for _, img := range imgs {
		names := img.RepoTags
		if names == nil {
			names = []string{}
		}
		out = append(out, map[string]interface{}{
			"Id":          img.ID,
			"Names":       names,
			"RepoTags":    names,
			"RepoDigests": img.RepoDigests,
			"Created":     img.Created,
			"Size":        img.Size,
			"VirtualSize": img.VirtualSize,
			"Labels":      img.Labels,
			"Dangling":    len(names) == 0,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *PodmanServer) handleImagesPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if !s.requireImages(w) {
		return
	}
	ref := r.URL.Query().Get("reference")
	if ref == "" {
		ref = r.URL.Query().Get("image")
	}
	if ref == "" {
		writeError(w, http.StatusBadRequest, "reference is required")
		return
	}
	rec, err := s.images.Pull(ref)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pull failed: "+ref)
		return
	}
	if s.events != nil {
		s.events.PublishImage(events.ActionPull, rec.ID, map[string]string{"name": ref})
	}
	// libpod's pull endpoint streams NDJSON and closes with the resolved IDs.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	_ = enc.Encode(map[string]interface{}{"stream": "Pulled " + ref + "\n"})
	_ = enc.Encode(map[string]interface{}{"images": []string{rec.ID}, "id": rec.ID})
}

func (s *PodmanServer) handleImagesPrune(w http.ResponseWriter, _ *http.Request) {
	if !s.requireImages(w) {
		return
	}
	removed, err := s.images.Prune()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "prune failed")
		return
	}
	out := make([]map[string]interface{}, 0, len(removed))
	for _, id := range removed {
		out = append(out, map[string]interface{}{"Id": id, "Size": 0})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *PodmanServer) handleImagesSearch(w http.ResponseWriter, r *http.Request) {
	if !s.requireImages(w) {
		return
	}
	term := r.URL.Query().Get("term")
	if term == "" {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	limit := 25
	results, err := s.images.Search(term, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	out := make([]map[string]interface{}, 0, len(results))
	for _, res := range results {
		out = append(out, map[string]interface{}{
			"Index":       "docker.io",
			"Name":        res.Name,
			"Description": res.Description,
			"Stars":       res.StarCount,
			"Official":    res.IsOfficial,
			"Automated":   res.IsAutomated,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *PodmanServer) handleImagesDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/images/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}
	if !s.requireImages(w) {
		return
	}
	switch action {
	case "", "json":
		switch r.Method {
		case http.MethodGet:
			cfg, err := s.images.Inspect(id)
			if err != nil {
				writeError(w, http.StatusNotFound, "no such image: "+id)
				return
			}
			rec, _ := s.images.Get(id)
			out := map[string]interface{}{"Id": id, "Config": cfg}
			if rec != nil {
				out["Id"] = rec.ID
				out["RepoTags"] = rec.RepoTags
				out["Size"] = rec.Size
				out["Created"] = rec.Created
			}
			writeJSON(w, http.StatusOK, out)
		case http.MethodDelete:
			if err := s.images.Remove(id); err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			if s.events != nil {
				s.events.PublishImage(events.ActionRemove, id, nil)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"Untagged": []string{id},
				"Deleted":  []string{id},
			})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "exists":
		if !s.images.Exists(id) {
			writeError(w, http.StatusNotFound, "no such image: "+id)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "history":
		hist, err := s.images.History(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, hist)
	default:
		s.handleImageAction(w, id, action, r)
	}
}

func (s *PodmanServer) handleImageAction(w http.ResponseWriter, id, action string, r *http.Request) {
	switch action {
	case "tag":
		target := r.URL.Query().Get("repo")
		if tag := r.URL.Query().Get("tag"); tag != "" && target != "" {
			target += ":" + tag
		}
		if target == "" {
			writeError(w, http.StatusBadRequest, "repo is required")
			return
		}
		if err := s.images.Tag(id, target); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if s.events != nil {
			s.events.PublishImage(events.ActionTag, id, map[string]string{"name": target})
		}
		w.WriteHeader(http.StatusCreated)
	case "untag":
		// Untagging the last reference is a removal in libpod's model.
		if err := s.images.Remove(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
	case "push":
		if err := s.images.Push(id); err != nil {
			writeError(w, http.StatusInternalServerError, "push failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"stream": "pushed " + id + "\n"})
	case "get", "save":
		w.Header().Set("Content-Type", "application/x-tar")
		w.WriteHeader(http.StatusOK)
		if err := s.images.Export(id, w); err != nil {
			// Headers are already committed; the truncated stream is the error
			// signal, and the daemon log carries the detail.
			return
		}
	default:
		writeError(w, http.StatusNotFound, "unsupported image action: "+action)
	}
}

// --- Volumes (P5) ----------------------------------------------------------

func (s *PodmanServer) requireVolumes(w http.ResponseWriter) bool {
	if s.volumes == nil {
		writeError(w, http.StatusServiceUnavailable, "volume store not available in this build")
		return false
	}
	return true
}

func toLibpodVolume(v *common.VolumeInfo) map[string]interface{} {
	return map[string]interface{}{
		"Name":       v.Name,
		"Driver":     v.Driver,
		"Mountpoint": v.Mountpoint,
		"CreatedAt":  v.CreatedAt,
		"Labels":     v.Labels,
		"Options":    v.Options,
		"Scope":      v.Scope,
	}
}

func (s *PodmanServer) handleVolumesList(w http.ResponseWriter, _ *http.Request) {
	if !s.requireVolumes(w) {
		return
	}
	vols := s.volumes.List()
	out := make([]map[string]interface{}, 0, len(vols))
	for _, v := range vols {
		out = append(out, toLibpodVolume(v))
	}
	// libpod returns a bare array here, unlike Docker's {"Volumes": [...]}.
	writeJSON(w, http.StatusOK, out)
}

func (s *PodmanServer) handleVolumesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if !s.requireVolumes(w) {
		return
	}
	var body struct {
		Name    string            `json:"Name"`
		Driver  string            `json:"Driver"`
		Labels  map[string]string `json:"Labels"`
		Options map[string]string `json:"Options"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" {
		body.Name = common.GenerateID(32)
	}
	vol, err := s.volumes.Create(body.Name, body.Driver, body.Options, body.Labels)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.events != nil {
		s.events.PublishVolume(events.ActionCreate, vol.Name)
	}
	writeJSON(w, http.StatusCreated, toLibpodVolume(vol))
}

func (s *PodmanServer) handleVolumesPrune(w http.ResponseWriter, _ *http.Request) {
	if !s.requireVolumes(w) {
		return
	}
	removed, err := s.volumes.Prune(s.referencedVolumes())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]interface{}, 0, len(removed))
	for _, name := range removed {
		out = append(out, map[string]interface{}{"Id": name, "Size": 0})
	}
	writeJSON(w, http.StatusOK, out)
}

// referencedVolumes collects volume names currently mounted by a container so
// prune cannot delete storage out from under a running workload.
func (s *PodmanServer) referencedVolumes() map[string]bool {
	refs := make(map[string]bool)
	if s.runtime == nil {
		return refs
	}
	states, err := s.runtime.List()
	if err != nil {
		return refs
	}
	for _, st := range states {
		if st.Config == nil {
			continue
		}
		for _, m := range st.Config.Mounts {
			if m.Type == common.MountVolume && m.Source != "" {
				refs[m.Source] = true
			}
		}
	}
	return refs
}

func (s *PodmanServer) handleVolumesDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/volumes/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "volume not found")
		return
	}
	if !s.requireVolumes(w) {
		return
	}
	switch action {
	case "", "json":
		switch r.Method {
		case http.MethodGet:
			vol, err := s.volumes.Get(id)
			if err != nil {
				writeError(w, http.StatusNotFound, "no such volume: "+id)
				return
			}
			writeJSON(w, http.StatusOK, toLibpodVolume(vol))
		case http.MethodDelete:
			if !r.URL.Query().Has("force") && s.referencedVolumes()[id] {
				writeError(w, http.StatusConflict, "volume is in use by a container")
				return
			}
			if err := s.volumes.Remove(id); err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			if s.events != nil {
				s.events.PublishVolume(events.ActionRemove, id)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "exists":
		if _, err := s.volumes.Get(id); err != nil {
			writeError(w, http.StatusNotFound, "no such volume: "+id)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusNotFound, "unsupported volume action: "+action)
	}
}

// --- Networks (P5) ---------------------------------------------------------

func (s *PodmanServer) requireNetwork(w http.ResponseWriter) bool {
	if s.network == nil {
		writeError(w, http.StatusServiceUnavailable, "network manager not available in this build")
		return false
	}
	return true
}

func toLibpodNetwork(n *common.NetworkInfo) map[string]interface{} {
	subnets := make([]map[string]interface{}, 0)
	for _, cfg := range n.IPAM.Config {
		subnets = append(subnets, map[string]interface{}{
			"subnet":  cfg.Subnet,
			"gateway": cfg.Gateway,
		})
	}
	return map[string]interface{}{
		"name":              n.Name,
		"id":                n.ID,
		"driver":            n.Driver,
		"network_interface": n.Name,
		"created":           n.Created,
		"subnets":           subnets,
		"ipv6_enabled":      n.EnableIPv6,
		"internal":          n.Internal,
		"dns_enabled":       true,
		"labels":            n.Labels,
		"options":           n.Options,
		"ipam_options":      map[string]string{"driver": n.IPAM.Driver},
	}
}

func (s *PodmanServer) handleNetworksList(w http.ResponseWriter, _ *http.Request) {
	if !s.requireNetwork(w) {
		return
	}
	nets, err := s.network.ListNetworks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list networks failed")
		return
	}
	out := make([]map[string]interface{}, 0, len(nets))
	for i := range nets {
		out = append(out, toLibpodNetwork(&nets[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *PodmanServer) handleNetworksCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if !s.requireNetwork(w) {
		return
	}
	var body struct {
		Name     string            `json:"name"`
		Driver   string            `json:"driver"`
		Internal bool              `json:"internal"`
		IPv6     bool              `json:"ipv6_enabled"`
		Labels   map[string]string `json:"labels"`
		Options  map[string]string `json:"options"`
		Subnets  []struct {
			Subnet  string `json:"subnet"`
			Gateway string `json:"gateway"`
		} `json:"subnets"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "network name is required")
		return
	}
	cfg := &network.NetworkConfig{
		Name:       body.Name,
		Driver:     body.Driver,
		Internal:   body.Internal,
		EnableIPv6: body.IPv6,
		Labels:     body.Labels,
		Options:    body.Options,
	}
	if len(body.Subnets) > 0 {
		cfg.Subnet = body.Subnets[0].Subnet
		cfg.Gateway = body.Subnets[0].Gateway
	}
	net, err := s.network.CreateNetwork(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.events != nil {
		s.events.PublishNetwork(events.ActionCreate, net.ID, net.Name)
	}
	info, err := s.network.Inspect(net.Name)
	if err != nil {
		writeJSON(w, http.StatusCreated, map[string]interface{}{"name": net.Name, "id": net.ID})
		return
	}
	writeJSON(w, http.StatusCreated, toLibpodNetwork(info))
}

func (s *PodmanServer) handleNetworksPrune(w http.ResponseWriter, _ *http.Request) {
	if !s.requireNetwork(w) {
		return
	}
	removed, err := s.network.Prune()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]interface{}, 0, len(removed))
	for _, name := range removed {
		out = append(out, map[string]interface{}{"Name": name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *PodmanServer) handleNetworksDispatch(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseDispatch("/libpod/networks/", r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "network not found")
		return
	}
	if !s.requireNetwork(w) {
		return
	}
	switch action {
	case "", "json":
		switch r.Method {
		case http.MethodGet:
			info, err := s.network.Inspect(id)
			if err != nil {
				writeError(w, http.StatusNotFound, "no such network: "+id)
				return
			}
			writeJSON(w, http.StatusOK, toLibpodNetwork(info))
		case http.MethodDelete:
			if err := s.network.RemoveNetwork(id); err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			if s.events != nil {
				s.events.PublishNetwork(events.ActionRemove, id, id)
			}
			writeJSON(w, http.StatusOK, []map[string]string{{"Name": id}})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "exists":
		if _, err := s.network.Inspect(id); err != nil {
			writeError(w, http.StatusNotFound, "no such network: "+id)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "connect", "disconnect":
		s.handleNetworkConnect(w, r, id, action)
	default:
		writeError(w, http.StatusNotFound, "unsupported network action: "+action)
	}
}

func (s *PodmanServer) handleNetworkConnect(w http.ResponseWriter, r *http.Request, netID, action string) {
	if !s.requireRuntime(w) {
		return
	}
	var body struct {
		Container string `json:"container"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPodmanJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := s.resolveContainer(body.Container)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if action == "connect" {
		err = s.network.Connect(netID, st.ID, "", nil, nil, st.Pid)
	} else {
		err = s.network.Disconnect(netID, st.ID, st.Pid)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Events (P4) -----------------------------------------------------------

// handleEvents streams the daemon's real event bus. It used to emit synthetic
// "ping" events on a ticker, which looked alive but told a client nothing about
// what the engine was actually doing.
func (s *PodmanServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	if s.events == nil {
		writeError(w, http.StatusServiceUnavailable, "event bus not available in this build")
		return
	}

	var filter events.Filter
	if raw := r.URL.Query().Get("filters"); raw != "" {
		f, err := events.FilterFromJSON([]byte(raw))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid filters")
			return
		}
		filter = f
	}

	sub := s.events.SubscribeContext(r.Context(), filter, 128)
	defer func() { _ = sub.Close() }()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.Channel():
			if !ok {
				return
			}
			if err := enc.Encode(toLibpodEvent(ev)); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func toLibpodEvent(ev events.Event) map[string]interface{} {
	name := ""
	if ev.Actor.Attributes != nil {
		name = ev.Actor.Attributes["name"]
	}
	return map[string]interface{}{
		"ID":         ev.Actor.ID,
		"Image":      ev.From,
		"Name":       name,
		"Status":     string(ev.Action),
		"Type":       string(ev.Type),
		"Action":     string(ev.Action),
		"Attributes": ev.Actor.Attributes,
		"time":       ev.Time,
		"timeNano":   ev.TimeNano,
	}
}

// --- system/df (P5) --------------------------------------------------------

func (s *PodmanServer) handleSystemDf(w http.ResponseWriter, _ *http.Request) {
	out := map[string]interface{}{
		"Images":     []interface{}{},
		"Containers": []interface{}{},
		"Volumes":    []interface{}{},
	}
	if s.images != nil {
		if imgs, err := s.images.List(); err == nil {
			rows := make([]map[string]interface{}, 0, len(imgs))
			for _, img := range imgs {
				tag := "<none>"
				if len(img.RepoTags) > 0 {
					tag = img.RepoTags[0]
				}
				rows = append(rows, map[string]interface{}{
					"Repository": tag,
					"Tag":        tag,
					"ImageID":    img.ID,
					"Created":    time.Unix(img.Created, 0).Format(time.RFC3339),
					"Size":       img.Size,
				})
			}
			out["Images"] = rows
		}
	}
	if s.runtime != nil {
		if states, err := s.runtime.List(); err == nil {
			rows := make([]map[string]interface{}, 0, len(states))
			for _, st := range states {
				rows = append(rows, map[string]interface{}{
					"ContainerID": st.ID,
					"Names":       containerName(st),
					"Status":      libpodState(st.Status),
					"Created":     st.Created.Format(time.RFC3339),
					"Size":        0,
				})
			}
			out["Containers"] = rows
		}
	}
	if s.volumes != nil {
		vols := s.volumes.List()
		rows := make([]map[string]interface{}, 0, len(vols))
		for _, v := range vols {
			rows = append(rows, map[string]interface{}{
				"VolumeName": v.Name,
				"Links":      0,
				"Size":       0,
			})
		}
		out["Volumes"] = rows
	}
	writeJSON(w, http.StatusOK, out)
}
