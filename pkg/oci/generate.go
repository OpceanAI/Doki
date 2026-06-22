package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpceanAI/Doki/pkg/common"
)

// GenerateConfig generates an OCI config.json from a Doki container config.
// It produces a Spec that is compatible with OCI Runtime Spec v1.2.0.
func GenerateConfig(cfg *Config, rootfsPath string) *Spec {
	spec := &Spec{
		Version:  "1.2.0",
		Hostname: cfg.Hostname,
		Root: &Root{
			Path:     rootfsPath,
			Readonly: cfg.ReadOnly,
		},
		Process:     generateProcess(cfg),
		Linux:       generateLinux(cfg),
		Mounts:      generateMounts(cfg),
		Annotations: cfg.Annotations,
	}

	// Add standard annotations.
	if spec.Annotations == nil {
		spec.Annotations = make(map[string]string)
	}
	spec.Annotations["com.doki.container.id"] = cfg.ID
	spec.Annotations["com.doki.image"] = cfg.ImageRef
	if cfg.Runtime != "" {
		spec.Annotations["com.doki.runtime"] = cfg.Runtime
	}
	if cfg.Platform != "" {
		spec.Annotations["com.doki.platform"] = cfg.Platform
	}

	return spec
}

// Config holds the minimal config needed for OCI spec generation.
// This mirrors the runtime.Config fields needed for OCI generation.
type Config struct {
	ID            string
	Hostname      string
	Args          []string
	Env           []string
	Cwd           string
	User          string
	Tty           bool
	Privileged    bool
	ReadOnly      bool
	NetworkMode   string
	Labels        map[string]string
	Annotations   map[string]string
	Mounts        []MountConfig
	Ports         []PortConfig
	DNS           []string
	DNSSearch     []string
	DNSOptions    []string
	ExtraHosts    []string
	CapAdd        []string
	CapDrop       []string
	SecurityOpt   []string
	Sysctls       map[string]string
	Resources     *ResourceConfig
	StopSignal    string
	Init          bool
	RestartPolicy string
	Runtime       string
	ImageRef      string
	ImageConfig   *ImageConfig
	Platform      string
}

// ImageConfig holds the OCI image config.
type ImageConfig struct {
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

// MountConfig holds a mount configuration.
type MountConfig struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

// PortConfig holds a port configuration.
type PortConfig struct {
	PrivatePort uint16
	PublicPort  uint16
	Type        string
}

// ResourceConfig holds resource limits.
type ResourceConfig struct {
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

func generateProcess(cfg *Config) *Process {
	p := &Process{
		Terminal: cfg.Tty,
		Cwd:      cfg.Cwd,
		Env:      cfg.Env,
	}

	// Determine command: entrypoint + cmd.
	entrypoint := cfg.Args
	cmd := []string{}
	if cfg.ImageConfig != nil {
		if len(cfg.ImageConfig.Entrypoint) > 0 && len(cfg.Args) == 0 {
			entrypoint = cfg.ImageConfig.Entrypoint
		}
		if len(cfg.ImageConfig.Cmd) > 0 {
			cmd = cfg.ImageConfig.Cmd
		}
		if len(cfg.ImageConfig.Env) > 0 {
			// Merge image env with user env (user takes precedence).
			envMap := make(map[string]string)
			for _, e := range cfg.ImageConfig.Env {
				k, v := parseEnv(e)
				if k != "" {
					envMap[k] = v
				}
			}
			for _, e := range cfg.Env {
				k, v := parseEnv(e)
				if k != "" {
					envMap[k] = v
				}
			}
			merged := make([]string, 0, len(envMap))
			for k, v := range envMap {
				merged = append(merged, k+"="+v)
			}
			p.Env = merged
		}
		if cfg.Cwd == "" && cfg.ImageConfig.WorkingDir != "" {
			p.Cwd = cfg.ImageConfig.WorkingDir
		}
	}

	// Combine entrypoint + cmd.
	if len(entrypoint) > 0 {
		p.Args = append(entrypoint, cmd...)
	} else if len(cmd) > 0 {
		p.Args = cmd
	}
	p.Entrypoint = entrypoint
	p.Cmd = cmd

	// Default cwd.
	if p.Cwd == "" {
		p.Cwd = "/"
	}

	// User.
	if cfg.User != "" {
		uid, gid, additionalGids := parseUser(cfg.User)
		p.User = User{UID: uid, GID: gid, AdditionalGids: additionalGids}
	}

	// Capabilities.
	if cfg.Privileged {
		p.Capabilities = &LinuxCapabilities{
			Bounding:    AllCapabilities(),
			Effective:   AllCapabilities(),
			Inheritable: AllCapabilities(),
			Permitted:   AllCapabilities(),
			Ambient:     AllCapabilities(),
		}
	} else {
		caps := DefaultCapabilities()
		caps = applyCapChanges(caps, cfg.CapAdd, cfg.CapDrop)
		p.Capabilities = &LinuxCapabilities{
			Bounding:    caps,
			Effective:   caps,
			Inheritable: caps,
			Permitted:   caps,
			Ambient:     caps,
		}
	}

	// AppArmor.
	if len(cfg.SecurityOpt) > 0 {
		for _, opt := range cfg.SecurityOpt {
			if strings.HasPrefix(opt, "apparmor=") {
				p.ApparmorProfile = strings.TrimPrefix(opt, "apparmor=")
			}
		}
	}

	// NoNewPrivileges.
	if contains(cfg.SecurityOpt, "no-new-privileges") {
		p.NoNewPrivileges = true
	}

	// StopSignal.
	if cfg.StopSignal != "" {
		// StopSignal is stored at the spec level, not process level.
	}

	// OOMScoreAdj is in HostConfig, not in Config.
	// We'll set it to nil for now.

	return p
}

func generateLinux(cfg *Config) *Linux {
	l := &Linux{
		Namespaces:    DefaultNamespaces(false),
		MaskedPaths:   MaskedPaths(),
		ReadonlyPaths: ReadonlyPaths(),
	}

	// User namespaces for rootless.
	if !cfg.Privileged {
		// Add user namespace.
		hasUser := false
		for _, ns := range l.Namespaces {
			if ns.Type == NamespaceUser {
				hasUser = true
				break
			}
		}
		if !hasUser {
			l.Namespaces = append(l.Namespaces, LinuxNamespace{Type: NamespaceUser})
		}
	}

	// Remove network namespace if host networking.
	if cfg.NetworkMode == "host" {
		filtered := make([]LinuxNamespace, 0, len(l.Namespaces))
		for _, ns := range l.Namespaces {
			if ns.Type != NamespaceNetwork {
				filtered = append(filtered, ns)
			}
		}
		l.Namespaces = filtered
	}

	// Sysctls.
	if len(cfg.Sysctls) > 0 {
		l.Sysctl = cfg.Sysctls
	}

	// Resources.
	if cfg.Resources != nil {
		l.Resources = generateResources(cfg.Resources)
	}

	// Seccomp.
	if cfg.Privileged {
		l.Seccomp = PrivilegedSeccompProfile()
	} else {
		l.Seccomp = DefaultSeccompProfile()
		// Apply security opt seccomp overrides.
		for _, opt := range cfg.SecurityOpt {
			if strings.HasPrefix(opt, "seccomp=") {
				// Custom seccomp profile path.
				profilePath := strings.TrimPrefix(opt, "seccomp=")
				if data, err := os.ReadFile(profilePath); err == nil {
					var profile LinuxSeccomp
					if json.Unmarshal(data, &profile) == nil {
						l.Seccomp = &profile
					}
				}
			}
		}
	}

	// Devices.
	l.Devices = defaultDevices()

	// CgroupsPath.
	l.CgroupsPath = fmt.Sprintf("/doki/%s", cfg.ID)

	return l
}

func generateResources(r *ResourceConfig) *LinuxResources {
	res := &LinuxResources{}

	if r.Memory > 0 {
		mem := &LinuxMemory{Limit: &r.Memory}
		if r.MemorySwap > 0 {
			mem.Swap = &r.MemorySwap
		}
		if r.OomKillDisable {
			mem.DisableOOMKiller = &r.OomKillDisable
		}
		res.Memory = mem
	}

	if r.CPUShares > 0 || r.NanoCpus > 0 || r.CPUPeriod > 0 || r.CPUQuota > 0 || r.CpusetCpus != "" {
		cpu := &LinuxCPU{}
		if r.CPUShares > 0 {
			shares := uint64(r.CPUShares)
			cpu.Shares = &shares
		}
		if r.NanoCpus > 0 {
			// Convert nanocpus to quota/period.
			// BUG fix: the previous formula (NanoCpus / 1000) produced a
			// quota/period ratio 10x too large. For 1 CPU (NanoCpus=1e9),
			// the code produced quota=1,000,000 with period=100,000,
			// yielding 10 CPUs. The correct formula is:
			// quota = NanoCpus * period / 1e9
			period := uint64(100000)
			quota := int64(float64(r.NanoCpus) * float64(period) / 1e9)
			cpu.Period = &period
			cpu.Quota = &quota
		}
		if r.CPUPeriod > 0 {
			period := uint64(r.CPUPeriod)
			cpu.Period = &period
		}
		if r.CPUQuota > 0 {
			cpu.Quota = &r.CPUQuota
		}
		if r.CpusetCpus != "" {
			cpu.Cpus = r.CpusetCpus
		}
		if r.CpusetMems != "" {
			cpu.Mems = r.CpusetMems
		}
		res.CPU = cpu
	}

	if r.PidsLimit > 0 {
		res.Pids = &LinuxPids{Limit: r.PidsLimit}
	}

	if r.BlkioWeight > 0 {
		res.BlockIO = &LinuxBlockIO{Weight: &r.BlkioWeight}
	}

	return res
}

func generateMounts(cfg *Config) []Mount {
	var mounts []Mount

	// Standard mounts.
	mounts = append(mounts, Mount{
		Destination: "/proc",
		Type:        "proc",
		Source:      "proc",
	})

	mounts = append(mounts, Mount{
		Destination: "/dev",
		Type:        "tmpfs",
		Source:      "tmpfs",
		Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
	})

	mounts = append(mounts, Mount{
		Destination: "/dev/pts",
		Type:        "devpts",
		Source:      "devpts",
		Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"},
	})

	mounts = append(mounts, Mount{
		Destination: "/dev/shm",
		Type:        "tmpfs",
		Source:      "shm",
		Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
	})

	mounts = append(mounts, Mount{
		Destination: "/dev/mqueue",
		Type:        "mqueue",
		Source:      "mqueue",
		Options:     []string{"nosuid", "noexec", "nodev"},
	})

	mounts = append(mounts, Mount{
		Destination: "/sys",
		Type:        "sysfs",
		Source:      "sysfs",
		Options:     []string{"nosuid", "noexec", "nodev", "ro"},
	})

	// User mounts.
	for _, m := range cfg.Mounts {
		mount := Mount{
			Destination: m.Target,
			Source:      m.Source,
		}
		if m.Type == "bind" {
			mount.Type = "bind"
			mount.Options = []string{"rbind"}
			if m.ReadOnly {
				mount.Options = append(mount.Options, "ro")
			}
		} else if m.Type == "tmpfs" {
			mount.Type = "tmpfs"
			mount.Source = "tmpfs"
		}
		mounts = append(mounts, mount)
	}

	return mounts
}

func defaultDevices() []LinuxDevice {
	return []LinuxDevice{
		{Path: "/dev/null", Type: "c", Major: 1, Minor: 3, FileMode: uint32Ptr(0666)},
		{Path: "/dev/zero", Type: "c", Major: 1, Minor: 5, FileMode: uint32Ptr(0666)},
		{Path: "/dev/full", Type: "c", Major: 1, Minor: 7, FileMode: uint32Ptr(0666)},
		{Path: "/dev/random", Type: "c", Major: 1, Minor: 8, FileMode: uint32Ptr(0666)},
		{Path: "/dev/urandom", Type: "c", Major: 1, Minor: 9, FileMode: uint32Ptr(0666)},
		{Path: "/dev/tty", Type: "c", Major: 5, Minor: 0, FileMode: uint32Ptr(0666)},
		{Path: "/dev/console", Type: "c", Major: 5, Minor: 1, FileMode: uint32Ptr(0600)},
		{Path: "/dev/ptmx", Type: "c", Major: 5, Minor: 2, FileMode: uint32Ptr(0666)},
	}
}

func uint32Ptr(v uint32) *uint32 { return &v }

func parseEnv(env string) (string, string) {
	parts := strings.SplitN(env, "=", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return env, ""
}

func parseUser(user string) (uid, gid uint32, additionalGids []uint32) {
	// Default: root
	uid, gid = 0, 0

	if user == "" {
		return
	}

	// Parse "uid:gid" format.
	parts := strings.SplitN(user, ":", 2)
	if len(parts) >= 1 {
		if id, err := parseUint32(parts[0]); err == nil {
			uid = id
		}
	}
	if len(parts) >= 2 {
		if id, err := parseUint32(parts[1]); err == nil {
			gid = id
		}
	}

	return
}

func parseUint32(s string) (uint32, error) {
	var v uint64
	_, err := fmt.Sscanf(s, "%d", &v)
	return uint32(v), err
}

func applyCapChanges(base, add, drop []string) []string {
	capSet := make(map[string]bool)
	for _, c := range base {
		capSet[c] = true
	}
	for _, c := range add {
		if parsed, err := ParseCapability(c); err == nil {
			capSet[parsed] = true
		}
	}
	for _, c := range drop {
		if parsed, err := ParseCapability(c); err == nil {
			delete(capSet, parsed)
		}
	}
	result := make([]string, 0, len(capSet))
	for c := range capSet {
		result = append(result, c)
	}
	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// WriteConfig writes an OCI config.json to a bundle directory.
func WriteConfig(bundleDir string, spec *Spec) error {
	data, err := MarshalSpec(spec)
	if err != nil {
		return fmt.Errorf("marshal spec: %w", err)
	}
	configPath := filepath.Join(bundleDir, "config.json")
	return os.WriteFile(configPath, data, 0644)
}

// ReadConfig reads an OCI config.json from a bundle directory.
func ReadConfig(bundleDir string) (*Spec, error) {
	configPath := filepath.Join(bundleDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &spec, nil
}

// CommonConfigToConfig converts a common.ContainerConfig to oci.Config.
func CommonConfigToConfig(commonCfg *common.ContainerConfig, id string) *Config {
	cfg := &Config{
		ID:       id,
		Hostname: commonCfg.Hostname,
		Args:     commonCfg.Cmd,
		Env:      commonCfg.Env,
		Cwd:      commonCfg.WorkingDir,
		User:     commonCfg.User,
		Runtime:  "auto",
	}
	if commonCfg.Image != "" {
		cfg.ImageRef = commonCfg.Image
	}
	return cfg
}
