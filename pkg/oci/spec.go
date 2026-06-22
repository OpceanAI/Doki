// Package oci implements the OCI Runtime Specification v1.2.0.
//
// It provides types for the OCI config.json bundle format, validation,
// and generation from Doki's internal container configuration.
//
// Reference: https://github.com/opencontainers/runtime-spec
package oci

import "encoding/json"

// Spec is the OCI Runtime Specification root object.
// It describes how to run a container filesystem bundle.
type Spec struct {
	// Version is the OCI runtime spec version.
	Version string `json:"ociVersion"`

	// Process configures the container process.
	Process *Process `json:"process,omitempty"`

	// Root configures the container's root filesystem.
	Root *Root `json:"root,omitempty"`

	// Hostname is the container's hostname.
	Hostname string `json:"hostname,omitempty"`

	// Mounts configures additional mounts.
	Mounts []Mount `json:"mounts,omitempty"`

	// Hooks configures lifecycle hooks.
	Hooks *Hooks `json:"hooks,omitempty"`

	// Annotations contains arbitrary metadata.
	Annotations map[string]string `json:"annotations,omitempty"`

	// Linux is platform-specific configuration for Linux-based containers.
	Linux *Linux `json:"linux,omitempty"`
}

// Process configures the container process.
type Process struct {
	// Terminal specifies whether to allocate a pseudo-TTY.
	Terminal bool `json:"terminal,omitempty"`

	// User specifies the user information for the process.
	User User `json:"user"`

	// Args is the command to run. The first element is the binary path.
	Args []string `json:"args"`

	// Entrypoint is the entrypoint of the container (from image).
	Entrypoint []string `json:"entrypoint,omitempty"`

	// Cmd is the default arguments (from image).
	Cmd []string `json:"cmd,omitempty"`

	// Env sets environment variables for the process.
	Env []string `json:"env,omitempty"`

	// Cwd is the working directory for the process.
	Cwd string `json:"cwd"`

	// Capabilities configures Linux capabilities.
	Capabilities *LinuxCapabilities `json:"capabilities,omitempty"`

	// Rlimits configures POSIX rlimits.
	Rlimits []POSIXRlimit `json:"rlimits,omitempty"`

	// NoNewPrivileges prevents the process from gaining privileges.
	NoNewPrivileges bool `json:"noNewPrivileges,omitempty"`

	// ApparmorProfile is the AppArmor profile to apply.
	ApparmorProfile string `json:"apparmorProfile,omitempty"`

	// OOMScoreAdj adjusts the OOM killer score.
	OOMScoreAdj *int `json:"oomScoreAdj,omitempty"`

	// SelinuxLabel is the SELinux process label.
	SelinuxLabel string `json:"selinuxLabel,omitempty"`
}

// User specifies user information for the process.
type User struct {
	// UID is the user ID.
	UID uint32 `json:"uid"`

	// GID is the group ID.
	GID uint32 `json:"gid"`

	// Umask is the file mode creation mask.
	Umask *uint32 `json:"umask,omitempty"`

	// AdditionalGids are additional group IDs.
	AdditionalGids []uint32 `json:"additionalGids,omitempty"`
}

// Root configures the container's root filesystem.
type Root struct {
	// Path is the path to the root filesystem.
	Path string `json:"path"`

	// Readonly makes the root filesystem read-only.
	Readonly bool `json:"readonly,omitempty"`
}

// Mount configures a mount point.
type Mount struct {
	// Destination is the mount path inside the container.
	Destination string `json:"destination"`

	// Type is the filesystem type (e.g., "proc", "sysfs", "tmpfs", "bind").
	Type string `json:"type,omitempty"`

	// Source is the source path for bind mounts.
	Source string `json:"source,omitempty"`

	// Options are mount options.
	Options []string `json:"options,omitempty"`
}

// Hooks configures lifecycle hooks.
type Hooks struct {
	// Prestart is a list of hooks to run before the container process starts.
	Prestart []Hook `json:"prestart,omitempty"`

	// CreateRuntime is a list of hooks to run after the container is created.
	CreateRuntime []Hook `json:"createRuntime,omitempty"`

	// CreateContainer is a list of hooks to run during container creation.
	CreateContainer []Hook `json:"createContainer,omitempty"`

	// StartContainer is a list of hooks to run when the container starts.
	StartContainer []Hook `json:"startContainer,omitempty"`

	// Poststart is a list of hooks to run after the container process starts.
	Poststart []Hook `json:"poststart,omitempty"`

	// Poststop is a list of hooks to run after the container process stops.
	Poststop []Hook `json:"poststop,omitempty"`
}

// Hook represents a lifecycle hook.
type Hook struct {
	Path    string   `json:"path"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	Timeout *int     `json:"timeout,omitempty"`
}

// Linux contains platform-specific configuration for Linux.
type Linux struct {
	// Namespaces configures Linux namespaces.
	Namespaces []LinuxNamespace `json:"namespaces,omitempty"`

	// UIDMappings configures user namespace UID mappings.
	UIDMappings []LinuxIDMapping `json:"uidMappings,omitempty"`

	// GIDMappings configures user namespace GID mappings.
	GIDMappings []LinuxIDMapping `json:"gidMappings,omitempty"`

	// Devices configures device nodes in the container.
	Devices []LinuxDevice `json:"devices,omitempty"`

	// CgroupsPath is the path to the cgroup.
	CgroupsPath string `json:"cgroupsPath,omitempty"`

	// Resources configures cgroup resources.
	Resources *LinuxResources `json:"resources,omitempty"`

	// Seccomp configures the seccomp profile.
	Seccomp *LinuxSeccomp `json:"seccomp,omitempty"`

	// RootfsPropagation sets the rootfs propagation mode.
	RootfsPropagation string `json:"rootfsPropagation,omitempty"`

	// MaskedPaths masks paths inside the container.
	MaskedPaths []string `json:"maskedPaths,omitempty"`

	// ReadonlyPaths makes paths read-only inside the container.
	ReadonlyPaths []string `json:"readonlyPaths,omitempty"`

	// MountLabel is the SELinux mount label.
	MountLabel string `json:"mountLabel,omitempty"`

	// IntelRdt configures Intel RDT/CAT.
	IntelRdt *LinuxIntelRdt `json:"intelRdt,omitempty"`

	// Personality configures the execution domain.
	Personality *LinuxPersonality `json:"personality,omitempty"`

	// TimeOffsets configures time namespace offsets.
	TimeOffsets map[string]LinuxTimeOffset `json:"timeOffsets,omitempty"`

	// Sysctl configures kernel parameters.
	Sysctl map[string]string `json:"sysctl,omitempty"`

	// Unified configures unified cgroup v2 key-value pairs.
	Unified map[string]string `json:"unified,omitempty"`
}

// LinuxNamespace represents a Linux namespace.
type LinuxNamespace struct {
	// Type is the namespace type.
	Type LinuxNamespaceType `json:"type"`

	// Path is the path to join an existing namespace.
	Path string `json:"path,omitempty"`
}

// LinuxNamespaceType is the type of a Linux namespace.
type LinuxNamespaceType string

const (
	NamespacePID     LinuxNamespaceType = "pid"
	NamespaceNetwork LinuxNamespaceType = "network"
	NamespaceMount   LinuxNamespaceType = "mount"
	NamespaceIPC     LinuxNamespaceType = "ipc"
	NamespaceUTS     LinuxNamespaceType = "uts"
	NamespaceUser    LinuxNamespaceType = "user"
	NamespaceCgroup  LinuxNamespaceType = "cgroup"
	NamespaceTime    LinuxNamespaceType = "time"
)

// LinuxIDMapping maps UIDs/GIDs between host and container.
type LinuxIDMapping struct {
	ContainerID uint32 `json:"containerID"`
	HostID      uint32 `json:"hostID"`
	Size        uint32 `json:"size"`
}

// LinuxDevice represents a device node.
type LinuxDevice struct {
	// Path is the device path inside the container.
	Path string `json:"path"`

	// Type is the device type (c, b, u, p).
	Type string `json:"type,omitempty"`

	// Major is the device major number.
	Major int64 `json:"major"`

	// Minor is the device minor number.
	Minor int64 `json:"minor"`

	// FileMode is the file mode of the device (e.g., 0666).
	FileMode *uint32 `json:"fileMode,omitempty"`

	// UID is the device owner UID.
	UID *uint32 `json:"uid,omitempty"`

	// GID is the device owner GID.
	GID *uint32 `json:"gid,omitempty"`
}

// LinuxDeviceCgroup represents a device cgroup rule.
type LinuxDeviceCgroup struct {
	Allow  bool   `json:"allow"`
	Type   string `json:"type,omitempty"`
	Major  *int64 `json:"major,omitempty"`
	Minor  *int64 `json:"minor,omitempty"`
	Access string `json:"access,omitempty"`
}

// LinuxResources configures cgroup resources.
type LinuxResources struct {
	Devices        []LinuxDeviceCgroup  `json:"devices,omitempty"`
	Memory         *LinuxMemory         `json:"memory,omitempty"`
	CPU            *LinuxCPU            `json:"cpu,omitempty"`
	Pids           *LinuxPids           `json:"pids,omitempty"`
	BlockIO        *LinuxBlockIO        `json:"blockIO,omitempty"`
	HugepageLimits []LinuxHugepageLimit `json:"hugepageLimits,omitempty"`
	Network        *LinuxNetwork        `json:"network,omitempty"`
	Rdma           map[string]LinuxRdma `json:"rdma,omitempty"`
}

// LinuxMemory configures memory cgroup limits.
type LinuxMemory struct {
	Limit            *int64  `json:"limit,omitempty"`
	Reservation      *int64  `json:"reservation,omitempty"`
	Swap             *int64  `json:"swap,omitempty"`
	Kernel           *int64  `json:"kernel,omitempty"`
	KernelTCP        *int64  `json:"kernelTCP,omitempty"`
	Swappiness       *uint64 `json:"swappiness,omitempty"`
	DisableOOMKiller *bool   `json:"disableOOMKiller,omitempty"`
	UseHierarchy     *bool   `json:"useHierarchy,omitempty"`
}

// LinuxCPU configures CPU cgroup limits.
type LinuxCPU struct {
	Shares          *uint64 `json:"shares,omitempty"`
	Quota           *int64  `json:"quota,omitempty"`
	Period          *uint64 `json:"period,omitempty"`
	RealtimeRuntime *int64  `json:"realtimeRuntime,omitempty"`
	RealtimePeriod  *uint64 `json:"realtimePeriod,omitempty"`
	Cpus            string  `json:"cpus,omitempty"`
	Mems            string  `json:"mems,omitempty"`
}

// LinuxPids configures PID cgroup limits.
type LinuxPids struct {
	Limit int64 `json:"limit"`
}

// LinuxBlockIO configures block I/O cgroup limits.
type LinuxBlockIO struct {
	Weight            *uint16               `json:"weight,omitempty"`
	LeafWeight        *uint16               `json:"leafWeight,omitempty"`
	WeightDevice      []LinuxWeightDevice   `json:"weightDevice,omitempty"`
	ThrottleRead      []LinuxThrottleDevice `json:"throttleReadBpsDevice,omitempty"`
	ThrottleWrite     []LinuxThrottleDevice `json:"throttleWriteBpsDevice,omitempty"`
	ThrottleReadIOPS  []LinuxThrottleDevice `json:"throttleReadIOPSDevice,omitempty"`
	ThrottleWriteIOPS []LinuxThrottleDevice `json:"throttleWriteIOPSDevice,omitempty"`
}

// LinuxWeightDevice configures per-device block I/O weight.
type LinuxWeightDevice struct {
	Major      int64   `json:"major"`
	Minor      int64   `json:"minor"`
	Weight     *uint16 `json:"weight,omitempty"`
	LeafWeight *uint16 `json:"leafWeight,omitempty"`
}

// LinuxThrottleDevice configures per-device block I/O throttle.
type LinuxThrottleDevice struct {
	Major int64  `json:"major"`
	Minor int64  `json:"minor"`
	Rate  uint64 `json:"rate"`
}

// LinuxHugepageLimit configures hugepage limits.
type LinuxHugepageLimit struct {
	Pagesize string `json:"pagesize"`
	Limit    int64  `json:"limit"`
}

// LinuxNetwork configures network cgroup limits.
type LinuxNetwork struct {
	ClassID    *uint32                  `json:"classID,omitempty"`
	Priorities []LinuxInterfacePriority `json:"priorities,omitempty"`
}

// LinuxInterfacePriority configures network interface priority.
type LinuxInterfacePriority struct {
	Name     string `json:"name"`
	Priority uint32 `json:"priority"`
}

// LinuxRdma configures RDMA resources.
type LinuxRdma struct {
	HcaHandles *uint32 `json:"hcaHandles,omitempty"`
	HcaObjects *uint32 `json:"hcaObjects,omitempty"`
}

// LinuxSeccomp configures the seccomp profile.
type LinuxSeccomp struct {
	DefaultAction LinuxSeccompAction `json:"defaultAction"`
	Architectures []Arch             `json:"architectures,omitempty"`
	Syscalls      []LinuxSyscall     `json:"syscalls,omitempty"`
	Flags         []string           `json:"flags,omitempty"`
}

// LinuxSeccompAction is a seccomp action.
type LinuxSeccompAction string

const (
	SeccompActKill        LinuxSeccompAction = "SCMP_ACT_KILL"
	SeccompActKillProcess LinuxSeccompAction = "SCMP_ACT_KILL_PROCESS"
	SeccompActTrap        LinuxSeccompAction = "SCMP_ACT_TRAP"
	SeccompActErrno       LinuxSeccompAction = "SCMP_ACT_ERRNO"
	SeccompActTrace       LinuxSeccompAction = "SCMP_ACT_TRACE"
	SeccompActAllow       LinuxSeccompAction = "SCMP_ACT_ALLOW"
	SeccompActLog         LinuxSeccompAction = "SCMP_ACT_LOG"
)

// Arch is a seccomp architecture.
type Arch string

const (
	ArchX86         Arch = "SCMP_ARCH_X86"
	ArchX86_64      Arch = "SCMP_ARCH_X86_64"
	ArchX32         Arch = "SCMP_ARCH_X32"
	ArchARM         Arch = "SCMP_ARCH_ARM"
	ArchAARCH64     Arch = "SCMP_ARCH_AARCH64"
	ArchMIPS        Arch = "SCMP_ARCH_MIPS"
	ArchMIPS64      Arch = "SCMP_ARCH_MIPS64"
	ArchMIPS64N32   Arch = "SCMP_ARCH_MIPS64N32"
	ArchMIPSEL      Arch = "SCMP_ARCH_MIPSEL"
	ArchMIPSEL64    Arch = "SCMP_ARCH_MIPSEL64"
	ArchMIPSEL64N32 Arch = "SCMP_ARCH_MIPSEL64N32"
	ArchPPC         Arch = "SCMP_ARCH_PPC"
	ArchPPC64       Arch = "SCMP_ARCH_PPC64"
	ArchPPC64LE     Arch = "SCMP_ARCH_PPC64LE"
	ArchS390        Arch = "SCMP_ARCH_S390"
	ArchS390X       Arch = "SCMP_ARCH_S390X"
	ArchPARISC      Arch = "SCMP_ARCH_PARISC"
	ArchPARISC64    Arch = "SCMP_ARCH_PARISC64"
	ArchRISCV64     Arch = "SCMP_ARCH_RISCV64"
)

// LinuxSyscall is a seccomp syscall rule.
type LinuxSyscall struct {
	Names   []string           `json:"names"`
	Action  LinuxSeccompAction `json:"action"`
	Args    []LinuxSeccompArg  `json:"args,omitempty"`
	Comment string             `json:"comment,omitempty"`
}

// LinuxSeccompArg is a seccomp syscall argument filter.
type LinuxSeccompArg struct {
	Index    uint           `json:"index"`
	Value    uint64         `json:"value"`
	ValueTwo uint64         `json:"valueTwo,omitempty"`
	Op       LinuxSeccompOp `json:"op"`
}

// LinuxSeccompOp is a seccomp comparison operator.
type LinuxSeccompOp string

const (
	SeccompOpNotEqual     LinuxSeccompOp = "SCMP_CMP_NE"
	SeccompOpLessThan     LinuxSeccompOp = "SCMP_CMP_LT"
	SeccompOpLessEqual    LinuxSeccompOp = "SCMP_CMP_LE"
	SeccompOpEqualTo      LinuxSeccompOp = "SCMP_CMP_EQ"
	SeccompOpGreaterEqual LinuxSeccompOp = "SCMP_CMP_GE"
	SeccompOpGreaterThan  LinuxSeccompOp = "SCMP_CMP_GT"
	SeccompOpMaskedEqual  LinuxSeccompOp = "SCMP_CMP_MASKED_EQ"
)

// LinuxIntelRdt configures Intel RDT/CAT.
type LinuxIntelRdt struct {
	ClosID        string `json:"closID,omitempty"`
	L3CacheSchema string `json:"l3CacheSchema,omitempty"`
	MemBwSchema   string `json:"memBwSchema,omitempty"`
}

// LinuxPersonality configures the execution domain.
type LinuxPersonality struct {
	Domain string   `json:"domain"`
	Flags  []string `json:"flags,omitempty"`
}

// LinuxTimeOffset configures time namespace offsets.
type LinuxTimeOffset struct {
	Secs     int64  `json:"secs"`
	Nanosecs uint32 `json:"nanosecs,omitempty"`
}

// POSIXRlimit represents a POSIX resource limit.
type POSIXRlimit struct {
	Type string `json:"type"`
	Hard uint64 `json:"hard"`
	Soft uint64 `json:"soft"`
}

// LinuxCapabilities configures process capabilities.
type LinuxCapabilities struct {
	Bounding    []string `json:"bounding,omitempty"`
	Effective   []string `json:"effective,omitempty"`
	Inheritable []string `json:"inheritable,omitempty"`
	Permitted   []string `json:"permitted,omitempty"`
	Ambient     []string `json:"ambient,omitempty"`
}

// MarshalSpec marshals the Spec to JSON with pretty printing.
func MarshalSpec(s *Spec) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}
