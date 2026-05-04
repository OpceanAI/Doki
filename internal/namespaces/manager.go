package namespaces

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Type represents a Linux namespace type.
type Type string

const (
	UserNS   Type = "user"
	MountNS  Type = "mount"
	PIDNS    Type = "pid"
	NetNS    Type = "net"
	UTSNS    Type = "uts"
	IPCNS    Type = "ipc"
	CgroupNS Type = "cgroup"
)

// CLONE flags for each namespace type.
var cloneFlags = map[Type]int{
	UserNS:   syscall.CLONE_NEWUSER,
	MountNS:  syscall.CLONE_NEWNS,
	PIDNS:    syscall.CLONE_NEWPID,
	NetNS:    syscall.CLONE_NEWNET,
	UTSNS:    syscall.CLONE_NEWUTS,
	IPCNS:    syscall.CLONE_NEWIPC,
	CgroupNS: syscall.CLONE_NEWCGROUP,
}

// Manager manages Linux namespaces for containers.
type Manager struct {
	root string
}

// NewManager creates a new namespace manager.
func NewManager(root string) *Manager {
	return &Manager{root: root}
}

// Config holds the configuration for creating namespaces.
type Config struct {
	User   bool
	Mount  bool
	PID    bool
	Net    bool
	UTS    bool
	IPC    bool
	Cgroup bool

	// User namespace configuration.
	UIDMappings []IDMap
	GIDMappings []IDMap

	// UTS configuration.
	Hostname   string
	Domainname string

	// Rootless mode.
	Rootless bool
}

// IDMap represents a UID/GID mapping.
type IDMap struct {
	ContainerID uint32
	HostID      uint32
	Size        uint32
}

// NamespacePaths holds paths to namespace files.
type NamespacePaths struct {
	User   string
	Mount  string
	PID    string
	Net    string
	UTS    string
	IPC    string
	Cgroup string
}

// CreateNamespaces creates namespaces for a container.
func (m *Manager) CreateNamespaces(containerID string, cfg *Config) (*NamespacePaths, error) {
	nsDir := filepath.Join(m.root, "namespaces", containerID)
	if err := common.EnsureDir(nsDir); err != nil {
		return nil, fmt.Errorf("failed to create namespace dir: %w", err)
	}

	paths := &NamespacePaths{}

	if cfg.User {
		paths.User = filepath.Join(nsDir, "user")
		if err := createNamespace(paths.User, UserNS); err != nil {
			return nil, fmt.Errorf("user namespace: %w", err)
		}
	}

	if cfg.Mount {
		paths.Mount = filepath.Join(nsDir, "mount")
		if err := createNamespace(paths.Mount, MountNS); err != nil {
			return nil, fmt.Errorf("mount namespace: %w", err)
		}
	}

	if cfg.PID {
		paths.PID = filepath.Join(nsDir, "pid")
		if err := createNamespace(paths.PID, PIDNS); err != nil {
			return nil, fmt.Errorf("pid namespace: %w", err)
		}
	}

	if cfg.Net {
		paths.Net = filepath.Join(nsDir, "net")
		if err := createNamespace(paths.Net, NetNS); err != nil {
			return nil, fmt.Errorf("net namespace: %w", err)
		}
	}

	if cfg.UTS {
		paths.UTS = filepath.Join(nsDir, "uts")
		if err := createNamespace(paths.UTS, UTSNS); err != nil {
			return nil, fmt.Errorf("uts namespace: %w", err)
		}
	}

	if cfg.IPC {
		paths.IPC = filepath.Join(nsDir, "ipc")
		if err := createNamespace(paths.IPC, IPCNS); err != nil {
			return nil, fmt.Errorf("ipc namespace: %w", err)
		}
	}

	if cfg.Cgroup {
		paths.Cgroup = filepath.Join(nsDir, "cgroup")
		if err := createNamespace(paths.Cgroup, CgroupNS); err != nil {
			return nil, fmt.Errorf("cgroup namespace: %w", err)
		}
	}

	return paths, nil
}

func createNamespace(path string, nsType Type) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	f.Close()

	cmd := exec.Command("unshare", fmt.Sprintf("--%s", nsType), "true")
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()

	return nil
}

// SetupUserNamespace sets up the user namespace with UID/GID mappings.
func (m *Manager) SetupUserNamespace(pid int, cfg *Config) error {
	if !cfg.Rootless && cfg.User {
		return writeIDMappings(pid, cfg.UIDMappings, cfg.GIDMappings)
	}

	if cfg.Rootless {
		uid := os.Getuid()
		gid := os.Getgid()

		uidMaps := []IDMap{
			{ContainerID: 0, HostID: uint32(uid), Size: 1},
			{ContainerID: 1, HostID: 1, Size: 1},
		}
		gidMaps := []IDMap{
			{ContainerID: 0, HostID: uint32(gid), Size: 1},
			{ContainerID: 1, HostID: 1, Size: 1},
		}

		if len(cfg.UIDMappings) > 0 {
			uidMaps = cfg.UIDMappings
		}
		if len(cfg.GIDMappings) > 0 {
			gidMaps = cfg.GIDMappings
		}

		return writeIDMappings(pid, uidMaps, gidMaps)
	}

	return nil
}

func writeIDMappings(pid int, uidMaps, gidMaps []IDMap) error {
	uidMapPath := fmt.Sprintf("/proc/%d/uid_map", pid)
	gidMapPath := fmt.Sprintf("/proc/%d/gid_map", pid)

	uidData := formatIDMaps(uidMaps)
	if err := os.WriteFile(uidMapPath, []byte(uidData), 0644); err != nil {
		return fmt.Errorf("write uid_map: %w", err)
	}

	setgroupsPath := fmt.Sprintf("/proc/%d/setgroups", pid)
	os.WriteFile(setgroupsPath, []byte("deny"), 0644)

	gidData := formatIDMaps(gidMaps)
	if err := os.WriteFile(gidMapPath, []byte(gidData), 0644); err != nil {
		return fmt.Errorf("write gid_map: %w", err)
	}

	return nil
}

func formatIDMaps(maps []IDMap) string {
	var sb strings.Builder
	for _, m := range maps {
		sb.WriteString(fmt.Sprintf("%d %d %d\n", m.ContainerID, m.HostID, m.Size))
	}
	return sb.String()
}

// JoinNamespace joins an existing namespace.
func JoinNamespace(nsType Type, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open namespace %s: %w", path, err)
	}
	defer f.Close()

	flag, ok := cloneFlags[nsType]
	if !ok {
		return fmt.Errorf("unknown namespace type: %s", nsType)
	}

	_, _, errno := syscall.RawSyscall(syscall.SYS_SETNS, f.Fd(), uintptr(flag), 0)
	if errno != 0 {
		return fmt.Errorf("setns %s: %w", nsType, errno)
	}

	return nil
}

// Supported checks if a namespace type is supported on this kernel.
func Supported(nsType Type) bool {
	path := fmt.Sprintf("/proc/self/ns/%s", nsType)
	_, err := os.Stat(path)
	return err == nil
}

// GetCurrentNamespace gets the current namespace inode for a given type.
func GetCurrentNamespace(nsType Type) (string, error) {
	path := fmt.Sprintf("/proc/self/ns/%s", nsType)
	link, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	return link, nil
}

// SetHostname sets the hostname for a UTS namespace.
func SetHostname(hostname string) error {
	return syscall.Sethostname([]byte(hostname))
}

// SetDomainname sets the domainname for a UTS namespace.
func SetDomainname(domainname string) error {
	return syscall.Setdomainname([]byte(domainname))
}

// NamespacePath returns the path to a namespace file for a given PID.
func NamespacePath(pid int, nsType Type) string {
	return fmt.Sprintf("/proc/%d/ns/%s", pid, nsType)
}

// CreatePersistentNamespace creates a persistent bind-mount of a namespace.
func CreatePersistentNamespace(targetPath string, pid int, nsType Type) error {
	nsPath := NamespacePath(pid, nsType)

	os.Remove(targetPath)

	f, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	f.Close()

	cmd := exec.Command("mount", "--bind", nsPath, targetPath)
	return cmd.Run()
}

// DeletePersistentNamespace removes a persistent namespace.
func (m *Manager) DeletePersistentNamespace(containerID string) error {
	nsDir := filepath.Join(m.root, "namespaces", containerID)

	entries, err := os.ReadDir(nsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		path := filepath.Join(nsDir, entry.Name())
		exec.Command("umount", path).Run()
		os.Remove(path)
	}

	os.Remove(nsDir)
	return nil
}

// DefaultRootlessMaps returns default UID/GID mappings for rootless mode.
func DefaultRootlessMaps() ([]IDMap, []IDMap) {
	uid := uint32(os.Getuid())
	gid := uint32(os.Getgid())

	return []IDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
			{ContainerID: 1, HostID: 1, Size: 65536},
		}, []IDMap{
			{ContainerID: 0, HostID: gid, Size: 1},
			{ContainerID: 1, HostID: 1, Size: 65536},
		}
}

// ParseCapability validates and normalizes a Linux capability name.
func ParseCapability(cap string) string {
	switch cap {
	case "ALL":
		return "ALL"
	default:
		return strings.ToUpper(cap)
	}
}

// TargetPath builds a path in the namespace directory.
func (m *Manager) TargetPath(containerID string, nsType Type) string {
	return filepath.Join(m.root, "namespaces", containerID, string(nsType))
}

// GetPIDForContainer retrieves the init PID for a container.
func (m *Manager) GetPIDForContainer(containerID string) (int, error) {
	pidFile := filepath.Join(m.root, "containers", containerID, "init.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, fmt.Errorf("read pid file for %s: %w", containerID, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse pid for %s: %w", containerID, err)
	}
	return pid, nil
}

// IsRootless checks if the current process is running as non-root.
func IsRootless() bool {
	return os.Geteuid() != 0
}
