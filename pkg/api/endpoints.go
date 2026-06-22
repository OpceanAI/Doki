// Package api provides the Docker-compatible HTTP API server.
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
	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
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
	endsWithSlash := strings.HasSuffix(path, "/")

	switch r.Method {
	case "GET":
		s.handleArchiveGet(w, r, rootfs, containerPath)
	case "PUT":
		s.handleArchivePut(w, r, rootfs, containerPath, endsWithSlash)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleArchiveGet(w http.ResponseWriter, _ *http.Request, rootfs, containerPath string) {
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
	defer func() { _ = tw.Close() }()

	if info.IsDir() {
		_ = filepath.Walk(containerPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			relPath, _ := filepath.Rel(rootfs, path)
			return addFileToTar(tw, path, relPath, fi)
		})
	} else {
		_ = addFileToTar(tw, containerPath, filepath.Base(containerPath), info)
	}
}

func (s *Server) handleArchivePut(w http.ResponseWriter, r *http.Request, _ string, containerPath string, pathEndsWithSlash bool) {
	destInfo, destStatErr := os.Stat(containerPath)
	destIsDir := destStatErr == nil && destInfo.IsDir()

	if !destIsDir && !pathEndsWithSlash {
		if err := os.MkdirAll(filepath.Dir(containerPath), 0755); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
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

		var target string
		if destIsDir || pathEndsWithSlash {
			target = filepath.Join(containerPath, hdr.Name)
			if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(containerPath)+string(os.PathSeparator)) {
				s.writeError(w, http.StatusBadRequest, "invalid path in archive")
				return
			}
		} else {
			target = containerPath
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, os.FileMode(hdr.Mode))
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				continue
			}
			const maxFileSize = 1 << 30 // 1 GB
			n, err := io.Copy(f, io.LimitReader(tr, maxFileSize+1))
			if err != nil {
				_ = f.Close()
				s.writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			_ = f.Close()
			if n > maxFileSize {
				_ = os.Remove(target)
				s.writeError(w, http.StatusRequestEntityTooLarge, "file too large")
				return
			}
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]string{})
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

	// Check if name already exists
	states, err := s.runtime.List()
	if err == nil {
		for _, state := range states {
			if state.Config != nil && state.Config.Annotations != nil {
				if existingName := state.Config.Annotations["doki.name"]; existingName == newName && state.ID != id {
					s.writeError(w, http.StatusConflict, "name already in use")
					return
				}
			}
		}
	}

	// Update container name in annotations.
	state, err := s.runtime.State(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if state.Config == nil {
		state.Config = &dokiruntime.Config{}
	}
	if state.Config.Annotations == nil {
		state.Config.Annotations = make(map[string]string)
	}
	state.Config.Annotations["doki.name"] = newName

	// Persist changes
	if err := s.runtime.SaveState(state); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to save state: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleContainerExecResize handles POST /exec/{id}/resize.
func (s *Server) handleExecResize(w http.ResponseWriter, r *http.Request, _ string) {
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
	defer func() { _ = f.Close() }()
	_, err = io.Copy(tw, f)
	return err
}

// handleContainerTop handles GET /containers/{id}/top.
func (s *Server) handleContainerTop(w http.ResponseWriter, _ *http.Request, id string) {
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
		"Titles":    []string{"PID", "USER", "TIME", "COMMAND"},
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
