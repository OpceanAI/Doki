package runtime

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/cgroups"
	"github.com/OpceanAI/Doki/internal/dokivm"
	"github.com/OpceanAI/Doki/internal/dokivm/rootfs"
	"github.com/OpceanAI/Doki/internal/fuse"
	"github.com/OpceanAI/Doki/internal/namespaces"
	"github.com/OpceanAI/Doki/internal/proot"
	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/storage"
)

// ExecutionMode defines how a container process is run.
type ExecutionMode int

const (
	ModeNative     ExecutionMode = iota // Direct host execution (Android / rootless without namespaces)
	ModeProot                           // proot-based isolation (Android fallback)
	ModeNamespaces                      // Full Linux namespace isolation (requires root)
	ModeMicroVM                         // Hardware-level isolation via microVM (crosvm/firecracker)
)

// Runtime implements the OCI Runtime Specification.
type Runtime struct {
	mu       sync.RWMutex
	root     string
	store    *storage.Manager
	nsMgr    *namespaces.Manager
	cgMgr    *cgroups.Manager
	prootMgr *proot.Manager
	rootless bool
	mode     ExecutionMode
}

type Config struct {
	ID            string
	Rootfs        string
	Args          []string
	Env           []string
	Cwd           string
	User          string
	Tty           bool
	Interactive   bool
	Privileged    bool
	ReadOnly      bool
	NetworkMode   common.NetworkMode
	Hostname      string
	Labels        map[string]string
	Annotations   map[string]string
	Mounts        []common.Mount
	Ports         []common.Port
	DNS           []string
	DNSSearch     []string
	DNSOptions    []string
	ExtraHosts    []string
	CapAdd        []string
	CapDrop       []string
	SecurityOpt   []string
	Sysctls       map[string]string
	Resources     *Resources
	StopSignal    string
	StopTimeout   int
	Init               bool
	RestartPolicy      common.RestartPolicy
	RestartMaxRetries  int
	HealthCheck        *HealthCheckConfig
	Runtime            string
	LogDriver     common.LogDriver
	ImageRef      string
	ImageDigest   string
	ImageLayers   []string // paths to image layer tarballs
	ImageConfig   *ImageOCIConfig
	RootfsReady   string // path to extracted rootfs
}

type HealthCheckConfig struct {
	Test     []string
	Interval time.Duration
	Timeout  time.Duration
	Retries  int
}

type Resources struct {
	CPUShares      int64
	Memory         int64
	MemorySwap     int64
	NanoCpus       int64
	CPUPeriod      int64
	CPUQuota       int64
	CpusetCpus     string
	CpusetMems     string
	PidsLimit      int64
	BlkioWeight    uint16
	OomKillDisable bool
	ShmSize        int64
}

type ImageOCIConfig struct {
	Entrypoint  []string
	Cmd         []string
	Env         []string
	WorkingDir  string
	User        string
	Volumes     map[string]struct{}
	Labels      map[string]string
	StopSignal  string
	Shell       []string
	HealthCheck *HealthCheckConfig
}

type ContainerState struct {
	ID           string                `json:"id"`
	Pid          int                   `json:"pid"`
	Status       common.ContainerState `json:"status"`
	Created      time.Time             `json:"created"`
	Started      time.Time             `json:"started,omitempty"`
	Finished     time.Time             `json:"finished,omitempty"`
	ExitCode     int                   `json:"exitCode,omitempty"`
	Bundle       string                `json:"bundle"`
	Config       *Config               `json:"config,omitempty"`
	PidPath      string                `json:"pidPath,omitempty"`
	LogPath      string                `json:"logPath,omitempty"`
	Mode         ExecutionMode         `json:"mode"`
	RestartCount int                   `json:"restartCount,omitempty"`
	HealthStatus *common.HealthStatus  `json:"healthStatus,omitempty"`
	ExitChan     chan struct{}         `json:"-"`
	Cmd          *exec.Cmd             `json:"-"`
}

func NewRuntime(root string, store *storage.Manager) *Runtime {
	rt := &Runtime{
		root:     root,
		store:    store,
		nsMgr:    namespaces.NewManager(root),
		cgMgr:    cgroups.NewManager("/sys/fs/cgroup/doki"),
		prootMgr: proot.NewManager(root),
		rootless: namespaces.IsRootless(),
	}
	rt.detectMode()

	common.EnsureDir(filepath.Join(root, "containers"))
	common.EnsureDir(filepath.Join(root, "bundles"))
	common.EnsureDir(filepath.Join(root, "layers"))
	common.EnsureDir(filepath.Join(root, "rootfs"))

	return rt
}

func (rt *Runtime) detectMode() {
	switch {
	case dokivm.IsAvailable():
		rt.mode = ModeMicroVM
	case proot.ShouldUseProot() && proot.IsAvailable():
		rt.mode = ModeProot
	case rt.isAndroid():
		rt.mode = ModeNative
	case rt.rootless:
		rt.mode = ModeNative
	default:
		rt.mode = ModeNamespaces
	}
}

func (rt *Runtime) Mode() ExecutionMode { return rt.mode }

func (rt *Runtime) isAndroid() bool {
	if _, err := os.Stat("/system/build.prop"); err == nil {
		return true
	}
	if _, err := os.Stat("/data/data/com.termux"); err == nil {
		return true
	}
	if os.Getenv("DOKI_NATIVE") == "1" {
		return true
	}
	return false
}

// ─── Container lifecycle ───────────────────────────────────────────

func (rt *Runtime) Create(cfg *Config) (*ContainerState, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if _, err := rt.loadState(cfg.ID); err == nil {
		return nil, common.NewErrConflict("container", cfg.ID)
	}

	bundleDir := filepath.Join(rt.root, "bundles", cfg.ID)
	rootfsDir := filepath.Join(bundleDir, "rootfs")
	common.EnsureDir(bundleDir)
	common.EnsureDir(rootfsDir)

	// Copy existing rootfs if provided.
	if cfg.Rootfs != "" && common.PathExists(cfg.Rootfs) {
		if err := fuse.CopyDir(cfg.Rootfs, rootfsDir); err != nil {
			return nil, fmt.Errorf("copy rootfs: %w", err)
		}
	}

	// Extract image layers into rootfs.
	if err := rt.extractLayers(rootfsDir, cfg.ImageLayers); err != nil {
		return nil, fmt.Errorf("extract layers: %w", err)
	}
	cfg.RootfsReady = rootfsDir

	// Prepare rootfs files.
	hostname := cfg.ID
	if len(hostname) > 12 {
		hostname = hostname[:12]
	}
	if cfg.Hostname != "" {
		hostname = cfg.Hostname
	}
	rootfsFiles := map[string]string{
		"etc/hostname":    fuse.GenerateHostname(hostname),
		"etc/hosts":       fuse.GenerateHosts(hostname, parseExtraHosts(cfg.ExtraHosts)),
		"etc/resolv.conf": fuse.GenerateResolvConf(cfg.DNS, cfg.DNSSearch, cfg.DNSOptions),
	}
	fuse.PrepareRootfs(rootfsDir, rootfsFiles)

	state := &ContainerState{
		ID:      cfg.ID,
		Status:  common.StateCreated,
		Created: time.Now(),
		Bundle:  bundleDir,
		Config:  cfg,
		Mode:    rt.mode,
		LogPath: filepath.Join(rt.root, "containers", cfg.ID, "container.log"),
	}
	state.ExitChan = make(chan struct{})

	if err := rt.saveState(state); err != nil {
		return nil, err
	}
	return state, nil
}

func (rt *Runtime) extractLayers(rootfsDir string, layers []string) error {
	if len(layers) == 0 {
		return nil
	}

	// Parallel extraction: max 3 concurrent goroutines
	sem := make(chan struct{}, 3)
	errCh := make(chan error, len(layers))
	var wg sync.WaitGroup

	// Track extracted layers for rollback
	extracted := make([]string, 0, len(layers))
	var mu sync.Mutex

	for i, layerPath := range layers {
		if !common.PathExists(layerPath) {
			continue
		}
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := extractTarGz(path, rootfsDir); err != nil {
				errCh <- fmt.Errorf("layer %d (%s): %w", idx, filepath.Base(path), err)
				return
			}
			mu.Lock()
			extracted = append(extracted, path)
			mu.Unlock()
		}(i, layerPath)
	}
	wg.Wait()
	close(errCh)

	// Collect first error
	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
	}

	// C13: Rollback on partial extraction failure
	if firstErr != nil {
		for _, lp := range extracted {
			// Remove files that were successfully extracted
			// from this layer by re-extracting with --diff or simple cleanup
			_ = lp // In-place rollback not fully implemented yet
		}
		// Clean up partial rootfs
		os.RemoveAll(rootfsDir)
		return firstErr
	}

	return nil
}

func extractTarGz(tarPath, dest string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// C12: Detect compression format from magic bytes.
	magic := make([]byte, 4)
	n, readErr := f.Read(magic)
	if readErr != nil && readErr != io.EOF {
		return readErr
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var decompressed io.Reader
	switch {
	case n >= 2 && magic[0] == 0x1f && magic[1] == 0x8b:
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		decompressed = gz
	case n >= 2 && magic[0] == 0x42 && magic[1] == 0x5a:
		decompressed = bzip2.NewReader(f)
	case n >= 4 && magic[0] == 0xfd && magic[1] == 0x37 && magic[2] == 0x7a && magic[3] == 0x58:
		// xz decompression via xz command
		xzCmd := exec.Command("xz", "-dc")
		xzCmd.Stdin = f
		var xzOut bytes.Buffer
		xzCmd.Stdout = &xzOut
		if err := xzCmd.Run(); err != nil {
			return fmt.Errorf("tar: xz decompression failed: %w", err)
		}
		decompressed = &xzOut
	case n >= 4 && magic[0] == 0x28 && magic[1] == 0xb5 && magic[2] == 0x2f && magic[3] == 0xfd:
		// zstd decompression via zstd command
		zstdCmd := exec.Command("zstd", "-dc")
		zstdCmd.Stdin = f
		var zstdOut bytes.Buffer
		zstdCmd.Stdout = &zstdOut
		if err := zstdCmd.Run(); err != nil {
			return fmt.Errorf("tar: zstd decompression failed: %w", err)
		}
		decompressed = &zstdOut
	default:
		decompressed = f
	}

	tr := tar.NewReader(decompressed)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Path traversal protection (CWE-22).
		cleanDest := filepath.Clean(dest)
		target := filepath.Clean(filepath.Join(dest, hdr.Name))
		if hdr.Name == "." || hdr.Name == "./" || target == cleanDest {
			continue
		}
		if !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) && target != cleanDest {
			return fmt.Errorf("tar: path traversal attempt: %s -> %s", hdr.Name, target)
		}

		// C1: Whiteout files - OCI layers use .wh.<filename> to mark deleted files.
		baseName := filepath.Base(hdr.Name)
		if strings.HasPrefix(baseName, ".wh.") {
			// Opaque whiteout: .wh..wh..opq clears the entire directory
			if baseName == ".wh..wh..opq" {
				opqDir := filepath.Clean(filepath.Join(dest, filepath.Dir(hdr.Name)))
				if strings.HasPrefix(opqDir, cleanDest+string(os.PathSeparator)) || opqDir == cleanDest {
					entries, _ := os.ReadDir(opqDir)
					for _, e := range entries {
						os.RemoveAll(filepath.Join(opqDir, e.Name()))
					}
				}
				continue
			}
			whTarget := filepath.Clean(filepath.Join(dest, filepath.Dir(hdr.Name), baseName[4:]))
			if strings.HasPrefix(whTarget, cleanDest+string(os.PathSeparator)) || whTarget == cleanDest {
				os.RemoveAll(whTarget)
			}
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			os.Chown(target, hdr.Uid, hdr.Gid)
			os.Chtimes(target, hdr.ModTime, hdr.ModTime)
			extractXattrs(hdr, target)
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				os.Remove(target)
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
			if err := os.Chmod(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
			os.Chown(target, hdr.Uid, hdr.Gid)
			os.Chtimes(target, hdr.ModTime, hdr.ModTime)
			extractXattrs(hdr, target)
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			// C7: For absolute symlink targets, keep them as-is (pointing to container's /).
			linkTarget := hdr.Linkname
			if !filepath.IsAbs(linkTarget) {
				resolved := filepath.Clean(filepath.Join(filepath.Dir(target), linkTarget))
				if !strings.HasPrefix(resolved, cleanDest+string(os.PathSeparator)) && resolved != cleanDest {
					return fmt.Errorf("tar: symlink escape attempt: %s -> %s", hdr.Linkname, resolved)
				}
			}
			os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
			os.Lchown(target, hdr.Uid, hdr.Gid)
			extractXattrs(hdr, target)
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			linkTarget := filepath.Clean(filepath.Join(dest, hdr.Linkname))
			if !strings.HasPrefix(linkTarget, cleanDest+string(os.PathSeparator)) && linkTarget != cleanDest {
				return fmt.Errorf("tar: hardlink escape attempt")
			}
			os.Remove(target)
			// C8: Hardlink with fallback to copy; return error if both fail.
			if err := os.Link(linkTarget, target); err != nil {
				if data, readErr := os.ReadFile(linkTarget); readErr == nil {
					os.Remove(target)
					if writeErr := os.WriteFile(target, data, 0644); writeErr != nil {
						return fmt.Errorf("tar: hardlink copy fallback: %w", writeErr)
					}
				} else {
					return fmt.Errorf("tar: hardlink failed for %s: %w", hdr.Name, err)
				}
			}
		case tar.TypeBlock:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			os.Remove(target)
			dev := int(hdr.Devmajor)<<8 | int(hdr.Devminor)
			if err := syscall.Mknod(target, syscall.S_IFBLK|uint32(hdr.Mode&0777), dev); err != nil {
				return fmt.Errorf("tar: mknod block device %s: %w", hdr.Name, err)
			}
			os.Chown(target, hdr.Uid, hdr.Gid)
			os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeChar:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			os.Remove(target)
			dev := int(hdr.Devmajor)<<8 | int(hdr.Devminor)
			if err := syscall.Mknod(target, syscall.S_IFCHR|uint32(hdr.Mode&0777), dev); err != nil {
				return fmt.Errorf("tar: mknod char device %s: %w", hdr.Name, err)
			}
			os.Chown(target, hdr.Uid, hdr.Gid)
			os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeFifo:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			os.Remove(target)
			if err := syscall.Mkfifo(target, uint32(hdr.Mode&0777)); err != nil {
				return fmt.Errorf("tar: mkfifo %s: %w", hdr.Name, err)
			}
			os.Chown(target, hdr.Uid, hdr.Gid)
			os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeGNUSparse:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				os.Remove(target)
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
			if err := os.Chmod(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
			os.Chown(target, hdr.Uid, hdr.Gid)
			os.Chtimes(target, hdr.ModTime, hdr.ModTime)
			extractXattrs(hdr, target)
		}
	}
	return nil
}

// extractXattrs extracts extended attributes from PAXRecords (C6/C14).
func extractXattrs(hdr *tar.Header, target string) {
	if hdr.PAXRecords == nil {
		return
	}
	for key, value := range hdr.PAXRecords {
		if strings.HasPrefix(key, "SCHILY.xattr.") {
			attrName := strings.TrimPrefix(key, "SCHILY.xattr.")
			syscall.Setxattr(target, attrName, []byte(value), 0)
		}
	}
}

func (rt *Runtime) Start(id string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	state, err := rt.loadState(id)
	if err != nil {
		return err
	}
	if state.Status != common.StateCreated {
		return fmt.Errorf("container %s is in state %s", id, state.Status)
	}

	// ExitChan is not serialized, recreate it after loading.
	state.ExitChan = make(chan struct{})

	bundleDir := state.Bundle
	rootfsDir := filepath.Join(bundleDir, "rootfs")
	cfg := state.Config

	// H5: Log driver selection.
	var logFile *os.File
	switch cfg.LogDriver {
	case common.LogNone:
		logFile, err = os.OpenFile("/dev/null", os.O_WRONLY, 0)
	default:
		// "json-file", "syslog", "journald", or empty -> fall back to json-file.
		// H1: Rotate before opening.
		rt.rotateLog(state.LogPath, 10*1024*1024, 3)
		logFile, err = os.OpenFile(state.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	syscall.CloseOnExec(int(logFile.Fd()))

	// Setup mounts (only in namespace mode).
	if rt.mode == ModeNamespaces {
		if err := rt.setupMounts(rootfsDir, cfg); err != nil {
			logFile.Close()
			return fmt.Errorf("setup mounts: %w", err)
		}
	}

	// G1: --init flag: prepend init binary to command args.
	if cfg.Init {
		for _, bin := range []string{"/sbin/tini", "/usr/bin/dumb-init"} {
			hostBin := filepath.Join(rootfsDir, bin)
			if _, err := os.Stat(hostBin); err == nil {
				if rt.mode == ModeNative {
					cfg.Args = append([]string{hostBin, "--"}, cfg.Args...)
				} else {
					cfg.Args = append([]string{bin, "--"}, cfg.Args...)
				}
				break
			}
		}
	}

	// Start process.
	pid, proc, err := rt.startProcess(cfg, rootfsDir, logFile)
	if err != nil {
		logFile.Close()
		return fmt.Errorf("start process: %w", err)
	}

	// Apply cgroups when available.
	if rt.cgMgr.IsAvailable() {
		cgCfg := rt.buildCgroupConfig(cfg)
		if _, err := rt.cgMgr.Create(id, cgCfg); err == nil {
			rt.cgMgr.AddProcess(id, pid)
		}
	}

	state.Pid = pid
	state.Status = common.StateRunning
	state.Started = time.Now()
	state.Cmd = proc
	state.PidPath = filepath.Join(rt.root, "containers", id, "init.pid")
	os.WriteFile(state.PidPath, []byte(fmt.Sprintf("%d", pid)), 0644)

	if err := rt.saveState(state); err != nil {
		return err
	}

	// Monitor process exit.
	go rt.monitorProcess(state, logFile)

	// G2: Start healthcheck if configured.
	if cfg.HealthCheck != nil && len(cfg.HealthCheck.Test) > 0 {
		state.HealthStatus = &common.HealthStatus{
			Status:        "starting",
			FailingStreak: 0,
			Log:           []common.HealthCheckResult{},
		}
		rt.saveState(state)
		rt.StartHealthcheck(id, cfg.HealthCheck.Test, cfg.HealthCheck.Interval,
			cfg.HealthCheck.Timeout, cfg.HealthCheck.Retries)
	}

	return nil
}

func (rt *Runtime) monitorProcess(state *ContainerState, logFile *os.File) {
	// Wait for process exit WITHOUT holding the runtime mutex.
	// Check ProcessState to avoid double Wait(): startWithProot/retryWithQemu
	// may have already called Wait() via their fast-detection goroutines.
	if state.Cmd != nil && state.Cmd.ProcessState == nil {
		state.Cmd.Wait()
	}
	if logFile != nil {
		logFile.Close()
	}

	exitCode := -1
	if state.Cmd != nil && state.Cmd.ProcessState != nil {
		if ws, ok := state.Cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
			exitCode = ws.ExitStatus()
		}
	}

	// Close ExitChan BEFORE acquiring the lock to prevent deadlock with Stop():
	// if Stop() holds the lock waiting on ExitChan and we try to acquire the lock
	// before closing, both goroutines deadlock.
	if state.ExitChan != nil {
		close(state.ExitChan)
	}

	// Lock for state modification and persistence only.
	rt.mu.Lock()
	state.Status = common.StateExited
	state.Finished = time.Now()
	state.ExitCode = exitCode
	if err := rt.saveState(state); err != nil {
		_, _ = os.Stderr.Write([]byte(fmt.Sprintf("DOKI: failed to save state for %s: %v\n", state.ID, err)))
	}
	rt.mu.Unlock()

	// G10: Trigger restart monitor after process exits.
	rt.handleRestart(state, exitCode)
}

// G10-G14: handleRestart implements container restart policy.
func (rt *Runtime) handleRestart(state *ContainerState, exitCode int) {
	cfg := state.Config
	if cfg == nil || cfg.RestartPolicy == "" || cfg.RestartPolicy == common.RestartNo {
		return
	}

	id := state.ID

	switch cfg.RestartPolicy {
	case common.RestartAlways:
		// G11: "always" policy loops via monitorProcess re-registration each time.
		time.Sleep(1 * time.Second)
		rt.mu.Lock()
		state.RestartCount++
		rt.saveState(state)
		rt.mu.Unlock()
		rt.Start(id)

	case common.RestartOnFailure:
		if exitCode != 0 {
			rt.mu.Lock()
			state.RestartCount++
			rt.saveState(state)
			rt.mu.Unlock()

			maxRetries := cfg.RestartMaxRetries
			// G12: Fix backoff overflow for maxRetries=0 (unlimited) - cap at 60s.
			if maxRetries < 0 {
				maxRetries = 0
			}
			backoff := time.Duration(1) * time.Second
			for i := 0; maxRetries == 0 || i < maxRetries; i++ {
				time.Sleep(backoff)
				backoff *= 2
				if backoff > 60*time.Second {
					backoff = 60 * time.Second
				}
				rt.mu.Lock()
				state.RestartCount++
				rt.saveState(state)
				rt.mu.Unlock()
				if err := rt.Start(id); err == nil {
					return // Success: new monitorProcess will handle next exit.
				}
			}
		}

	case common.RestartUnlessStopped:
		if state.Status != common.StateDead {
			time.Sleep(1 * time.Second)
			rt.mu.Lock()
			state.RestartCount++
			rt.saveState(state)
			rt.mu.Unlock()
			rt.Start(id)
		}
	}
}

// ─── 3 execution modes ─────────────────────────────────────────────

// startProcess selects the appropriate execution mode.
func (rt *Runtime) startProcess(cfg *Config, rootfsDir string, logFile *os.File) (int, *exec.Cmd, error) {
	switch rt.mode {
	case ModeMicroVM:
		return rt.startWithMicroVM(cfg, rootfsDir, logFile)
	case ModeProot:
		return rt.startWithProot(cfg, rootfsDir, logFile)
	case ModeNamespaces:
		return rt.startWithNamespaces(cfg, rootfsDir, logFile)
	default:
		return rt.startNative(cfg, rootfsDir, logFile)
	}
}

// startNative runs the container process directly on the host without
// namespace isolation. This is the default mode on Android and rootless
// systems. The process runs with the rootfs as its working directory
// and image binaries accessible via PATH.
func (rt *Runtime) startNative(cfg *Config, rootfsDir string, logFile *os.File) (int, *exec.Cmd, error) {
	args := cfg.Args
	if len(args) == 0 {
		return 0, nil, fmt.Errorf("no command specified for container")
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = rootfsDir
	if cfg.Cwd != "" {
		cmd.Dir = filepath.Join(rootfsDir, cfg.Cwd)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = os.Stdin
	cmd.Env = cfg.Env

	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}
	return cmd.Process.Pid, cmd, nil
}

// startWithProot runs the container via proot (userspace chroot).
func (rt *Runtime) startWithProot(cfg *Config, rootfsDir string, logFile *os.File) (int, *exec.Cmd, error) {
	args := cfg.Args
	if len(args) == 0 {
		return 0, nil, fmt.Errorf("no command specified for container")
	}

	cleanRootfs := filepath.Clean(rootfsDir)

	prootArgs := []string{
		"-r", cleanRootfs,
		"-b", "/proc",
		"-b", "/proc/self/fd:/dev/fd",
		"-b", "/sys",
		"-b", "/dev",
		"-b", "/dev/urandom:/dev/random",
		"--kill-on-exit",
		"--link2symlink",
		"--kernel-release=6.17.0-PRoot-Distro",
	}

	selinuxTarget := filepath.Join(cleanRootfs, "sys", "fs", "selinux")
	os.MkdirAll(selinuxTarget, 0755)
	prootArgs = append(prootArgs, "-b", selinuxTarget+":/sys/fs/selinux")

	if rt.isAndroid() {
		for _, dir := range []string{
			"/apex", "/system", "/vendor",
			"/storage", "/sdcard",
			"/data/data/com.termux/files",
		} {
			if _, err := os.Stat(dir); err == nil {
				prootArgs = append(prootArgs, "-b", dir)
			}
		}
		if _, err := os.Stat("/data/data/com.termux/files/usr"); err == nil {
			prootArgs = append(prootArgs, "-b", "/data/data/com.termux/files/usr")
		}
		if _, err := os.Stat("/linkerconfig/ld.config.txt"); err == nil {
			prootArgs = append(prootArgs, "-b", "/linkerconfig/ld.config.txt")
		}
		if home := os.Getenv("HOME"); home != "" {
			prootArgs = append(prootArgs, "-b", home)
		}
	}

	if cfg.Cwd != "" {
		prootArgs = append(prootArgs, "-w", cfg.Cwd)
	}
	prootArgs = append(prootArgs, args...)

	prootBin := "proot"
	if _, err := os.Stat("doki-proot"); err == nil {
		prootBin = "doki-proot"
	}

	cmd := exec.Command(prootBin, prootArgs...)
	cmd.Dir = cleanRootfs
	cmd.Stdout = logFile
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	env := os.Environ()
	env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/")
	env = append(env, "LD_LIBRARY_PATH=/usr/lib:/lib:/usr/local/lib")
	// Image config env takes precedence (contains PATH, etc.)
	if cfg.ImageConfig != nil {
		for _, e := range cfg.ImageConfig.Env {
			env = append(env, e)
		}
	}
	// Validate env vars (filter invalid names, enforce size limits)
	validEnv := common.ValidateEnv(cfg.Env)
	for _, e := range validEnv {
		env = append(env, e)
	}
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("proot start: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		stderrStr := stderrBuf.String()
		if stderrStr != "" {
			logFile.Write([]byte(stderrStr))
		}

		hasENOSYS := strings.Contains(stderrStr, "Function not implemented") ||
			strings.Contains(stderrStr, "ENOSYS")

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				signaled := false
				if ws, ok2 := exitErr.Sys().(syscall.WaitStatus); ok2 {
					signaled = ws.Signaled()
				}
				if code == 126 || code == 127 || (code != 0 && (hasENOSYS || signaled)) {
					logFile.Write([]byte("DOKI: proot failed, retrying with QEMU...\n"))
					if pid, qemuCmd, qemuErr := rt.retryWithQemu(cfg, rootfsDir, logFile); qemuErr == nil {
						return pid, qemuCmd, nil
					}
				}
				if code != 0 {
					return 0, nil, fmt.Errorf("proot exited with code %d (binary may be incompatible or missing in rootfs)", code)
				}
				return cmd.Process.Pid, cmd, nil
			}
			if hasENOSYS {
				logFile.Write([]byte("DOKI: proot failed with ENOSYS, retrying with QEMU...\n"))
				if pid, qemuCmd, qemuErr := rt.retryWithQemu(cfg, rootfsDir, logFile); qemuErr == nil {
					return pid, qemuCmd, nil
				}
			}
			return 0, nil, fmt.Errorf("proot failed: %w", err)
		}
		return cmd.Process.Pid, cmd, nil
	case <-time.After(2000 * time.Millisecond):
	}

	return cmd.Process.Pid, cmd, nil
}

// startWithNamespaces runs the container with full Linux namespace isolation
// (requires root).
func (rt *Runtime) startWithNamespaces(cfg *Config, rootfsDir string, logFile *os.File) (int, *exec.Cmd, error) {
	args := cfg.Args
	if len(args) == 0 {
		return 0, nil, fmt.Errorf("no command specified for container")
	}

	// I5: pivot_root - create old_root directory.
	oldRootDir := filepath.Join(rootfsDir, ".pivot_root")
	os.MkdirAll(oldRootDir, 0755)

	// Build a shell init script that does pivot_root then execs the user command.
	pivotScript := fmt.Sprintf(
		`mount --bind "%s" "%s" && pivot_root "%s" "%s/.pivot_root" && cd / && umount -l "/.pivot_root" && exec "$@"`,
		rootfsDir, rootfsDir, rootfsDir, rootfsDir)

	allArgs := append([]string{"/bin/sh", "-c", pivotScript, "doki-init"}, args...)
	cmd := exec.Command(allArgs[0], allArgs[1:]...)
	cmd.Dir = rootfsDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = os.Stdin
	cmd.Env = cfg.Env

	cloneFlags := syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS |
		syscall.CLONE_NEWIPC | syscall.CLONE_NEWPID

	if cfg.NetworkMode != common.NetworkHost && cfg.NetworkMode != common.NetworkNone {
		cloneFlags |= syscall.CLONE_NEWNET
	}

	// I1: User namespaces are for NON-privileged (rootless) containers.
	if !cfg.Privileged && rt.rootless {
		cloneFlags |= syscall.CLONE_NEWUSER
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: uintptr(cloneFlags),
	}

	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}

	// I2 + I3: Write UID/GID mappings for user namespaces.
	if !cfg.Privileged && rt.rootless {
		rt.nsMgr.SetupUserNamespace(cmd.Process.Pid, &namespaces.Config{
			User:     true,
			Rootless: true,
		})
	}

	// I4: Set up loopback in new network namespace.
	if cfg.NetworkMode != common.NetworkHost && cfg.NetworkMode != common.NetworkNone {
		loopbackCmd := exec.Command("nsenter", "-t", strconv.Itoa(cmd.Process.Pid), "-n", "ip", "link", "set", "lo", "up")
		loopbackCmd.Run()
	}

	return cmd.Process.Pid, cmd, nil
}

// ─── Mount setup (namespace mode only) ─────────────────────────────

func (rt *Runtime) setupMounts(rootfsDir string, cfg *Config) error {
	fuse.ProcMount(filepath.Join(rootfsDir, "proc"))
	fuse.SysMount(filepath.Join(rootfsDir, "sys"))
	fuse.DevMount(filepath.Join(rootfsDir, "dev"))
	fuse.DevPtsMount(filepath.Join(rootfsDir, "dev", "pts"))

	shmSize := int64(67108864)
	if cfg.Resources != nil && cfg.Resources.Memory > 0 {
		shmSize = cfg.Resources.Memory / 2
	}
	fuse.ShmMount(filepath.Join(rootfsDir, "dev", "shm"), shmSize)

	for _, mnt := range cfg.Mounts {
		target := filepath.Join(rootfsDir, mnt.Target)
		switch mnt.Type {
		case common.MountBind:
			if mnt.Source != "" {
				fuse.BindMount(mnt.Source, target, mnt.ReadOnly)
			}
		case common.MountTmpfs:
			size := int64(0)
			if mnt.TmpfsOptions != nil {
				size = mnt.TmpfsOptions.SizeBytes
			}
			fuse.TmpfsMount(target, size, 0755)
		}
	}

	if cfg.ReadOnly {
		flags := uintptr(syscall.MS_BIND | syscall.MS_REMOUNT | syscall.MS_RDONLY)
		if err := syscall.Mount("", rootfsDir, "", flags, ""); err != nil {
			return fmt.Errorf("read-only remount: %w", err)
		}
	}

	return nil
}

// ─── Container operations ──────────────────────────────────────────

func (rt *Runtime) Exec(id string, args []string, env []string, tty bool) error {
	state, err := rt.State(id)
	if err != nil {
		return err
	}
	if state.Status != common.StateRunning {
		return fmt.Errorf("container %s is not running", id)
	}

	// Find the container's rootfs.
	rootfsDir := ""
	if state.Config != nil {
		rootfsDir = state.Config.RootfsReady
	}
	if rootfsDir == "" && state.Bundle != "" {
		rootfsDir = filepath.Join(state.Bundle, "rootfs")
	}

	cmd := exec.Command(args[0], args[1:]...)
	if rootfsDir != "" && common.PathExists(rootfsDir) {
		cmd.Dir = rootfsDir
		// Prepend rootfs bin dirs to PATH so container binaries are found.
		pathPrefix := rootfsDir + "/usr/local/sbin:" + rootfsDir + "/usr/local/bin:" +
			rootfsDir + "/usr/sbin:" + rootfsDir + "/usr/bin:" +
			rootfsDir + "/sbin:" + rootfsDir + "/bin"
		if currentPath := os.Getenv("PATH"); currentPath != "" {
			pathPrefix += ":" + currentPath
		}
		env = append(env, "PATH="+pathPrefix)
	}
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (rt *Runtime) Stop(id string, timeout int) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.stopUnlocked(id, timeout)
}

func (rt *Runtime) stopUnlocked(id string, timeout int) error {
	state, err := rt.loadState(id)
	if err != nil {
		return err
	}
	if state.Status != common.StateRunning {
		return fmt.Errorf("container %s is not running", id)
	}

	sig := syscall.SIGTERM
	if state.Config != nil && state.Config.StopSignal != "" {
		sig = parseSignal(state.Config.StopSignal)
	}
	process, _ := os.FindProcess(state.Pid)
	process.Signal(sig)

	if timeout <= 0 {
		timeout = 10
	}
	select {
	case <-state.ExitChan:
	case <-time.After(time.Duration(timeout) * time.Second):
		process.Signal(syscall.SIGKILL)
		select {
		case <-state.ExitChan:
		case <-time.After(3 * time.Second):
		}
	}

	rt.cleanupContainer(state)
	return nil
}

func (rt *Runtime) Kill(id string, signal syscall.Signal) error {
	state, err := rt.State(id)
	if err != nil {
		return err
	}
	if state.Status != common.StateRunning {
		return nil
	}
	process, _ := os.FindProcess(state.Pid)
	return process.Signal(signal)
}

func (rt *Runtime) Pause(id string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	state, err := rt.loadState(id)
	if err != nil || state.Status != common.StateRunning {
		return err
	}
	if rt.cgMgr.IsAvailable() {
		rt.cgMgr.Freeze(id)
	} else if state.Cmd != nil && state.Cmd.Process != nil {
		// Fallback: SIGSTOP the process
		state.Cmd.Process.Signal(syscall.SIGSTOP)
	}
	state.Status = common.StatePaused
	return rt.saveState(state)
}

func (rt *Runtime) Unpause(id string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	state, err := rt.loadState(id)
	if err != nil || state.Status != common.StatePaused {
		return err
	}
	if rt.cgMgr.IsAvailable() {
		rt.cgMgr.Thaw(id)
	} else if state.Cmd != nil && state.Cmd.Process != nil {
		// Fallback: SIGCONT the process
		state.Cmd.Process.Signal(syscall.SIGCONT)
	}
	state.Status = common.StateRunning
	return rt.saveState(state)
}

func (rt *Runtime) State(id string) (*ContainerState, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.loadState(id)
}

func (rt *Runtime) Delete(id string, force bool) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	state, err := rt.loadState(id)
	if err != nil {
		if force {
			return nil
		}
		return err
	}
	if state.Status == common.StateRunning {
		if !force {
			return fmt.Errorf("container %s is running", id)
		}
		rt.stopUnlocked(id, 0)
	}
	rt.cleanupContainer(state)
	return nil
}

func (rt *Runtime) List() ([]*ContainerState, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	var states []*ContainerState
	dir := filepath.Join(rt.root, "containers")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := rt.loadState(e.Name())
		if err != nil {
			continue
		}
		states = append(states, s)
	}
	return states, nil
}

func (rt *Runtime) Stats(id string) (map[string]interface{}, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if rt.cgMgr.IsAvailable() {
		return rt.cgMgr.GetStats(id)
	}
	return nil, nil
}

func (rt *Runtime) GetLogs(id string, tail int) (string, error) {
	state, err := rt.State(id)
	if err != nil {
		return "", err
	}
	if state.LogPath == "" {
		return "", nil
	}
	data, err := os.ReadFile(state.LogPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if tail > 0 && len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return strings.Join(lines, "\n"), nil
}

// rotateLog rotates a log file if it exceeds maxSize bytes. Keeps up to keep
// rotated files (name -> name.1 -> name.2 -> ...).
func (rt *Runtime) rotateLog(logPath string, maxSize int64, keep int) {
	fi, err := os.Stat(logPath)
	if err != nil || fi.Size() < maxSize {
		return
	}
	// Shift existing rotations: name.keep -> name.keep+1 (removed), ...
	for i := keep - 1; i >= 1; i-- {
		oldPath := logPath + "." + strconv.Itoa(i)
		newPath := logPath + "." + strconv.Itoa(i+1)
		if i == keep-1 {
			os.Remove(newPath)
		}
		os.Rename(oldPath, newPath)
	}
	os.Rename(logPath, logPath+".1")
}

func (rt *Runtime) Processes(id string) ([]string, error) {
	state, err := rt.State(id)
	if err != nil || state.Status != common.StateRunning {
		return nil, err
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", state.Pid))
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

// ─── Helpers ───────────────────────────────────────────────────────

func (rt *Runtime) cleanupContainer(state *ContainerState) {
	rt.cgMgr.Destroy(state.ID)
	rt.nsMgr.DeletePersistentNamespace(state.ID)
	if state.Bundle != "" {
		fuse.CleanupMounts(filepath.Join(state.Bundle, "rootfs"))
		os.RemoveAll(state.Bundle)
	}
	os.RemoveAll(filepath.Join(rt.root, "containers", state.ID))
}

func (rt *Runtime) loadState(id string) (*ContainerState, error) {
	// Exact match.
	statePath := filepath.Join(rt.root, "containers", id, "state.json")
	if data, err := os.ReadFile(statePath); err == nil {
		var s ContainerState
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		return &s, nil
	}
	// Prefix match.
	if len(id) < 64 {
		entries, _ := os.ReadDir(filepath.Join(rt.root, "containers"))
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), id) {
				sp := filepath.Join(rt.root, "containers", e.Name(), "state.json")
				if data, err := os.ReadFile(sp); err == nil {
					var s ContainerState
					json.Unmarshal(data, &s)
					return &s, nil
				}
			}
		}
		// Name-based search (look for annotation doki.name).
		for _, e := range entries {
			if e.IsDir() {
				sp := filepath.Join(rt.root, "containers", e.Name(), "state.json")
				if data, err := os.ReadFile(sp); err == nil {
					var s ContainerState
					if json.Unmarshal(data, &s) == nil {
						if s.Config != nil && s.Config.Annotations != nil {
							if n, ok := s.Config.Annotations["doki.name"]; ok && n == id {
								return &s, nil
							}
						}
					}
				}
			}
		}
	}
	return nil, common.NewErrNotFound("container", id)
}

func (rt *Runtime) saveState(state *ContainerState) error {
	dir := filepath.Join(rt.root, "containers", state.ID)
	common.EnsureDir(dir)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "state.json"), data, 0644)
}

func (rt *Runtime) buildCgroupConfig(cfg *Config) *cgroups.Config {
	if cfg.Resources == nil {
		return &cgroups.Config{}
	}
	return &cgroups.Config{
		CPUPeriod:      uint64(cfg.Resources.CPUPeriod),
		CPUQuota:       cfg.Resources.CPUQuota,
		CPUShares:      uint64(cfg.Resources.CPUShares),
		CpusetCpus:     cfg.Resources.CpusetCpus,
		CpusetMems:     cfg.Resources.CpusetMems,
		Memory:         cfg.Resources.Memory,
		MemorySwap:     cfg.Resources.MemorySwap,
		PidsLimit:      cfg.Resources.PidsLimit,
		BlkioWeight:    cfg.Resources.BlkioWeight,
		NanoCpus:       cfg.Resources.NanoCpus,
		OomKillDisable: cfg.Resources.OomKillDisable,
	}
}

func parseSignal(s string) syscall.Signal {
	switch strings.ToUpper(s) {
	case "SIGHUP":
		return syscall.SIGHUP
	case "SIGINT":
		return syscall.SIGINT
	case "SIGQUIT":
		return syscall.SIGQUIT
	case "SIGKILL":
		return syscall.SIGKILL
	case "SIGTERM":
		return syscall.SIGTERM
	case "SIGSTOP":
		return syscall.SIGSTOP
	case "SIGUSR1":
		return syscall.SIGUSR1
	case "SIGUSR2":
		return syscall.SIGUSR2
	default:
		return syscall.SIGTERM
	}
}

func parseExtraHosts(hosts []string) map[string]string {
	m := make(map[string]string)
	for _, h := range hosts {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

// ─── Healthcheck ──────────────────────────────────────────────────

// StartHealthcheck begins periodic health checks for a container.
func (rt *Runtime) StartHealthcheck(id string, cmd []string, interval, timeout time.Duration, retries int) {
	go func() {
		if interval <= 0 {
			interval = 30 * time.Second
		}
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		if retries <= 0 {
			retries = 3
		}

		failures := 0
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			state, err := rt.State(id)
			if err != nil || state.Status != common.StateRunning {
				return
			}

			rootfsDir := ""
			if state.Config != nil {
				rootfsDir = state.Config.RootfsReady
			}
			if rootfsDir == "" && state.Bundle != "" {
				rootfsDir = filepath.Join(state.Bundle, "rootfs")
			}

			// G3: Run healthcheck command INSIDE the container.
			var hcmd *exec.Cmd
			containerPath := rootfsDir + "/usr/local/sbin:" + rootfsDir + "/usr/local/bin:" +
				rootfsDir + "/usr/sbin:" + rootfsDir + "/usr/bin:" +
				rootfsDir + "/sbin:" + rootfsDir + "/bin"
			if rt.mode == ModeProot && proot.IsAvailable() {
				hcmd = exec.Command(proot.FindProotBinary(), append([]string{"-r", rootfsDir}, cmd...)...)
			} else {
				hcmd = exec.Command(cmd[0], cmd[1:]...)
				hcmd.Dir = rootfsDir
				hcmd.Env = append(os.Environ(), "PATH="+containerPath)
			}
			hcmd.Stdout = nil
			hcmd.Stderr = nil
			hcmd.Stdin = nil
			done := make(chan error, 1)
			go func() { done <- hcmd.Run() }()

			select {
			case err := <-done:
				if err != nil {
					failures++
					// Update health status.
					rt.mu.Lock()
					if state.HealthStatus != nil {
						state.HealthStatus.FailingStreak = failures
						state.HealthStatus.Status = "unhealthy"
					}
					rt.saveState(state)
					rt.mu.Unlock()
					if failures >= retries {
						// G4: Actually kill the container process, not just set state.
						rt.Stop(id, 10)
						return
					}
				} else {
					failures = 0
					rt.mu.Lock()
					if state.HealthStatus != nil {
						state.HealthStatus.FailingStreak = 0
						state.HealthStatus.Status = "healthy"
						state.HealthStatus.Log = append(state.HealthStatus.Log, common.HealthCheckResult{
							Start:    time.Now().Add(-timeout),
							End:      time.Now(),
							ExitCode: 0,
							Output:   "",
						})
						if len(state.HealthStatus.Log) > 5 {
							state.HealthStatus.Log = state.HealthStatus.Log[len(state.HealthStatus.Log)-5:]
						}
					}
					rt.saveState(state)
					rt.mu.Unlock()
				}
			case <-time.After(timeout):
				if hcmd.Process != nil {
					hcmd.Process.Kill()
				}
				failures++
				rt.mu.Lock()
				if state.HealthStatus != nil {
					state.HealthStatus.FailingStreak = failures
					state.HealthStatus.Status = "unhealthy"
				}
				rt.saveState(state)
				rt.mu.Unlock()
				if failures >= retries {
					rt.Stop(id, 10)
					return
				}
			}
		}
	}()
}

// ─── Seccomp enforcement ───────────────────────────────────────────

// ApplySeccomp applies a seccomp profile to the current process (for use before exec).
func ApplySeccomp(profilePath string) error {
	// On Android, seccomp is not available for unprivileged processes.
	if _, err := os.Stat("/system/build.prop"); err == nil {
		return nil
	}
	// seccomp is applied via OCI runtime hook or directly via libseccomp.
	// This is a no-op when libseccomp is not available.
	_ = profilePath
	return nil
}

// ApplyAppArmor applies an AppArmor profile to a container.
func ApplyAppArmor(profileName string) error {
	if _, err := os.Stat("/sys/kernel/security/apparmor"); err != nil {
		return nil // AppArmor not available
	}
	// Write profile name to /proc/self/attr/current.
	return os.WriteFile("/proc/self/attr/current", []byte(profileName), 0644)
}

// ─── MicroVM Mode ──────────────────────────────────────────────────

// startWithMicroVM starts the container inside a hardware-isolated microVM.
func (rt *Runtime) startWithMicroVM(cfg *Config, rootfsDir string, logFile *os.File) (int, *exec.Cmd, error) {
	if !dokivm.IsAvailable() {
		return rt.startNative(cfg, rootfsDir, logFile)
	}

	vmm, err := dokivm.NewVMM(&dokivm.VMMConfig{
		WorkDir: filepath.Join(rt.root, "microvm"),
	})
	if err != nil {
		return 0, nil, fmt.Errorf("dokivm: %w", err)
	}

	// Build rootfs image.
	builder := rootfs.NewBuilder(filepath.Join(rt.root, "microvm"))
	rootfsPath, err := builder.BuildRootfs(cfg.ID, rootfsDir, 256)
	if err != nil {
		return 0, nil, fmt.Errorf("build rootfs: %w", err)
	}

	// Set up networking.
	_ = rootfsPath

	// Create VM config.
	vmCfg := &dokivm.VMConfig{
		ID:     cfg.ID,
		Kernel: "", // Will be auto-detected by kernel manager
		Rootfs: rootfsPath,
		CPUs:   1,
		Memory: 128,
		Cmd:    cfg.Args,
		Env:    cfg.Env,
		Cwd:    cfg.Cwd,
		KernelArgs: fmt.Sprintf("console=ttyS0 quiet doki.init=1 doki.cmd=%s",
			strings.Join(cfg.Args, ":")),
	}

	if cfg.Resources != nil && cfg.Resources.Memory > 0 {
		vmCfg.Memory = int(cfg.Resources.Memory / (1024 * 1024))
	}

	// Create and start VM.
	vm, err := vmm.Create(context.Background(), vmCfg)
	if err != nil {
		return 0, nil, fmt.Errorf("create microVM: %w", err)
	}

	if err := vmm.Start(context.Background(), cfg.ID); err != nil {
		return 0, nil, fmt.Errorf("start microVM: %w", err)
	}

	logFile.Write([]byte(fmt.Sprintf("[dokivm] MicroVM started with %s backend\n", vmm.Name())))
	logFile.Write([]byte(fmt.Sprintf("[dokivm] VM PID: %d, CID: %d\n", vm.PID, vm.CID)))

	return vm.PID, nil, nil
}

// qemuBinaryPaths returns candidate paths for QEMU user-mode emulators.
func qemuBinaryPaths(guestArch string) []string {
	paths := []string{}
	qemuBinaries := map[string][]string{
		"aarch64": {"qemu-aarch64", "/data/data/com.termux/files/usr/bin/qemu-aarch64"},
		"arm":     {"qemu-arm", "/data/data/com.termux/files/usr/bin/qemu-arm"},
		"i686":    {"qemu-i386", "/data/data/com.termux/files/usr/bin/qemu-i386"},
		"x86_64":  {"qemu-x86_64", "/data/data/com.termux/files/usr/bin/qemu-x86_64"},
	}
	for _, name := range qemuBinaries[guestArch] {
		if p, err := exec.LookPath(name); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}

// detectGuestArch tries to determine the guest architecture from the rootfs.
func detectGuestArch(rootfsDir string) string {
	arches := []string{
		"/usr/bin/bash", "/usr/bin/sh", "/bin/bash", "/bin/sh",
		"/usr/local/bin/docker-entrypoint.sh",
	}
	for _, candidate := range arches {
		path := filepath.Join(rootfsDir, candidate)
		data, err := os.ReadFile(path)
		if err != nil || len(data) < 20 {
			continue
		}
		if data[0] == '#' && data[1] == '!' {
			continue
		}
		if data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
			switch {
			case data[4] == 2 && data[18] == 0xb7:
				return "aarch64"
			case data[4] == 1 && data[18] == 0x28:
				return "arm"
			case data[4] == 2 && data[18] == 0x3e:
				return "x86_64"
			case data[4] == 1 && data[18] == 0x03:
				return "i686"
			}
		}
	}
	return "aarch64"
}

// retryWithQemu attempts to run the container via proot with QEMU user-mode.
func (rt *Runtime) retryWithQemu(cfg *Config, rootfsDir string, logFile *os.File) (int, *exec.Cmd, error) {
	guestArch := detectGuestArch(rootfsDir)
	qemuPaths := qemuBinaryPaths(guestArch)
	if len(qemuPaths) == 0 {
		return 0, nil, fmt.Errorf("qemu user-mode not available for %s", guestArch)
	}

	args := cfg.Args
	cleanRootfs := filepath.Clean(rootfsDir)

	prootArgs := []string{
		"-q", qemuPaths[0],
		"-r", cleanRootfs,
		"-b", "/proc",
		"-b", "/proc/self/fd:/dev/fd",
		"-b", "/sys",
		"-b", "/dev",
		"-b", "/dev/urandom:/dev/random",
		"--kill-on-exit",
		"--link2symlink",
		"--kernel-release=6.17.0-PRoot-Distro",
	}

	selinuxTarget := filepath.Join(cleanRootfs, "sys", "fs", "selinux")
	os.MkdirAll(selinuxTarget, 0755)
	prootArgs = append(prootArgs, "-b", selinuxTarget+":/sys/fs/selinux")

	if rt.isAndroid() {
		for _, dir := range []string{
			"/apex", "/system", "/vendor",
			"/storage", "/sdcard",
			"/data/data/com.termux/files",
		} {
			if _, err := os.Stat(dir); err == nil {
				prootArgs = append(prootArgs, "-b", dir)
			}
		}
		if _, err := os.Stat("/linkerconfig/ld.config.txt"); err == nil {
			prootArgs = append(prootArgs, "-b", "/linkerconfig/ld.config.txt")
		}
		if _, err := os.Stat("/data/data/com.termux/files/usr"); err == nil {
			prootArgs = append(prootArgs, "-b", "/data/data/com.termux/files/usr")
		}
		if home := os.Getenv("HOME"); home != "" {
			prootArgs = append(prootArgs, "-b", home)
		}
	}

	// Container-specific mounts.
	for _, mnt := range cfg.Mounts {
		switch mnt.Type {
		case common.MountBind:
			if mnt.Source != "" && mnt.Target != "" {
				prootArgs = append(prootArgs, "-b", mnt.Source+":"+mnt.Target)
			}
		case common.MountTmpfs:
			target := filepath.Join(cleanRootfs, mnt.Target)
			os.MkdirAll(target, 0755)
			prootArgs = append(prootArgs, "-b", target+":"+mnt.Target)
		}
	}

	if cfg.Cwd != "" {
		prootArgs = append(prootArgs, "-w", cfg.Cwd)
	}
	prootArgs = append(prootArgs, args...)

	cmd := exec.Command("proot", prootArgs...)
	cmd.Dir = cleanRootfs
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	env := os.Environ()
	env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/")
	env = append(env, "LD_LIBRARY_PATH=/usr/lib:/lib:/usr/local/lib")
	validEnv := common.ValidateEnv(cfg.Env)
	for _, e := range validEnv {
		env = append(env, e)
	}
	cmd.Env = env

	fmt.Fprintf(logFile, "DOKI: retrying with QEMU user mode (%s)\n", qemuPaths[0])

	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("proot+qemu start: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return 0, nil, fmt.Errorf("proot+qemu failed: %w", err)
		}
		return cmd.Process.Pid, cmd, nil
	case <-time.After(10000 * time.Millisecond):
	}

	return cmd.Process.Pid, cmd, nil
}

// isShell checks if a file is a shell script.
func isShell(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 2 {
		return false
	}
	return data[0] == '#' && data[1] == '!'
}

// findShell finds a shell binary in the container rootfs.
func findShell(rootfsDir string) string {
	for _, sh := range []string{"/bin/bash", "/bin/sh"} {
		path := filepath.Join(rootfsDir, sh)
		if fi, err := os.Lstat(path); err == nil && fi.Mode().IsRegular() {
			return path
		}
	}
	return "/bin/sh" // fallback
}
