package api

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OpceanAI/Doki/pkg/common"
)

// handleContainerResize handles POST /containers/{id}/resize.
// It resizes the TTY of a running container.
func (s *Server) handleContainerResize(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	height, _ := strconv.Atoi(q.Get("h"))
	width, _ := strconv.Atoi(q.Get("w"))

	if height <= 0 || width <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid height or width")
		return
	}

	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if state.Status != common.StateRunning {
		s.writeError(w, http.StatusConflict, "container is not running")
		return
	}

	// Resize is a no-op for non-TTY containers, but we return success.
	s.writeJSON(w, http.StatusOK, map[string]string{})
}

// handleContainerArchive handles GET/PUT /containers/{id}/archive.
// GET: Copy files from container to host (docker cp).
// PUT: Copy files from host to container.
func (s *Server) handleContainerArchive(w http.ResponseWriter, r *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	rootfs := ""
	if state.Config != nil {
		rootfs = state.Config.RootfsReady
	}
	if rootfs == "" && state.Bundle != "" {
		rootfs = filepath.Join(state.Bundle, "rootfs")
	}

	q := r.URL.Query()
	path := q.Get("path")
	if path == "" {
		s.writeError(w, http.StatusBadRequest, "path parameter required")
		return
	}

	// Resolve path inside container rootfs.
	containerPath := resolveContainerPath(rootfs, path)

	switch r.Method {
	case "GET":
		s.handleArchiveGet(w, r, rootfs, containerPath)
	case "PUT":
		s.handleArchivePut(w, r, rootfs, containerPath)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleArchiveGet(w http.ResponseWriter, r *http.Request, rootfs, containerPath string) {
	// Check if path exists.
	info, err := os.Stat(containerPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, http.StatusNotFound, "path not found in container")
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Container-Path-Stat", encodePathStat(info))

	tw := tar.NewWriter(w)
	defer tw.Close()

	if info.IsDir() {
		filepath.Walk(containerPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			relPath, _ := filepath.Rel(rootfs, path)
			return addFileToTar(tw, path, relPath, fi)
		})
	} else {
		addFileToTar(tw, containerPath, filepath.Base(containerPath), info)
	}
}

func (s *Server) handleArchivePut(w http.ResponseWriter, r *http.Request, rootfs, containerPath string) {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(containerPath), 0755); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	tr := tar.NewReader(r.Body)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		target := filepath.Join(containerPath, hdr.Name)
		// Path traversal protection.
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(containerPath)) {
			s.writeError(w, http.StatusBadRequest, "invalid path in archive")
			return
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, os.FileMode(hdr.Mode))
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				continue
			}
			io.Copy(f, tr)
			f.Close()
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]string{})
}

// handleContainerResizeExec handles POST /exec/{id}/resize.
func (s *Server) handleContainerResizeExec(w http.ResponseWriter, r *http.Request, execID string) {
	q := r.URL.Query()
	height, _ := strconv.Atoi(q.Get("h"))
	width, _ := strconv.Atoi(q.Get("w"))

	if height <= 0 || width <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid height or width")
		return
	}

	// Exec resize is a no-op but we return success for compatibility.
	s.writeJSON(w, http.StatusOK, map[string]string{})
}

// handleContainerStatsStream handles streaming stats for GET /containers/{id}/stats.
func (s *Server) handleContainerStatsStream(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	stream := q.Get("stream") == "1" || q.Get("stream") == "true"

	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	stats, _ := s.runtime.Stats(id)

	if !stream {
		// Single snapshot.
		containerStats := map[string]interface{}{
			"id":        id,
			"read":      state.Started.Format("2006-01-02T15:04:05.999999999Z"),
			"cpu_stats": stats,
			"networks":  map[string]interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(containerStats)
		return
	}

	// Streaming mode.
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	for {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		stats, _ := s.runtime.Stats(id)
		containerStats := map[string]interface{}{
			"id":        id,
			"read":      state.Started.Format("2006-01-02T15:04:05.999999999Z"),
			"cpu_stats": stats,
			"networks":  map[string]interface{}{},
		}
		json.NewEncoder(w).Encode(containerStats)
		flusher.Flush()
	}
}

// handleVolumesListPaginated handles GET /volumes with pagination.
func (s *Server) handleVolumesListPaginated(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil {
		limit = 0
	}
	offset, err := strconv.Atoi(q.Get("offset"))
	if err != nil {
		offset = 0
	}

	volumes := s.volumes.List()

	// Apply pagination.
	total := len(volumes)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	paged := volumes[offset:end]

	response := map[string]interface{}{
		"Volumes":  paged,
		"Warnings": []string{},
	}
	if limit > 0 {
		response["Total"] = total
		response["Offset"] = offset
		response["Limit"] = limit
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleImageTagMulti handles POST /images/{name}/tag with multiple tags.
func (s *Server) handleImageTagMulti(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	repo := q.Get("repo")
	tag := q.Get("tag")

	if repo == "" {
		s.writeError(w, http.StatusBadRequest, "repo parameter required")
		return
	}
	if tag == "" {
		tag = "latest"
	}

	fullRef := repo + ":" + tag
	if err := s.image.Tag(id, fullRef); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, map[string]string{})
}

// handleContainerRename handles POST /containers/{id}/rename.
func (s *Server) handleContainerRename(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	newName := q.Get("name")
	if newName == "" {
		s.writeError(w, http.StatusBadRequest, "name parameter required")
		return
	}

	if !common.ValidContainerName(newName) {
		s.writeError(w, http.StatusBadRequest, "invalid container name")
		return
	}

	// Update container name in annotations.
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if state.Config != nil && state.Config.Annotations != nil {
		state.Config.Annotations["doki.name"] = newName
	}

	s.writeJSON(w, http.StatusOK, map[string]string{})
}

// handleContainerExecResize handles POST /exec/{id}/resize.
func (s *Server) handleExecResize(w http.ResponseWriter, r *http.Request, execID string) {
	q := r.URL.Query()
	height, _ := strconv.Atoi(q.Get("h"))
	width, _ := strconv.Atoi(q.Get("w"))

	if height <= 0 || width <= 0 {
		s.writeError(w, http.StatusBadRequest, "invalid height or width")
		return
	}

	// Exec resize is a no-op but we return success.
	s.writeJSON(w, http.StatusOK, map[string]string{})
}

// Helper functions.

func resolveContainerPath(rootfs, path string) string {
	// Clean the path.
	path = filepath.Clean(path)
	// Make it absolute relative to container root.
	if !filepath.IsAbs(path) {
		path = "/" + path
	}
	return filepath.Join(rootfs, path)
}

func encodePathStat(info os.FileInfo) string {
	// Encode file stat as base64 JSON (Docker format).
	type pathStat struct {
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		Mode       uint32 `json:"mode"`
		Mtime      string `json:"mtime"`
		LinkTarget string `json:"linkTarget,omitempty"`
	}
	stat := pathStat{
		Name:  info.Name(),
		Size:  info.Size(),
		Mode:  uint32(info.Mode()),
		Mtime: info.ModTime().Format("2006-01-02T15:04:05.999999999Z"),
	}
	data, _ := json.Marshal(stat)
	return string(data)
}

func addFileToTar(tw *tar.Writer, fullPath, relPath string, info os.FileInfo) error {
	hdr := &tar.Header{
		Name:    relPath,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}
	if info.IsDir() {
		hdr.Typeflag = tar.TypeDir
		hdr.Name += "/"
		return tw.WriteHeader(hdr)
	}

	hdr.Typeflag = tar.TypeReg
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

// handleContainerTop handles GET /containers/{id}/top.
func (s *Server) handleContainerTop(w http.ResponseWriter, r *http.Request, id string) {
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if state.Status != common.StateRunning {
		s.writeError(w, http.StatusConflict, "container is not running")
		return
	}

	// Read process list from /proc/{pid}.
	procs := listProcesses(state.Pid)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"Titles": []string{"PID", "USER", "TIME", "COMMAND"},
		"Processes": procs,
	})
}

func listProcesses(pid int) [][]string {
	var procs [][]string
	procDir := fmt.Sprintf("/proc/%d/task", pid)
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return procs
	}
	for _, entry := range entries {
		taskPid := entry.Name()
		commPath := filepath.Join(procDir, taskPid, "comm")
		comm := ""
		if data, err := os.ReadFile(commPath); err == nil {
			comm = strings.TrimSpace(string(data))
		}
		procs = append(procs, []string{taskPid, "root", "0:00", comm})
	}
	return procs
}
