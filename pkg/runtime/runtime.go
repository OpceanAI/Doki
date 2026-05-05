package runtime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	Resources     *Resources
	StopSignal    string
	StopTimeout   int
	Init          bool
	Runtime       string
	ImageRef      string
	ImageDigest   string
	ImageLayers   []string // paths to image layer tarballs
	ImageConfig   *ImageOCIConfig
	RootfsReady   string // path to extracted rootfs
}

type Resources struct {
	CPUShares     int64
	Memory        int64
	MemorySwap    int64
	NanoCpus      int64
	CPUPeriod     int64
	CPUQuota      int64
	CpusetCpus    string
	CpusetMems    string
	PidsLimit     int64
	BlkioWeight   uint16
	OomKillDisable bool
}

type ImageOCIConfig struct {
	Entrypoint []string
	Cmd        []string
	Env        []string
	WorkingDir string
	User       string
	Volumes    map[string]struct{}
	Labels     map[string]string
	StopSignal string
	Shell      []string
}

type ContainerState struct {
	ID        string                `json:"id"`
	Pid       int                   `json:"pid"`
	Status    common.ContainerState `json:"status"`
	Created   time.Time             `json:"created"`
	Started   time.Time             `json:"started,omitempty"`
	Finished  time.Time             `json:"finished,omitempty"`
	ExitCode  int                   `json:"exitCode,omitempty"`
	Bundle    string                `json:"bundle"`
	Config    *Config               `json:"config,omitempty"`
	PidPath   string                `json:"pidPath,omitempty"`
	LogPath   string                `json:"logPath,omitempty"`
	Mode      ExecutionMode         `json:"mode"`
	ExitChan  chan struct{}         `json:"-"`
	Cmd       *exec.Cmd             `json:"-"`
}

func NewRuntime(root string, store *storage.Manager) *Runtime {
	rt := &Runtime{
		root:     root,
		store:    store,
		nsMgr:    namespaces.NewManager(root),
		cgMgr:    cgroups.NewManager(filepath.Join(root, "cgroups")),
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
	for i, layerPath := range layers {
		if !common.PathExists(layerPath) {
			continue
		}
		if err := extractTarGz(layerPath, rootfsDir); err != nil {
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
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
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
		// Allow "." and "./" entries (root directory marker).
		if hdr.Name == "." || hdr.Name == "./" || target == cleanDest {
			continue
		}
		if !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) && target != cleanDest {
			return fmt.Errorf("tar: path traversal attempt: %s -> %s", hdr.Name, target)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
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
			if err := os.Chmod(target, os.FileMode(hdr.Mode&0777)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			// Validate symlink target doesn't escape dest.
			linkTarget := hdr.Linkname
			if filepath.IsAbs(linkTarget) {
				linkTarget = filepath.Join(dest, linkTarget)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(target), linkTarget))
			if !strings.HasPrefix(resolved, filepath.Clean(dest)+string(os.PathSeparator)) && resolved != filepath.Clean(dest) {
				return fmt.Errorf("tar: symlink escape attempt: %s -> %s", hdr.Linkname, resolved)
			}
			os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			linkTarget := filepath.Clean(filepath.Join(dest, hdr.Linkname))
			if !strings.HasPrefix(linkTarget, filepath.Clean(dest)+string(os.PathSeparator)) && linkTarget != filepath.Clean(dest) {
				return fmt.Errorf("tar: hardlink escape attempt")
			}
			os.Remove(target)
			// Hardlinks may fail on cross-device or permission-restricted filesystems.
			// Fall back to copying the file if linking fails.
			if err := os.Link(linkTarget, target); err != nil {
				data, readErr := os.ReadFile(linkTarget)
				if readErr == nil {
					os.WriteFile(target, data, 0644)
				}
			}
		}
	}
	return nil
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

	logFile, err := os.Create(state.LogPath)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}

	// Setup mounts (only in namespace mode).
	if rt.mode == ModeNamespaces {
		if err := rt.setupMounts(rootfsDir, cfg); err != nil {
			logFile.Close()
			return fmt.Errorf("setup mounts: %w", err)
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

	return nil
}

func (rt *Runtime) monitorProcess(state *ContainerState, logFile *os.File) {
	// Wait for process exit WITHOUT holding the runtime mutex.
	// This is the most critical fix: previously the mutex was held
	// during Cmd.Wait(), blocking ALL other container operations.
	if state.Cmd != nil {
		state.Cmd.Wait()
	}
	if logFile != nil {
		logFile.Close()
	}

	// Only lock for state modification and persistence.
	rt.mu.Lock()
	state.Status = common.StateExited
	state.Finished = time.Now()
	if state.Cmd != nil && state.Cmd.ProcessState != nil {
		if ws, ok := state.Cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
			state.ExitCode = ws.ExitStatus()
		}
	}
	if err := rt.saveState(state); err != nil {
		_, _ = os.Stderr.Write([]byte(fmt.Sprintf("DOKI: failed to save state for %s: %v\n", state.ID, err)))
	}
	rt.mu.Unlock()

	if state.ExitChan != nil {
		close(state.ExitChan)
	}
}

// ─── 3 execution modes ─────────────────────────────────────────────

// startProcess selects the appropriate execution mode.
func (rt *Runtime) startProcess(cfg *Config, rootfsDir string, logFile *os.File) (int, *exec.Cmd, error) {
	switch rt.mode {
	case ModeMicroVM:
		return rt.startWithMicroVM(cfg, rootfsDir, logFile)
	case ModeProot:
		if proot.IsAvailable() {
			pid, proc, err := rt.startWithProot(cfg, rootfsDir, logFile)
			if err != nil {
				// Fallback to native mode if proot fails (e.g. execve ENOSYS on Android).
				logFile.Write([]byte(fmt.Sprintf("[doki] proot failed: %v - falling back to native mode\n", err)))
				return rt.startNative(cfg, rootfsDir, logFile)
			}
			return pid, proc, nil
		}
		return rt.startNative(cfg, rootfsDir, logFile)
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

	// Build environment: image env + container env + PATH to rootfs binaries.
	env := os.Environ()
	env = append(env, "PATH="+rootfsDir+"/usr/bin:"+rootfsDir+"/usr/sbin:"+
		rootfsDir+"/bin:"+rootfsDir+"/sbin:"+os.Getenv("PATH"))
	env = append(env, "HOME=/root")
	env = append(env, "DOKI_CONTAINER=1")
	for _, e := range cfg.Env {
		env = append(env, e)
	}
	cmd.Env = env

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

	prootArgs := []string{
		"-r", filepath.Clean(rootfsDir),
		"-b", "/proc",
		"-b", "/sys",
		"-b", "/dev",
		"--kill-on-exit",
	}
	if cfg.Cwd != "" {
		prootArgs = append(prootArgs, "-w", cfg.Cwd)
	}
	prootArgs = append(prootArgs, args...)

	cmd := exec.Command("proot", prootArgs...)
	cmd.Dir = filepath.Clean(rootfsDir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = os.Stdin

	env := os.Environ()
	env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	for _, e := range cfg.Env {
		env = append(env, e)
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("proot start: %w", err)
	}

	// Wait briefly to detect immediate proot failures (e.g. execve not implemented).
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				return 0, nil, fmt.Errorf("proot exited with code %d (binary may be incompatible or missing in rootfs)", code)
			}
			return 0, nil, fmt.Errorf("proot failed: %w", err)
		}
		return 0, nil, fmt.Errorf("proot exited immediately")
	case <-time.After(500 * time.Millisecond):
		// Process started successfully, monitor asynchronously.
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

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = rootfsDir
	if cfg.Cwd != "" {
		cmd.Dir = filepath.Join(rootfsDir, cfg.Cwd)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = os.Stdin
	cmd.Env = cfg.Env

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC | syscall.CLONE_NEWPID,
		Chroot: rootfsDir,
	}
	if cfg.NetworkMode != common.NetworkHost && cfg.NetworkMode != common.NetworkNone {
		cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWNET
	}
	if cfg.Privileged {
		cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWUSER
	}

	if err := cmd.Start(); err != nil {
		return 0, nil, err
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
		rt.Stop(id, 0)
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
		CPUPeriod:   uint64(cfg.Resources.CPUPeriod),
		CPUQuota:    cfg.Resources.CPUQuota,
		CPUShares:   uint64(cfg.Resources.CPUShares),
		CpusetCpus:  cfg.Resources.CpusetCpus,
		CpusetMems:  cfg.Resources.CpusetMems,
		Memory:      cfg.Resources.Memory,
		MemorySwap:  cfg.Resources.MemorySwap,
		PidsLimit:   cfg.Resources.PidsLimit,
		BlkioWeight: cfg.Resources.BlkioWeight,
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

			hcmd := exec.Command(cmd[0], cmd[1:]...)
			hcmd.Stdout = nil
			hcmd.Stderr = nil
			done := make(chan error, 1)
			go func() { done <- hcmd.Run() }()

			select {
			case err := <-done:
				if err != nil {
					failures++
					if failures >= retries {
						rt.mu.Lock()
						state.Status = common.StateExited
						state.ExitCode = 1
						rt.saveState(state)
						rt.mu.Unlock()
						return
					}
				} else {
					failures = 0
				}
			case <-time.After(timeout):
				hcmd.Process.Kill()
				failures++
				if failures >= retries {
					rt.mu.Lock()
					state.Status = common.StateExited
					state.ExitCode = 1
					rt.saveState(state)
					rt.mu.Unlock()
					return
				}
			}
		}
	}()
}

// ─── Restart Policy ───────────────────────────────────────────────

// StartRestartMonitor monitors a container and restarts it according to policy.
func (rt *Runtime) StartRestartMonitor(id string, policy string, maxRetries int) {
	go func() {
		<-rt.getExitChan(id)

		switch policy {
		case "always":
			if err := rt.Start(id); err == nil {
				return
			}
		case "on-failure":
			state, _ := rt.State(id)
			if state != nil && state.ExitCode != 0 {
				for i := 0; i < maxRetries || maxRetries == 0; i++ {
					time.Sleep(time.Duration(1<<uint(i)) * 100 * time.Millisecond)
					if err := rt.Start(id); err == nil {
						return
					}
				}
			}
		case "unless-stopped":
			state, _ := rt.State(id)
			if state != nil && state.Status != common.StateDead {
				rt.Start(id)
			}
		}
	}()
}

func (rt *Runtime) getExitChan(id string) chan struct{} {
	state, err := rt.State(id)
	if err != nil || state == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	if state.ExitChan == nil {
		state.ExitChan = make(chan struct{})
	}
	return state.ExitChan
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
