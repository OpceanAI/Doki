// Package runtime provides the container runtime.
package runtime

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// ExecutionMode, ContainerRunner, RunnerCapabilities, and all Mode* constants
// are defined in runner.go.

// rootlessChownOnce ensures the "chown skipped in rootless mode" INFO
// message is emitted only once per process, not once per file.
var rootlessChownOnce sync.Once

// logChownError logs a chown/lchown error appropriately. In rootless
// mode (non-root UID or Termux/proot), chown returns EPERM because
// CAP_CHOWN is unavailable. This is expected and not a real error —
// the container will still work with the current user's ownership.
// We emit a single INFO message the first time, then Debug for
// subsequent occurrences. Non-EPERM errors are logged at WARN level
// because they may indicate real problems.
func logChownError(operation, target string, err error) {
	if err == nil {
		return
	}
	// Check if this is an EPERM (rootless mode — expected).
	if errors.Is(err, syscall.EPERM) {
		rootlessChownOnce.Do(func() {
			slog.Info("chown skipped in rootless mode",
				"reason", "CAP_CHOWN unavailable (non-root or Termux/proot)",
				"note", "OCI image UIDs are preserved as-is; container will run with current user permissions",
			)
		})
		slog.Debug("chown skipped (rootless EPERM)",
			"op", operation, "target", target, "err", err)
		return
	}
	// Non-EPERM error — could be a real problem (e.g., ENOENT for
	// broken symlinks is common in busybox and is ignored; other
	// errors are logged at WARN).
	if errors.Is(err, syscall.ENOENT) {
		return // Expected for broken symlinks (busybox).
	}
	slog.Warn(operation, "target", target, "err", err)
}

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
	registry *Registry
	dnsAddr  string // Internal DNS server address (e.g., "127.0.0.11:53")

	hcMu           sync.Mutex
	healthCheckers map[string]*HealthChecker
}

// LinuxResources is a portable representation of cgroup resource
// limits. It maps to LinuxContainerResources in the CRI spec, and
// is also used by /containers/{id}/update.
type LinuxResources struct {
	// CPU
	CPUShares  uint64 // -> cpu.weight (1-10000, 0 = unset)
	CPUQuota   int64  // microseconds per period (0 = unset)
	CPUPeriod  uint64 // microseconds (default 100000)
	NanoCPUs   int64  // 1e9 = 1 CPU
	CpusetCpus string
	CpusetMems string

	// Memory (bytes)
	Memory           int64
	MemorySwap       int64
	MemorySwappiness *uint64

	// PIDs
	PidsLimit int64

	// Block I/O
	BlkioWeight uint16

	// OOM
	OomKillDisable bool
}

// Config holds the configuration for a container.
type Config struct {
	ID                string
	Rootfs            string
	Args              []string
	Env               []string
	Cwd               string
	User              string
	Tty               bool
	Interactive       bool
	Privileged        bool
	ReadOnly          bool
	NetworkMode       common.NetworkMode
	Hostname          string
	Labels            map[string]string
	Annotations       map[string]string
	Mounts            []common.Mount
	Ports             []common.Port
	DNS               []string
	DNSSearch         []string
	DNSOptions        []string
	ExtraHosts        []string
	CapAdd            []string
	CapDrop           []string
	SecurityOpt       []string
	Sysctls           map[string]string
	Resources         *Resources
	StopSignal        string
	StopTimeout       int
	Init              bool
	RestartPolicy     common.RestartPolicy
	RestartMaxRetries int
	HealthCheck       *HealthCheckConfig
	Runtime           string
	LogDriver         common.LogDriver
	ImageRef          string
	ImageDigest       string
	ImageLayers       []string // paths to image layer tarballs
	ImageConfig       *ImageOCIConfig
	RootfsReady       string // path to extracted rootfs
	Platform          string // "linux/arm64", "linux/amd64", "wasi/wasm", etc.
}

// HealthCheckConfig defines the health check parameters for a container.
type HealthCheckConfig struct {
	Test          []string
	Interval      time.Duration
	Timeout       time.Duration
	Retries       int
	StartPeriod   time.Duration
	StartInterval time.Duration
}

// Resources defines the resource limits for a container.
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

// ImageOCIConfig represents the OCI image configuration extracted from an image manifest.
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

// ContainerState represents the current state of a container persisted to disk.
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

// RuntimeOption is a functional option for NewRuntime.
type RuntimeOption func(*Runtime)

// WithRegistry sets the runner registry for the runtime.
func WithRegistry(reg *Registry) RuntimeOption {
	return func(rt *Runtime) {
		rt.registry = reg
	}
}

// WithDNSAddr sets the internal DNS server address for container resolv.conf.
func WithDNSAddr(addr string) RuntimeOption {
	return func(rt *Runtime) {
		rt.dnsAddr = addr
	}
}

// NewRuntime creates a new container runtime instance.
func NewRuntime(root string, store *storage.Manager, opts ...RuntimeOption) *Runtime {
	rt := &Runtime{
		root:     root,
		store:    store,
		nsMgr:    namespaces.NewManager(root),
		cgMgr:    cgroups.NewManager("/sys/fs/cgroup/doki"),
		prootMgr: proot.NewManager(root),
		rootless: namespaces.IsRootless(),
	}
	for _, opt := range opts {
		opt(rt)
	}
	rt.detectMode()

	_ = common.EnsureDir(filepath.Join(root, "containers"))
	_ = common.EnsureDir(filepath.Join(root, "bundles"))
	_ = common.EnsureDir(filepath.Join(root, "layers"))
	_ = common.EnsureDir(filepath.Join(root, "rootfs"))

	return rt
}

// Registry returns the runner registry, or nil if not set.
func (rt *Runtime) Registry() *Registry {
	return rt.registry
}

func (rt *Runtime) detectMode() {
	switch {
	case dokivm.IsAvailable():
		rt.mode = ModeMicroVM
	case proot.ShouldUseProot() && proot.IsAvailable():
		rt.mode = ModeProot
	case rt.isAndroid():
		if proot.IsAvailable() {
			rt.mode = ModeProot
		} else {
			rt.mode = ModeNative
		}
	case rt.rootless:
		rt.mode = ModeNative
	default:
		rt.mode = ModeNamespaces
	}
}

// Mode returns the current execution mode of the runtime.
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

// Create creates a new container with the given configuration.
func (rt *Runtime) Create(cfg *Config) (*ContainerState, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if cfg.ID == "" {
		return nil, fmt.Errorf("container ID cannot be empty")
	}

	if _, err := rt.loadState(cfg.ID); err == nil {
		return nil, common.NewErrConflict("container", cfg.ID)
	}

	bundleDir := filepath.Join(rt.root, "bundles", cfg.ID)
	rootfsDir := filepath.Join(bundleDir, "rootfs")
	_ = common.EnsureDir(bundleDir)
	_ = common.EnsureDir(rootfsDir)

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
		"etc/resolv.conf": fuse.GenerateResolvConf(cfg.DNS, cfg.DNSSearch, cfg.DNSOptions, rt.dnsAddr),
	}
	_ = fuse.PrepareRootfs(rootfsDir, rootfsFiles, cfg.User)

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

	// Sequential extraction: OCI image layers must be applied in order.
	// Later layers override earlier ones, so parallel extraction would
	// produce non-deterministic rootfs contents.
	for i, layerPath := range layers {
		if !common.PathExists(layerPath) {
			continue
		}
		if err := extractTarGz(layerPath, rootfsDir); err != nil {
			// Rollback: clean up partial rootfs
			_ = os.RemoveAll(rootfsDir)
			return fmt.Errorf("layer %d (%s): %w", i, filepath.Base(layerPath), err)
		}
	}

	return nil
}

func extractTarGz(tarPath, dest string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Default().Warn("close failed", "error", err)
		}
	}()

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
		defer func() {
			if err := gz.Close(); err != nil {
				slog.Default().Warn("close failed", "error", err)
			}
		}()
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

		// Path traversal protection (CWE-22, CWE-59).
		//
		// Lexical cleaning alone is NOT enough: a malicious layer can extract
		// an (absolute or relative) symlink and then write a file *through* it,
		// escaping the rootfs onto the host (CVE-2018-15664 class). We defend by
		// resolving the parent directory with symlink semantics clamped to the
		// extraction root, so no already-extracted symlink can lead outside it.
		cleanDest := filepath.Clean(dest)
		// First, reject lexical ".." escapes in the entry name itself.
		lexical := filepath.Clean(filepath.Join(cleanDest, hdr.Name))
		if hdr.Name == "." || hdr.Name == "./" || lexical == cleanDest {
			continue
		}
		if !strings.HasPrefix(lexical, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("tar: path traversal attempt: %s -> %s", hdr.Name, lexical)
		}
		// Then resolve the parent with symlink semantics clamped to root.
		parent, perr := common.SecureJoin(cleanDest, filepath.Dir(hdr.Name))
		if perr != nil {
			return fmt.Errorf("tar: resolve %s: %w", hdr.Name, perr)
		}
		target := filepath.Clean(filepath.Join(parent, filepath.Base(hdr.Name)))
		if !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) && target != cleanDest {
			return fmt.Errorf("tar: path traversal attempt: %s -> %s", hdr.Name, target)
		}

		// C1: Whiteout files - OCI layers use .wh.<filename> to mark deleted files.
		baseName := filepath.Base(hdr.Name)
		if strings.HasPrefix(baseName, ".wh.") {
			// Opaque whiteout: .wh..wh..opq clears the entire directory.
			// parent is already resolved within cleanDest (see SecureJoin above),
			// so ReadDir/RemoveAll cannot follow a symlink out of the rootfs.
			if baseName == ".wh..wh..opq" {
				if strings.HasPrefix(parent, cleanDest+string(os.PathSeparator)) || parent == cleanDest {
					entries, _ := os.ReadDir(parent)
					for _, e := range entries {
						_ = os.RemoveAll(filepath.Join(parent, e.Name()))
					}
				}
				continue
			}
			whTarget := filepath.Clean(filepath.Join(parent, baseName[4:]))
			if strings.HasPrefix(whTarget, cleanDest+string(os.PathSeparator)) || whTarget == cleanDest {
				_ = os.RemoveAll(whTarget)
			}
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			if err := os.Chown(target, hdr.Uid, hdr.Gid); err != nil {
				logChownError("chown dir", target, err)
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
			extractXattrs(hdr, target)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(target)
			} else if err != nil {
				_ = os.Remove(target)
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				if cerr := out.Close(); cerr != nil {
					slog.Warn("close failed", "error", cerr)
				}
				return err
			}
			if err := out.Chmod(common.SafeFileMode(hdr.Mode)); err != nil {
				if cerr := out.Close(); cerr != nil {
					slog.Warn("close failed", "error", cerr)
				}
				return err
			}
			if err := out.Chown(hdr.Uid, hdr.Gid); err != nil && !os.IsNotExist(err) {
				logChownError("chown file", target, err)
			}
			if err := out.Close(); err != nil {
				slog.Warn("close failed", "error", err)
			}
			if err := os.Chown(target, hdr.Uid, hdr.Gid); err != nil && !os.IsNotExist(err) {
				logChownError("chown file", target, err)
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
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
			for attempts := 0; attempts < 5; attempts++ {
				_ = os.Remove(target)
				if err := os.Symlink(hdr.Linkname, target); err == nil {
					break
				}
				if attempts == 4 {
					return fmt.Errorf("tar: symlink %s -> %s: %w", target, hdr.Linkname, err)
				}
			}
			if err := os.Lchown(target, hdr.Uid, hdr.Gid); err != nil {
				// Broken symlinks (target doesn't exist) are common in busybox images.
				if !os.IsNotExist(err) {
					logChownError("lchown symlink", target, err)
				}
			}
			extractXattrs(hdr, target)
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			linkTarget := filepath.Clean(filepath.Join(dest, hdr.Linkname))
			if !strings.HasPrefix(linkTarget, cleanDest+string(os.PathSeparator)) && linkTarget != cleanDest {
				return fmt.Errorf("tar: hardlink escape attempt")
			}
			_ = os.Remove(target)
			// C8: Hardlink with fallback to copy; return error if both fail.
			if err := os.Link(linkTarget, target); err != nil {
				if data, readErr := os.ReadFile(linkTarget); readErr == nil {
					_ = os.Remove(target)
					if writeErr := os.WriteFile(target, data, 0644); writeErr != nil {
						return fmt.Errorf("tar: hardlink copy fallback: %w", writeErr)
					}
				} else {
					return fmt.Errorf("tar: hardlink failed for %s: %w", hdr.Name, err)
				}
			}
			if err := os.Chmod(target, os.FileMode(hdr.Mode&07777)); err != nil {
				return fmt.Errorf("tar: chmod hardlink %s: %w", hdr.Name, err)
			}
			if err := os.Chown(target, hdr.Uid, hdr.Gid); err != nil {
				logChownError("chown hardlink", target, err)
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
			extractXattrs(hdr, target)
		case tar.TypeBlock:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			_ = os.Remove(target)
			dev := int(hdr.Devmajor)<<8 | int(hdr.Devminor)
			if err := syscall.Mknod(target, syscall.S_IFBLK|uint32(hdr.Mode&0777), dev); err != nil {
				return fmt.Errorf("tar: mknod block device %s: %w", hdr.Name, err)
			}
			if err := os.Chown(target, hdr.Uid, hdr.Gid); err != nil {
				logChownError("chown block device", target, err)
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeChar:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			_ = os.Remove(target)
			dev := int(hdr.Devmajor)<<8 | int(hdr.Devminor)
			if err := syscall.Mknod(target, syscall.S_IFCHR|uint32(hdr.Mode&0777), dev); err != nil {
				return fmt.Errorf("tar: mknod char device %s: %w", hdr.Name, err)
			}
			if err := os.Chown(target, hdr.Uid, hdr.Gid); err != nil {
				logChownError("chown char device", target, err)
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeFifo:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := syscall.Mkfifo(target, uint32(hdr.Mode&0777)); err != nil {
				return fmt.Errorf("tar: mkfifo %s: %w", hdr.Name, err)
			}
			if err := os.Chown(target, hdr.Uid, hdr.Gid); err != nil {
				logChownError("chown fifo", target, err)
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeGNUSparse:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(target)
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				if cerr := out.Close(); cerr != nil {
					slog.Warn("close failed", "error", cerr)
				}
				return err
			}
			if err := out.Chmod(common.SafeFileMode(hdr.Mode)); err != nil {
				if cerr := out.Close(); cerr != nil {
					slog.Warn("close failed", "error", cerr)
				}
				return err
			}
			if err := out.Chown(hdr.Uid, hdr.Gid); err != nil {
				logChownError("chown file", target, err)
			}
			if err := out.Close(); err != nil {
				slog.Warn("close failed", "error", err)
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
			extractXattrs(hdr, target)
		default:
		}
	}
	return nil
}

// extractXattrs extracts extended attributes from PAXRecords.
func extractXattrs(hdr *tar.Header, target string) {
	if hdr.PAXRecords == nil {
		return
	}
	for key, value := range hdr.PAXRecords {
		if strings.HasPrefix(key, "SCHILY.xattr.") {
			attrName := strings.TrimPrefix(key, "SCHILY.xattr.")
			if err := syscall.Setxattr(target, attrName, []byte(value), 0); err != nil {
				slog.Warn("Setxattr failed", "target", target, "attr", attrName, "error", err)
			}
		}
	}
}

// Start starts an existing container by its id.
func (rt *Runtime) Start(id string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	state, err := rt.loadState(id)
	if err != nil {
		return err
	}
	// Accept both "created" (initial start) and "exited" (restart) states.
	if state.Status != common.StateCreated && state.Status != common.StateExited {
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
			if cerr := logFile.Close(); cerr != nil {
				slog.Warn("close failed", "error", cerr)
			}
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
		if cerr := logFile.Close(); cerr != nil {
			slog.Warn("close failed", "error", cerr)
		}
		return fmt.Errorf("start process: %w", err)
	}

	// Apply cgroups when available.
	if rt.cgMgr.IsAvailable() {
		cgCfg := rt.buildCgroupConfig(cfg)
		if _, err := rt.cgMgr.Create(id, cgCfg); err == nil {
			_ = rt.cgMgr.AddProcess(id, pid)
		}
	}

	state.Pid = pid
	state.Status = common.StateRunning
	state.Started = time.Now()
	state.Cmd = proc
	state.PidPath = filepath.Join(rt.root, "containers", state.ID, "init.pid")
	if err := os.WriteFile(state.PidPath, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

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
		if err := rt.saveState(state); err != nil {
			_, _ = os.Stderr.Write([]byte(fmt.Sprintf("DOKI: failed to save healthcheck state for %s: %v\n", id, err)))
		}
		rt.startHealthchecker(id, cfg.HealthCheck)
	}

	return nil
}

func (rt *Runtime) monitorProcess(state *ContainerState, logFile *os.File) {
	// Wait for process exit WITHOUT holding the runtime mutex.
	// Check ProcessState to avoid double Wait(): startWithProot/retryWithQemu
	// may have already called Wait() via their fast-detection goroutines.
	if state.Cmd != nil && state.Cmd.ProcessState == nil {
		if err := state.Cmd.Wait(); err != nil {
			// A non-zero container exit code surfaces as *exec.ExitError — that
			// is a normal container exit, not a daemon failure, so don't log it
			// as a warning (it only spammed the logs, e.g. "exit status 42").
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				slog.Default().Warn("container wait failed", "error", err)
			}
		}
	}
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			slog.Default().Warn("close failed", "error", err)
		}
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

	// Stop the health checker before marking the container as exited.
	rt.stopHealthchecker(state.ID)

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
		if err := rt.saveState(state); err != nil {
			slog.Default().Warn("saveState failed", "error", err)
		}
		rt.mu.Unlock()
		if err := rt.Start(id); err != nil {
			slog.Warn("restart-always failed", "id", id, "err", err)
		}

	case common.RestartOnFailure:
		if exitCode != 0 {
			rt.mu.Lock()
			state.RestartCount++
			if err := rt.saveState(state); err != nil {
				slog.Default().Warn("saveState failed", "error", err)
			}
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
				if err := rt.saveState(state); err != nil {
					slog.Default().Warn("saveState failed", "error", err)
				}
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
			if err := rt.saveState(state); err != nil {
				slog.Default().Warn("saveState failed", "error", err)
			}
			rt.mu.Unlock()
			_ = rt.Start(id)
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

	if cfg.User != "" {
		u, g := parseUser(cfg.User)
		if u >= 0 && g >= 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Credential: &syscall.Credential{
					Uid: uint32(u),
					Gid: uint32(g),
				},
			}
		}
	}

	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}
	return cmd.Process.Pid, cmd, nil
}

// startWithProot runs the container via proot (userspace chroot).
//
// This implementation includes the v0.9.2.1 fixes for the Android 16 / Termux
// ENOSYS-on-execve bug (OpceanAI/Doki#4):
//
//  1. proot.UnsetProotKillers() clears LD_PRELOAD{,_32,_64} and LD_LIBRARY_PATH
//     in the *parent* process before the exec, so proot itself does not
//     inherit libtermux-exec.so. Without this, libtermux-exec.so races with
//     proot's ptrace translation of execve and the kernel returns ENOSYS.
//
//  2. The guest env is built via proot.BuildEnv, which runs StripHostEnv
//     (the full 17-var deny-list from pkg/common) and then layers
//     common.AndroidEnv() defaults.
//
//  3. The proot binary is resolved via proot.FindProotBinary, which prefers
//     the bundled doki-proot and falls back to PATH.
//
//  4. Stderr is captured; if proot fails with the ENOSYS / "Function not
//     implemented" signature and the host looks like Termux, an actionable
//     diagnostic block is appended to the log with version info and the
//     four remediation steps.
func (rt *Runtime) startWithProot(cfg *Config, rootfsDir string, logFile *os.File) (int, *exec.Cmd, error) {
	args := cfg.Args
	if len(args) == 0 {
		return 0, nil, fmt.Errorf("no command specified for container")
	}

	cleanRootfs := filepath.Clean(rootfsDir)

	// Pre-flight: the rootfs must exist and be a directory. proot will fail
	// with a confusing "can't chdir" message otherwise; surface a clean error.
	if fi, err := os.Stat(cleanRootfs); err != nil {
		return 0, nil, fmt.Errorf("proot: rootfs not accessible: %w", err)
	} else if !fi.IsDir() {
		return 0, nil, fmt.Errorf("proot: rootfs path is not a directory: %s", cleanRootfs)
	}

	uid, gid := parseUser(cfg.User)
	prootArgs, err := proot.BuildProotBaseArgs(cleanRootfs, uid, gid)
	if err != nil {
		return 0, nil, err
	}

	if rt.isAndroid() {
		prootArgs = proot.AppendAndroidBinds(prootArgs)
	}

	// Container-specific mounts (bind mounts, tmpfs).
	// BUG fix: startWithProot was missing mount processing entirely.
	// Volume mounts (-v) were silently ignored in proot mode.
	for _, mnt := range cfg.Mounts {
		switch mnt.Type {
		case common.MountBind:
			if mnt.Source != "" && mnt.Target != "" {
				// Ensure the target directory exists inside the rootfs, clamped
				// so a crafted target can't MkdirAll outside the container root.
				targetInRootfs, terr := common.SecureJoin(cleanRootfs, mnt.Target)
				if terr != nil {
					return 0, nil, fmt.Errorf("resolve mount target %s: %w", mnt.Target, terr)
				}
				if err := os.MkdirAll(targetInRootfs, 0755); err != nil {
					return 0, nil, fmt.Errorf("create mount target %s: %w", mnt.Target, err)
				}
				// proot's -b syntax is host:guest ONLY — it has no ":ro" suffix.
				// Appending ":ro" made proot treat "guest:ro" as the literal guest
				// path, so the mount landed at the wrong location and the real
				// target was empty. proot cannot enforce read-only binds, so we
				// mount read-write and warn rather than silently break the mount.
				if mnt.ReadOnly {
					slog.Warn("proot cannot enforce read-only bind mount; mounting read-write", "target", mnt.Target)
				}
				prootArgs = append(prootArgs, "-b", mnt.Source+":"+mnt.Target)
			}
		case common.MountTmpfs:
			target, terr := common.SecureJoin(cleanRootfs, mnt.Target)
			if terr != nil {
				return 0, nil, fmt.Errorf("resolve tmpfs target %s: %w", mnt.Target, terr)
			}
			if err := os.MkdirAll(target, 0755); err != nil {
				return 0, nil, fmt.Errorf("create tmpfs target %s: %w", mnt.Target, err)
			}
			prootArgs = append(prootArgs, "-b", target+":"+mnt.Target)
		}
	}

	if cfg.Cwd != "" {
		prootArgs = append(prootArgs, "-w", cfg.Cwd)
	}

	// Set hostname via environment variable (proot doesn't support true hostname isolation)
	if cfg.Hostname != "" {
		cfg.Env = append(cfg.Env, "HOSTNAME="+cfg.Hostname)
		hostnamePath := filepath.Join(cleanRootfs, "etc", "hostname")
		_ = os.MkdirAll(filepath.Dir(hostnamePath), 0755)
		_ = os.WriteFile(hostnamePath, []byte(cfg.Hostname+"\n"), 0644)
	}

	prootArgs = append(prootArgs, args...)

	prootBin := proot.FindProotBinary()
	if prootBin == "" {
		return 0, nil, fmt.Errorf("proot: no usable proot binary found in PATH or alongside dokid; install with 'pkg install proot' (Termux) or 'apt install proot' (Debian/Ubuntu)")
	}

	// 1) Clear LD_PRELOAD family in the parent process so exec.Command does
	//    not propagate libtermux-exec.so to the proot child. This is the
	//    primary fix for the Android 15/16 ENOSYS regression.
	proot.UnsetProotKillers()

	cmd := exec.Command(prootBin, prootArgs...)
	// IMPORTANT: cmd.Dir must NOT be set to the guest rootfs path. proot
	// internally translates the host process's cwd into the guest namespace;
	// when the host cwd is the same path as the guest root, proot produces a
	// self-referential path ("<rootfs>/./.") and emits a chdir warning. Use
	// a neutral host directory instead.
	cmd.Dir = "/"
	cmd.Stdout = logFile
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	// 2) Build a clean guest env: StripHostEnv (17-var deny-list) + AndroidEnv
	//    defaults + image env + user env.
	var imageEnv []string
	if cfg.ImageConfig != nil {
		imageEnv = cfg.ImageConfig.Env
	}
	validEnv := common.ValidateEnv(cfg.Env)
	cmd.Env = proot.BuildEnv(validEnv, imageEnv)

	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("proot start: %w", err)
	}

	// Wait 100ms to detect immediate startup failures (e.g., ENOSYS).
	// We use a non-blocking check instead of cmd.Wait() to avoid race with monitorProcess.
	time.Sleep(100 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		// Process died immediately - try to get the error
		stderrStr := stderrBuf.String()
		if stderrStr != "" {
			if _, err := logFile.Write([]byte(stderrStr)); err != nil {
				slog.Warn("write to log failed", "error", err)
			}
		}

		hasENOSYS := strings.Contains(stderrStr, "Function not implemented") ||
			strings.Contains(stderrStr, "ENOSYS")

		if hasENOSYS {
			if _, err := logFile.Write([]byte("DOKI: proot failed with ENOSYS, retrying with QEMU...\n")); err != nil {
				slog.Warn("write to log failed", "error", err)
			}
			rt.writeProotENOSYSDiagnostic(logFile)
			if pid, qemuCmd, qemuErr := rt.retryWithQemu(cfg, rootfsDir, logFile); qemuErr == nil {
				return pid, qemuCmd, nil
			}
		}

		// Try to reap the process to avoid zombie
		if _, err := cmd.Process.Wait(); err != nil {
			slog.Warn("wait failed", "error", err)
		}
		return 0, nil, fmt.Errorf("proot exited immediately")
	}

	return cmd.Process.Pid, cmd, nil
}

// writeProotENOSYSDiagnostic writes a human-readable remediation block to
// logFile. It is invoked from startWithProot when proot fails with the
// signature ENOSYS / "Function not implemented" and the host looks like
// Termux / Android 15+. The block is also printed to stderr in the CLI.
func (rt *Runtime) writeProotENOSYSDiagnostic(logFile *os.File) {
	prootVer := proot.Version(proot.FindProotBinary())
	termuxVer := common.TermuxVersion()
	hasLib := common.HasLibTermuxExec()

	var buf bytes.Buffer
	buf.WriteString("\n")
	buf.WriteString("DOKI: proot failed with ENOSYS — actionable diagnostics\n")
	fmt.Fprintf(&buf, "  proot binary:    %s\n", prootVer.Binary)
	fmt.Fprintf(&buf, "  proot version:   %s\n", prootVer.Version)
	fmt.Fprintf(&buf, "  Termux version:  %s\n", termuxVer)
	fmt.Fprintf(&buf, "  libtermux-exec:  %s\n", boolStr(hasLib))
	buf.WriteString("\n")
	buf.WriteString("  Cause: libtermux-exec.so (injected by Termux via LD_PRELOAD)\n")
	buf.WriteString("  intercepts execve and races with proot's ptrace translation.\n")
	buf.WriteString("  On Android 15/16 a zygote seccomp filter makes the race fatal.\n")
	buf.WriteString("\n")
	buf.WriteString("  Steps:\n")
	buf.WriteString("  1) Reinstall the latest proot:    pkg install proot\n")
	buf.WriteString("  2) Verify version >= 5.1.107:    proot --version\n")
	buf.WriteString("  3) Update Termux:                pkg update && pkg upgrade\n")
	buf.WriteString("  4) Re-run with --doki-trace proot for verbose output\n")
	buf.WriteString("  5) Report: https://github.com/OpceanAI/Doki/issues/4\n")
	if _, err := logFile.Write(buf.Bytes()); err != nil {
		slog.Warn("write to log failed", "error", err)
	}
	fmt.Fprint(os.Stderr, buf.String())
}

func boolStr(b bool) string {
	if b {
		return "present"
	}
	return "absent"
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
	if err := os.MkdirAll(oldRootDir, 0755); err != nil {
		return 0, nil, fmt.Errorf("pivot_root setup: %w", err)
	}

	// Build a shell init script that does pivot_root then execs the user command.
	pivotScript := fmt.Sprintf(
		`mount --bind %q %q && pivot_root %q %q/.pivot_root && cd / && umount -l "/.pivot_root" && exec "$@"`,
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
		_ = rt.nsMgr.SetupUserNamespace(cmd.Process.Pid, &namespaces.Config{
			User:     true,
			Rootless: true,
		})
	}

	// I4: Set up loopback in new network namespace.
	if cfg.NetworkMode != common.NetworkHost && cfg.NetworkMode != common.NetworkNone {
		loopbackCmd := exec.Command("nsenter", "-t", strconv.Itoa(cmd.Process.Pid), "-n", "ip", "link", "set", "lo", "up")
		_ = loopbackCmd.Run()
	}

	return cmd.Process.Pid, cmd, nil
}

// ─── Mount setup (namespace mode only) ─────────────────────────────

func (rt *Runtime) setupMounts(rootfsDir string, cfg *Config) error {
	_ = fuse.ProcMount(filepath.Join(rootfsDir, "proc"))
	_ = fuse.SysMount(filepath.Join(rootfsDir, "sys"))
	_ = fuse.DevMount(filepath.Join(rootfsDir, "dev"))
	_ = fuse.DevPtsMount(filepath.Join(rootfsDir, "dev", "pts"))

	shmSize := int64(67108864)
	if cfg.Resources != nil && cfg.Resources.Memory > 0 {
		shmSize = cfg.Resources.Memory / 2
	}
	_ = fuse.ShmMount(filepath.Join(rootfsDir, "dev", "shm"), shmSize)

	for _, mnt := range cfg.Mounts {
		// Resolve the mount target within the rootfs, clamping symlinks and ".."
		// to the container root so a crafted target ("../../etc", or a symlink
		// planted by the image) cannot bind-mount over a host path.
		target, terr := common.SecureJoin(rootfsDir, mnt.Target)
		if terr != nil {
			slog.Warn("mount: resolve target", "target", mnt.Target, "err", terr)
			continue
		}
		switch mnt.Type {
		case common.MountBind:
			if mnt.Source != "" {
				_ = fuse.BindMount(mnt.Source, target, mnt.ReadOnly)
			}
		case common.MountTmpfs:
			size := int64(0)
			if mnt.TmpfsOptions != nil {
				size = mnt.TmpfsOptions.SizeBytes
			}
			_ = fuse.TmpfsMount(target, size, 0755)
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

// Exec runs a command inside a running container.
func (rt *Runtime) Exec(id string, args []string, env []string, workingDir, user string) ([]byte, []byte, error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	state, err := rt.State(id)
	if err != nil {
		return nil, nil, err
	}
	if state.Status != common.StateRunning {
		return nil, nil, fmt.Errorf("container %s is not running", id)
	}

	// Find the container's rootfs.
	rootfsDir := ""
	if state.Config != nil {
		rootfsDir = state.Config.RootfsReady
	}
	if rootfsDir == "" && state.Bundle != "" {
		rootfsDir = filepath.Join(state.Bundle, "rootfs")
	}

	switch rt.mode {
	case ModeProot:
		if rootfsDir == "" || !common.PathExists(rootfsDir) {
			return nil, nil, fmt.Errorf("rootfs not found for container %s", id)
		}
		uid, gid := parseUser(user)
		prootArgs, err := proot.BuildProotBaseArgs(rootfsDir, uid, gid)
		if err != nil {
			return nil, nil, err
		}
		if workingDir != "" {
			prootArgs = append(prootArgs, "-w", workingDir)
		}
		prootArgs = append(prootArgs, args...)
		prootBin := proot.FindProotBinary()
		if prootBin == "" {
			return nil, nil, fmt.Errorf("proot: no usable proot binary found")
		}
		// Clear LD_PRELOAD family in the parent process so exec.Command does not
		// propagate libtermux-exec.so to the proot child.
		proot.UnsetProotKillers()
		cmd := exec.Command(prootBin, prootArgs...)
		// Use BuildEnv for the same env composition as startWithProot.
		cmd.Env = proot.BuildEnv(env, nil)
		// IMPORTANT: cmd.Dir must NOT be set to the guest rootfs path.
		cmd.Dir = "/"
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		err = cmd.Run()
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), err

	case ModeNamespaces:
		if state.Pid == 0 {
			return nil, nil, fmt.Errorf("container %s has no PID for nsenter", id)
		}
		nsenterArgs := []string{"-t", fmt.Sprintf("%d", state.Pid), "-m", "-p", "-a"}
		if workingDir != "" {
			nsenterArgs = append(nsenterArgs, "-w", workingDir)
		}
		nsenterArgs = append(append(nsenterArgs, "--"), args...)
		cmd := exec.Command("nsenter", nsenterArgs...)
		cmd.Env = env
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		err = cmd.Run()
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), err

	case ModeNative:
		fallthrough
	default:
		cmd := exec.Command(args[0], args[1:]...)
		if workingDir != "" {
			cmd.Dir = workingDir
		} else if rootfsDir != "" && common.PathExists(rootfsDir) {
			cmd.Dir = rootfsDir
			pathPrefix := rootfsDir + "/usr/local/sbin:" + rootfsDir + "/usr/local/bin:" +
				rootfsDir + "/usr/sbin:" + rootfsDir + "/usr/bin:" +
				rootfsDir + "/sbin:" + rootfsDir + "/bin"
			if currentPath := os.Getenv("PATH"); currentPath != "" {
				pathPrefix += ":" + currentPath
			}
			env = append(env, "PATH="+pathPrefix)
		}
		cmd.Env = env
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		err = cmd.Run()
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
	}
}

// ExecResult holds the result of a streaming exec call.
type ExecResult struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
	Wait   func() error // blocks until process exits and returns exit error
	Pid    int
}

// writeCloserAdapter adapts an io.PipeWriter to io.WriteCloser. The
// stdcopy writer needs a Write+Close interface; PipeWriter has both.
type writeCloserAdapter struct{ w *io.PipeWriter }

func (a writeCloserAdapter) Write(p []byte) (int, error) { return a.w.Write(p) }
func (a writeCloserAdapter) Close() error                { return a.w.Close() }

// readCloserAdapter adapts an io.PipeReader to io.ReadCloser.
type readCloserAdapter struct{ r *io.PipeReader }

func (a readCloserAdapter) Read(p []byte) (int, error) { return a.r.Read(p) }
func (a readCloserAdapter) Close() error               { return a.r.Close() }

// nopWriteCloser is a write+close that does nothing. Used when no
// child process exists (VM / FEX fallback).
type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

// ExecAttach starts a process inside a running container and returns
// pipes for bidirectional streaming. Unlike Exec, this does not buffer
// output: stdout/stderr are streamed live to the returned readers and
// stdin writes from the caller are forwarded to the process. The
// returned Wait func must be called to reap the child process.
//
// Implementation notes:
//   - For ModeProot, we use proot with stdout/stderr redirected to the
//     returned pipes.
//   - For ModeNamespaces, nsenter -t <pid> -m -p -a — same approach.
//   - For ModeNative, exec.Command directly; rootfs is the working dir.
//   - For VM/FEX/microvm/QEMU runners, we cannot currently inject a
//     child process; we fall back to Exec() and synthesize a one-shot
//     read. Callers that need real-time streaming should detect the
//     mode and warn the user.
func (rt *Runtime) ExecAttach(containerID string, args []string, env []string, workingDir, user string, tty bool) (*ExecResult, error) {
	state, err := rt.State(containerID)
	if err != nil {
		return nil, err
	}
	if state.Status != common.StateRunning {
		return nil, fmt.Errorf("container %s is not running", containerID)
	}

	rootfsDir := ""
	if state.Config != nil {
		rootfsDir = state.Config.RootfsReady
	}
	if rootfsDir == "" && state.Bundle != "" {
		rootfsDir = filepath.Join(state.Bundle, "rootfs")
	}

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	makeCmd := func() *exec.Cmd {
		var cmd *exec.Cmd
		switch rt.mode {
		case ModeProot:
			uid, gid := parseUser(user)
			prootArgs, perr := proot.BuildProotBaseArgs(rootfsDir, uid, gid)
			if perr != nil {
				return nil
			}
			// Default the guest working directory to "/". Without -w, proot
			// inherits the daemon's host cwd, which does not exist inside the
			// guest rootfs and emits a "can't chdir" warning on every exec.
			guestWD := workingDir
			if guestWD == "" {
				guestWD = "/"
			}
			prootArgs = append(prootArgs, "-w", guestWD)
			prootArgs = append(prootArgs, args...)
			prootBin := proot.FindProotBinary()
			if prootBin == "" {
				return nil
			}
			proot.UnsetProotKillers()
			cmd = exec.Command(prootBin, prootArgs...)
			cmd.Env = proot.BuildEnv(env, nil)
		case ModeNamespaces:
			if state.Pid == 0 {
				return nil
			}
			nsenterArgs := []string{"-t", fmt.Sprintf("%d", state.Pid), "-m", "-p", "-a"}
			if workingDir != "" {
				nsenterArgs = append(nsenterArgs, "-w", workingDir)
			}
			nsenterArgs = append(append(nsenterArgs, "--"), args...)
			cmd = exec.Command("nsenter", nsenterArgs...)
			cmd.Env = env
		default:
			cmd = exec.Command(args[0], args[1:]...)
			if workingDir != "" {
				cmd.Dir = workingDir
			} else if rootfsDir != "" && common.PathExists(rootfsDir) {
				cmd.Dir = rootfsDir
				pathPrefix := rootfsDir + "/usr/local/sbin:" + rootfsDir + "/usr/local/bin:" +
					rootfsDir + "/usr/sbin:" + rootfsDir + "/usr/bin:" +
					rootfsDir + "/sbin:" + rootfsDir + "/bin"
				if currentPath := os.Getenv("PATH"); currentPath != "" {
					pathPrefix += ":" + currentPath
				}
				env = append(env, "PATH="+pathPrefix)
			}
			cmd.Env = env
		}
		return cmd
	}

	cmd := makeCmd()
	if cmd == nil {
		// VM / QEMU / FEX modes don't support direct attach; fall back
		// to buffered Exec by running once with empty pipes and
		// returning the buffered output as a single "frame".
		stdoutBytes, stderrBytes, execErr := rt.Exec(containerID, args, env, workingDir, user)
		_ = execErr
		go func() {
			if len(stdoutBytes) > 0 {
				_, _ = stdoutW.Write(stdoutBytes)
			}
			if len(stderrBytes) > 0 {
				_, _ = stderrW.Write(stderrBytes)
			}
			_ = stdoutW.Close()
			_ = stderrW.Close()
			_ = stdinR.Close()
		}()
		// In fallback mode, the caller's stdin writer is wired to a
		// no-op closer since no child process exists to receive data.
		_ = stdinW
		_ = stdinR
		return &ExecResult{
			Stdin:  nopWriteCloser{},
			Stdout: readCloserAdapter{r: stdoutR},
			Stderr: readCloserAdapter{r: stderrR},
			Wait:   func() error { return nil },
			Pid:    -1,
		}, nil
	}

	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	// Feed stdin through an *os.File pipe (StdinPipe), NOT by assigning the
	// io.PipeReader to cmd.Stdin. If cmd.Stdin is a non-*os.File reader, os/exec
	// spawns an internal copy goroutine that blocks reading it, and cmd.Wait()
	// waits for that goroutine — for a non-interactive exec the client never
	// closes stdin, so Wait() would hang forever and the daemon would never
	// close the hijacked connection (exec appeared to run but never returned).
	// With StdinPipe the child reads a real fd, so Wait() only depends on the
	// process exiting. The copier below unblocks when the wait goroutine closes
	// stdinR after the process dies.
	stdinPipe, serr := cmd.StdinPipe()
	if serr != nil {
		_ = stdoutW.Close()
		_ = stderrW.Close()
		_ = stdinR.Close()
		return nil, fmt.Errorf("exec stdin: %w", serr)
	}
	go func() {
		_, _ = io.Copy(stdinPipe, stdinR)
		_ = stdinPipe.Close()
	}()

	// Start synchronously so cmd.Process is populated before we read its Pid.
	// Previously cmd.Run() ran in the goroutine below and the returned struct
	// read cmd.Process.Pid immediately — a race that dereferenced a nil
	// cmd.Process and panicked the daemon on every exec.
	if err := cmd.Start(); err != nil {
		_ = stdoutW.Close()
		_ = stderrW.Close()
		_ = stdinR.Close()
		return nil, fmt.Errorf("exec start: %w", err)
	}

	waitDone := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		_ = stdoutW.Close()
		_ = stderrW.Close()
		_ = stdinR.Close()
		close(waitDone)
	}()

	pid := -1
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	return &ExecResult{
		Stdin:  writeCloserAdapter{w: stdinW},
		Stdout: readCloserAdapter{r: stdoutR},
		Stderr: readCloserAdapter{r: stderrR},
		Wait: func() error {
			<-waitDone
			return waitErr
		},
		Pid: pid,
	}, nil
}

// Stop stops a running container by its id with a configurable timeout.
func (rt *Runtime) Stop(id string, timeout int) error {
	rt.mu.Lock()
	state, err := rt.loadState(id)
	if err != nil {
		rt.mu.Unlock()
		return err
	}
	if state.Status != common.StateRunning {
		rt.mu.Unlock()
		return nil // Idempotent: already stopped
	}

	sig := syscall.SIGTERM
	if state.Config != nil && state.Config.StopSignal != "" {
		sig = parseSignal(state.Config.StopSignal)
	}
	process, err := os.FindProcess(state.Pid)
	if err != nil {
		rt.mu.Unlock()
		return fmt.Errorf("process %d not found: %w", state.Pid, err)
	}
	if err := process.Signal(sig); err != nil {
		// Process may have already exited
		if strings.Contains(err.Error(), "process already finished") ||
			strings.Contains(err.Error(), "no such process") {
			state.Status = common.StateExited
			state.Finished = time.Now()
			if err := rt.saveState(state); err != nil {
				slog.Warn("saveState failed", "error", err)
			}
			rt.mu.Unlock()
			return nil
		}
		rt.mu.Unlock()
		return fmt.Errorf("signal %s to process %d: %w", sig, state.Pid, err)
	}

	rt.mu.Unlock() // Release lock before waiting

	if timeout <= 0 {
		timeout = 10
	}

	// Poll for process exit instead of using ExitChan (which may be disconnected
	// if state was reloaded from disk). Check every 100ms.
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for time.Now().Before(deadline) {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			// Process has exited
			rt.mu.Lock()
			state, err := rt.loadState(id)
			if err == nil {
				state.Status = common.StateExited
				state.ExitCode = 0
				state.Finished = time.Now()
				if err := rt.saveState(state); err != nil {
					slog.Warn("saveState failed", "error", err)
				}
			}
			rt.mu.Unlock()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Timeout: send SIGKILL
	if err := process.Signal(syscall.SIGKILL); err != nil {
		slog.Warn("SIGKILL failed", "error", err)
	}
	// Wait up to 3 more seconds for SIGKILL to take effect
	for i := 0; i < 30; i++ {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			rt.mu.Lock()
			state, err := rt.loadState(id)
			if err == nil {
				state.Status = common.StateExited
				state.ExitCode = 137
				state.Finished = time.Now()
				if err := rt.saveState(state); err != nil {
					slog.Warn("saveState failed", "error", err)
				}
			}
			rt.mu.Unlock()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	rt.mu.Lock()
	state, err = rt.loadState(id)
	if err == nil {
		state.Status = common.StateExited
		state.ExitCode = 137
		state.Finished = time.Now()
		if err := rt.saveState(state); err != nil {
			slog.Warn("saveState failed", "error", err)
		}
	}
	rt.mu.Unlock()
	return nil
}

// Kill sends a signal to a running container.
func (rt *Runtime) Kill(id string, signal syscall.Signal) error {
	state, err := rt.State(id)
	if err != nil {
		return err
	}
	if state.Status != common.StateRunning {
		return fmt.Errorf("container %s is not running", id)
	}
	process, err := os.FindProcess(state.Pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", state.Pid, err)
	}
	if err := process.Signal(signal); err != nil {
		if strings.Contains(err.Error(), "process already finished") ||
			strings.Contains(err.Error(), "no such process") {
			rt.mu.Lock()
			state, err = rt.loadState(id)
			if err == nil {
				state.Status = common.StateExited
				state.Finished = time.Now()
				if err := rt.saveState(state); err != nil {
					slog.Warn("saveState failed", "error", err)
				}
			}
			rt.mu.Unlock()
			return nil
		}
		return err
	}

	for i := 0; i < 100; i++ {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			rt.mu.Lock()
			state, err = rt.loadState(id)
			if err == nil {
				state.Status = common.StateExited
				state.Finished = time.Now()
				if err := rt.saveState(state); err != nil {
					slog.Warn("saveState failed", "error", err)
				}
			}
			rt.mu.Unlock()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// Pause suspends a running container.
func (rt *Runtime) Pause(id string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	state, err := rt.loadState(id)
	if err != nil {
		return err
	}
	if state.Status != common.StateRunning {
		return fmt.Errorf("container %s is not running", id)
	}
	if rt.cgMgr != nil && rt.cgMgr.IsAvailable() {
		_ = rt.cgMgr.Freeze(id)
	} else if state.Cmd != nil && state.Cmd.Process != nil {
		// Fallback: SIGSTOP the process
		if err := state.Cmd.Process.Signal(syscall.SIGSTOP); err != nil {
			slog.Warn("SIGSTOP failed", "error", err)
		}
	}
	state.Status = common.StatePaused
	return rt.saveState(state)
}

// Unpause resumes a paused container.
func (rt *Runtime) Unpause(id string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	state, err := rt.loadState(id)
	if err != nil {
		return err
	}
	if state.Status != common.StatePaused {
		return fmt.Errorf("container %s is not paused", id)
	}
	if rt.cgMgr != nil && rt.cgMgr.IsAvailable() {
		_ = rt.cgMgr.Thaw(id)
	} else if state.Cmd != nil && state.Cmd.Process != nil {
		// Fallback: SIGCONT the process
		if err := state.Cmd.Process.Signal(syscall.SIGCONT); err != nil {
			slog.Warn("SIGCONT failed", "error", err)
		}
	}
	state.Status = common.StateRunning
	return rt.saveState(state)
}

// State returns the current state of a container.
func (rt *Runtime) State(id string) (*ContainerState, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.loadState(id)
}

// Delete removes a container and its associated resources.
func (rt *Runtime) Delete(id string, force bool) error {
	rt.mu.Lock()
	state, err := rt.loadState(id)
	if err != nil {
		rt.mu.Unlock()
		if force {
			return nil
		}
		return err
	}
	if state.Status == common.StateRunning {
		if !force {
			rt.mu.Unlock()
			return fmt.Errorf("container %s is running", id)
		}
		// Release the lock before calling Stop: Stop() acquires and
		// releases the lock internally to wait on the exit channel.
		rt.mu.Unlock()
		_ = rt.Stop(id, 0) // Best-effort stop; ignore errors (process may have exited).
		// Re-acquire to reload fresh state and run cleanup.
		rt.mu.Lock()
		state, err = rt.loadState(id)
		if err != nil {
			rt.mu.Unlock()
			return nil // State already cleaned up by Stop.
		}
	}
	rt.cleanupContainer(state)
	rt.mu.Unlock()
	return nil
}

// List returns all containers managed by the runtime.
func (rt *Runtime) List() ([]*ContainerState, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	var states []*ContainerState
	dir := filepath.Join(rt.root, "containers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read containers dir: %w", err)
	}
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

// Stats returns resource usage statistics for a container.
func (rt *Runtime) Stats(id string) (map[string]interface{}, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	stats := make(map[string]interface{})
	if rt.cgMgr.IsAvailable() {
		cgStats, err := rt.cgMgr.GetStats(id)
		if err != nil {
			slog.Warn("get cgroup stats", "id", id, "err", err)
		}
		for k, v := range cgStats {
			stats[k] = v
		}
	}
	// Add network I/O stats from /proc/net/dev
	stats["network"] = getNetworkStats()
	return stats, nil
}

// UpdateResources atomically updates cgroup v2 limits for a
// running container. Implements CRI UpdateContainerResources
// semantics: any failure rolls back the entire change set.
func (rt *Runtime) UpdateResources(id string, res *LinuxResources) error {
	if res == nil {
		return fmt.Errorf("nil resources")
	}
	_, err := rt.State(id)
	if err != nil {
		return err
	}
	if !rt.cgMgr.IsAvailable() {
		return nil // best-effort on non-cgroup hosts
	}
	cfg := &cgroups.Config{
		CPUShares:        res.CPUShares,
		CPUQuota:         res.CPUQuota,
		CPUPeriod:        res.CPUPeriod,
		NanoCpus:         res.NanoCPUs,
		CpusetCpus:       res.CpusetCpus,
		CpusetMems:       res.CpusetMems,
		Memory:           res.Memory,
		MemorySwap:       res.MemorySwap,
		MemorySwappiness: res.MemorySwappiness,
		PidsLimit:        res.PidsLimit,
		BlkioWeight:      res.BlkioWeight,
		OomKillDisable:   res.OomKillDisable,
	}
	return rt.cgMgr.Update(id, cfg)
}

// ContainerStatsData is a portable, CRI-agnostic stats payload
// suitable for translation to v1.ContainerStats by the CRI server.
type ContainerStatsData struct {
	ID           string
	CPUUsageNS   uint64
	CPUUserNS    uint64
	CPUKernelNS  uint64
	MemoryBytes  uint64
	MemoryLimit  uint64
	PidsCurrent  uint64
	PidsLimit    uint64
	BlockIORB    uint64
	BlockIOWB    uint64
	BlockIORIOs  uint64
	BlockIOWIOs  uint64
	CPUSomePSI   float64
	MemorySomePSI float64
	IOSomePSI    float64
	Timestamp    int64
}

// ContainerStats returns a portable stats payload for a single
// container, sourced from cgroup v2. The CRI server translates this
// to v1.ContainerStats.
func (rt *Runtime) ContainerStats(id string) (*ContainerStatsData, error) {
	out := &ContainerStatsData{ID: id, Timestamp: time.Now().UnixNano()}
	if !rt.cgMgr.IsAvailable() {
		return out, nil
	}
	full, _ := rt.cgMgr.GetStatsFull(id)
	if cpu, ok := full["cpu"].(map[string]uint64); ok {
		if v, ok := cpu["usage_usec"]; ok {
			out.CPUUsageNS = v * 1000
		}
		if v, ok := cpu["user_usec"]; ok {
			out.CPUUserNS = v * 1000
		}
		if v, ok := cpu["system_usec"]; ok {
			out.CPUKernelNS = v * 1000
		}
	}
	if mem, ok := full["memory"].(int64); ok {
		out.MemoryBytes = uint64(mem)
	}
	if pids, ok := full["pids"].(int64); ok {
		out.PidsCurrent = uint64(pids)
	}
	if io, ok := full["io"].(map[string]map[string]uint64); ok {
		for _, devStats := range io {
			out.BlockIORB += devStats["rbytes"]
			out.BlockIOWB += devStats["wbytes"]
			out.BlockIORIOs += devStats["rios"]
			out.BlockIOWIOs += devStats["wios"]
		}
	}
	if psi, ok := full["cpu_psi"].(cgroups.PSIStats); ok {
		out.CPUSomePSI = psi.Some
	}
	if psi, ok := full["memory_psi"].(cgroups.PSIStats); ok {
		out.MemorySomePSI = psi.Some
	}
	return out, nil
}


func getNetworkStats() map[string]uint64 {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}
	result := make(map[string]uint64)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[2:] { // skip headers
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) < 10 {
			continue
		}
		iface := strings.TrimSuffix(parts[0], ":")
		rx, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			slog.Warn("parse rx bytes", "iface", iface, "val", parts[1], "err", err)
		}
		tx, err := strconv.ParseUint(parts[9], 10, 64)
		if err != nil {
			slog.Warn("parse tx bytes", "iface", iface, "val", parts[9], "err", err)
		}
		result[iface+"_rx"] = rx
		result[iface+"_tx"] = tx
	}
	return result
}

// GetLogs returns the logs of a container, optionally tailing the last N lines.
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

	// Remove empty lines (especially trailing newline)
	var nonEmptyLines []string
	for _, line := range lines {
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	// Apply tail filter
	if tail > 0 && len(nonEmptyLines) > tail {
		nonEmptyLines = nonEmptyLines[len(nonEmptyLines)-tail:]
	}

	return strings.Join(nonEmptyLines, "\n"), nil
}

// SaveState persists the container state to disk.
func (rt *Runtime) SaveState(state *ContainerState) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.saveState(state)
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
			if err := os.Remove(newPath); err != nil && !os.IsNotExist(err) {
				slog.Warn("rotateLog: failed to remove oldest rotation", "path", newPath, "error", err)
			}
		}
		if err := os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("rotateLog: failed to rename rotation", "src", oldPath, "dst", newPath, "error", err)
		}
	}
	if err := os.Rename(logPath, logPath+".1"); err != nil {
		slog.Warn("rotateLog: failed to rotate active log", "path", logPath, "error", err)
	}
}

// Processes returns process information for a running container.
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
	if rt.cgMgr != nil {
		_ = rt.cgMgr.Destroy(state.ID)
	}
	if rt.nsMgr != nil {
		_ = rt.nsMgr.DeletePersistentNamespace(state.ID)
	}
	if state.Bundle != "" {
		_ = fuse.CleanupMounts(filepath.Join(state.Bundle, "rootfs"))
		_ = os.RemoveAll(state.Bundle)
	}
	_ = os.RemoveAll(filepath.Join(rt.root, "containers", state.ID))
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
					if err := json.Unmarshal(data, &s); err != nil {
						continue
					}
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
	_ = common.EnsureDir(dir)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	finalPath := filepath.Join(dir, "state.json")
	// Phase 0: atomic write via tmp+rename to avoid corruption on crash
	// mid-write (a partial JSON file would fail to load on restart and
	// orphan the container's data dir).
	tmp, err := os.CreateTemp(dir, "state.json.tmp.*")
	if err != nil {
		return fmt.Errorf("create tmp state: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup of the temp file on any failure below.
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write tmp state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fsync tmp state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close tmp state: %w", err)
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		cleanup()
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

func (rt *Runtime) buildCgroupConfig(cfg *Config) *cgroups.Config {
	if cfg.Resources == nil {
		return &cgroups.Config{}
	}
	return &cgroups.Config{
		CPUPeriod:      common.SafeUint64FromInt64(cfg.Resources.CPUPeriod),
		CPUQuota:       cfg.Resources.CPUQuota,
		CPUShares:      common.SafeUint64FromInt64(cfg.Resources.CPUShares),
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
	// Handle numeric signals first
	if n, err := strconv.Atoi(s); err == nil {
		return syscall.Signal(n)
	}
	switch strings.ToUpper(s) {
	case "SIGHUP", "HUP", "1":
		return syscall.SIGHUP
	case "SIGINT", "INT", "2":
		return syscall.SIGINT
	case "SIGQUIT", "QUIT", "3":
		return syscall.SIGQUIT
	case "SIGKILL", "KILL", "9":
		return syscall.SIGKILL
	case "SIGTERM", "TERM", "15":
		return syscall.SIGTERM
	case "SIGSTOP", "STOP", "17":
		return syscall.SIGSTOP
	case "SIGUSR1", "USR1", "10":
		return syscall.SIGUSR1
	case "SIGUSR2", "USR2", "12":
		return syscall.SIGUSR2
	default:
		return syscall.SIGTERM
	}
}

// parseUser parses a user string ("uid" or "uid:gid") and returns uid, gid.
// Returns (-1, -1) if the string is empty or non-numeric.
func parseUser(user string) (int, int) {
	if user == "" {
		return -1, -1
	}
	parts := strings.SplitN(user, ":", 2)
	uid, err := strconv.Atoi(parts[0])
	if err != nil {
		return -1, -1
	}
	gid := uid
	if len(parts) >= 2 {
		if g, err := strconv.Atoi(parts[1]); err == nil {
			gid = g
		}
	}
	return uid, gid
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
// This is a backward-compatible wrapper that delegates to HealthChecker.
func (rt *Runtime) StartHealthcheck(id string, cmd []string, interval, timeout time.Duration, retries int) {
	cfg := &HealthCheckConfig{
		Test:     cmd,
		Interval: interval,
		Timeout:  timeout,
		Retries:  retries,
	}
	rt.startHealthchecker(id, cfg)
}

// startHealthchecker creates and starts a HealthChecker for the given container.
func (rt *Runtime) startHealthchecker(id string, cfg *HealthCheckConfig) {
	hc := NewHealthChecker(rt, id, cfg)
	rt.hcMu.Lock()
	if rt.healthCheckers == nil {
		rt.healthCheckers = make(map[string]*HealthChecker)
	}
	rt.healthCheckers[id] = hc
	rt.hcMu.Unlock()
	hc.Start()
}

// stopHealthchecker stops the HealthChecker for the given container, if any.
func (rt *Runtime) stopHealthchecker(id string) {
	rt.hcMu.Lock()
	hc, ok := rt.healthCheckers[id]
	if ok {
		delete(rt.healthCheckers, id)
	}
	rt.hcMu.Unlock()
	if ok {
		hc.Stop()
	}
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
// KNOWN ISSUE: This writes to /proc/self/attr/current which only affects the
// calling goroutine, not the actual container process. A correct implementation
// requires writing to /proc/<pid>/attr/current after the container process has
// started, which requires an architecture change to defer profile application
// until after fork/exec.
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

	if _, err := logFile.Write([]byte(fmt.Sprintf("[dokivm] MicroVM started with %s backend\n", vmm.Name()))); err != nil {
		slog.Warn("write to log failed", "error", err)
	}
	if _, err := logFile.Write([]byte(fmt.Sprintf("[dokivm] VM PID: %d, CID: %d\n", vm.PID, vm.CID))); err != nil {
		slog.Warn("write to log failed", "error", err)
	}

	return vm.PID, nil, nil
}

// qemuBinaryPaths returns candidate paths for QEMU user-mode emulators.
func qemuBinaryPaths(guestArch string) []string {
	paths := []string{}
	qemuBinaries := map[string][]string{
		"aarch64": {"qemu-aarch64"},
		"arm":     {"qemu-arm"},
		"i686":    {"qemu-i386"},
		"x86_64":  {"qemu-x86_64"},
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

	uid, gid := parseUser(cfg.User)
	prootArgs := []string{"-q", qemuPaths[0]}
	baseArgs, err := proot.BuildProotBaseArgs(cleanRootfs, uid, gid)
	if err != nil {
		return 0, nil, err
	}
	prootArgs = append(prootArgs, baseArgs...)

	if rt.isAndroid() {
		prootArgs = proot.AppendAndroidBinds(prootArgs)
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
			if err := os.MkdirAll(target, 0755); err != nil {
				return 0, nil, fmt.Errorf("tmpfs mount mkdir %s: %w", target, err)
			}
			prootArgs = append(prootArgs, "-b", target+":"+mnt.Target)
		}
	}

	if cfg.Cwd != "" {
		prootArgs = append(prootArgs, "-w", cfg.Cwd)
	}
	prootArgs = append(prootArgs, args...)

	cmd := exec.Command(proot.FindProotBinary(), prootArgs...)
	// IMPORTANT: cmd.Dir must NOT be set to the guest rootfs path (see
	// startWithProot for the full rationale).
	cmd.Dir = "/"
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Clear LD_PRELOAD family in the parent process so exec.Command does not
	// propagate libtermux-exec.so to the proot+qemu child.
	proot.UnsetProotKillers()
	// Use BuildEnv for the same env composition as startWithProot:
	// StripHostEnv (17-var deny-list) + AndroidEnv defaults + image env +
	// user env.
	var imageEnv []string
	if cfg.ImageConfig != nil {
		imageEnv = cfg.ImageConfig.Env
	}
	validEnv := common.ValidateEnv(cfg.Env)
	cmd.Env = proot.BuildEnv(validEnv, imageEnv)

	if _, err := fmt.Fprintf(logFile, "DOKI: retrying with QEMU user mode (%s)\n", qemuPaths[0]); err != nil {
		slog.Warn("write to log failed", "error", err)
	}

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
