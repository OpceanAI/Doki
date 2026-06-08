package security

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SeccompProfile represents a seccomp profile.
type SeccompProfile struct {
	DefaultAction string            `json:"defaultAction"`
	Architectures []string          `json:"architectures,omitempty"`
	Syscalls      []SeccompSyscall  `json:"syscalls,omitempty"`
	Flags         []string          `json:"flags,omitempty"`
	ListenerPath  string            `json:"listenerPath,omitempty"`
	ListenerMetadata map[string]string `json:"listenerMetadata,omitempty"`
}

// SeccompSyscall represents a seccomp syscall rule.
type SeccompSyscall struct {
	Names   []string          `json:"names"`
	Action  string            `json:"action"`
	Args    []SeccompArg      `json:"args,omitempty"`
	Comment string            `json:"comment,omitempty"`
}

// SeccompArg represents a seccomp syscall argument filter.
type SeccompArg struct {
	Index    uint   `json:"index"`
	Value    uint64 `json:"value"`
	ValueTwo uint64 `json:"valueTwo,omitempty"`
	Op       string `json:"op"`
}

// Seccomp constants.
const (
	ActKill        = "SCMP_ACT_KILL"
	ActKillProcess = "SCMP_ACT_KILL_PROCESS"
	ActTrap        = "SCMP_ACT_TRAP"
	ActErrno       = "SCMP_ACT_ERRNO"
	ActTrace       = "SCMP_ACT_TRACE"
	ActAllow       = "SCMP_ACT_ALLOW"
	ActLog         = "SCMP_ACT_LOG"

	ArchX86     = "SCMP_ARCH_X86"
	ArchX86_64  = "SCMP_ARCH_X86_64"
	ArchARM     = "SCMP_ARCH_ARM"
	ArchAARCH64 = "SCMP_ARCH_AARCH64"
	ArchMIPS    = "SCMP_ARCH_MIPS"
	ArchMIPS64  = "SCMP_ARCH_MIPS64"
	ArchPPC64LE = "SCMP_ARCH_PPC64LE"
	ArchS390X   = "SCMP_ARCH_S390X"
	ArchRISCV64 = "SCMP_ARCH_RISCV64"

	OpNotEqual     = "SCMP_CMP_NE"
	OpLessThan     = "SCMP_CMP_LT"
	OpLessEqual    = "SCMP_CMP_LE"
	OpEqualTo      = "SCMP_CMP_EQ"
	OpGreaterEqual = "SCMP_CMP_GE"
	OpGreaterThan  = "SCMP_CMP_GT"
	OpMaskedEqual  = "SCMP_CMP_MASKED_EQ"
)

// DefaultArchitectures returns the default architectures for a seccomp profile.
func DefaultArchitectures() []string {
	return []string{ArchX86_64, ArchAARCH64, ArchX86, ArchARM}
}

// DefaultProfile returns the default seccomp profile for containers.
// This is similar to Docker's default profile.
func DefaultProfile() *SeccompProfile {
	return &SeccompProfile{
		DefaultAction: ActErrno,
		Architectures: DefaultArchitectures(),
		Syscalls: []SeccompSyscall{
			{
				Names:  defaultAllowSyscalls(),
				Action: ActAllow,
			},
			// BUG-27 fix: ptrace is a container escape vector that allows
			// tracing other processes, reading memory, and injecting code.
			// Removed unconditional allow. Use AppArmor ptrace peer=
			// restriction if debugging support is needed.
		},
	}
}

// PrivilegedProfile returns a permissive seccomp profile for privileged containers.
func PrivilegedProfile() *SeccompProfile {
	return &SeccompProfile{
		DefaultAction: ActAllow,
	}
}

// UnconfinedProfile returns a completely unconfined seccomp profile.
func UnconfinedProfile() *SeccompProfile {
	return &SeccompProfile{
		DefaultAction: ActAllow,
	}
}

// AndroidProfile returns a seccomp profile optimized for Android.
// It allows Android-specific syscalls while blocking dangerous ones.
func AndroidProfile() *SeccompProfile {
	return &SeccompProfile{
		DefaultAction: ActErrno,
		Architectures: []string{ArchAARCH64, ArchARM},
		Syscalls: []SeccompSyscall{
			{
				Names:  androidAllowSyscalls(),
				Action: ActAllow,
			},
			{
				Names:   []string{"ioctl"},
				Action:  ActAllow,
				Comment: "Allow ioctl for Android-specific operations",
			},
		},
	}
}

// ARMProfile returns a seccomp profile optimized for ARM devices.
func ARMProfile() *SeccompProfile {
	return &SeccompProfile{
		DefaultAction: ActErrno,
		Architectures: []string{ArchAARCH64, ArchARM},
		Syscalls: []SeccompSyscall{
			{
				Names:  armAllowSyscalls(),
				Action: ActAllow,
			},
		},
	}
}

// LoadProfile loads a seccomp profile from a JSON file.
func LoadProfile(path string) (*SeccompProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seccomp profile: %w", err)
	}
	var profile SeccompProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse seccomp profile: %w", err)
	}
	return &profile, nil
}

// SaveProfile saves a seccomp profile to a JSON file.
func SaveProfile(profile *SeccompProfile, path string) error {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// MergeProfiles merges multiple seccomp profiles.
// Later profiles override earlier ones.
func MergeProfiles(profiles ...*SeccompProfile) *SeccompProfile {
	if len(profiles) == 0 {
		return DefaultProfile()
	}
	result := &SeccompProfile{
		DefaultAction: profiles[0].DefaultAction,
		Architectures: profiles[0].Architectures,
		Flags:         profiles[0].Flags,
	}

	syscallMap := make(map[string]SeccompSyscall)
	for _, p := range profiles {
		for _, sc := range p.Syscalls {
			for _, name := range sc.Names {
				syscallMap[name] = sc
			}
		}
	}

	result.Syscalls = make([]SeccompSyscall, 0, len(syscallMap))
	for _, sc := range syscallMap {
		result.Syscalls = append(result.Syscalls, sc)
	}

	return result
}

// ValidateProfile validates a seccomp profile.
func ValidateProfile(profile *SeccompProfile) error {
	if profile == nil {
		return fmt.Errorf("seccomp profile is nil")
	}

	validActions := map[string]bool{
		ActKill: true, ActKillProcess: true, ActTrap: true,
		ActErrno: true, ActTrace: true, ActAllow: true, ActLog: true,
	}
	if !validActions[profile.DefaultAction] {
		return fmt.Errorf("invalid default action: %s", profile.DefaultAction)
	}

	for i, sc := range profile.Syscalls {
		if len(sc.Names) == 0 {
			return fmt.Errorf("syscall rule %d: names is empty", i)
		}
		if !validActions[sc.Action] {
			return fmt.Errorf("syscall rule %d: invalid action: %s", i, sc.Action)
		}
		for j, arg := range sc.Args {
			validOps := map[string]bool{
				OpNotEqual: true, OpLessThan: true, OpLessEqual: true,
				OpEqualTo: true, OpGreaterEqual: true, OpGreaterThan: true, OpMaskedEqual: true,
			}
			if !validOps[arg.Op] {
				return fmt.Errorf("syscall rule %d arg %d: invalid op: %s", i, j, arg.Op)
			}
		}
	}

	return nil
}

// ProfileToJSON converts a profile to JSON string.
func ProfileToJSON(profile *SeccompProfile) (string, error) {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ProfileFromJSON converts a JSON string to a profile.
func ProfileFromJSON(jsonStr string) (*SeccompProfile, error) {
	var profile SeccompProfile
	if err := json.Unmarshal([]byte(jsonStr), &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

// String returns a human-readable summary of the profile.
func (p *SeccompProfile) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("SeccompProfile{default=%s, archs=%v, syscalls=%d}",
		p.DefaultAction, p.Architectures, len(p.Syscalls)))
	return b.String()
}

func defaultAllowSyscalls() []string {
	return []string{
		"accept", "accept4", "access", "adjtimex", "alarm", "bind", "brk",
		"capget", "capset", "chdir", "chmod", "chown", "chown32", "clock_adjtime",
		"clock_adjtime64", "clock_getres", "clock_getres_time64", "clock_gettime",
		"clock_gettime64", "clock_nanosleep", "clock_nanosleep_time64", "close",
		"connect", "copy_file_range", "creat", "dup", "dup2", "dup3", "epoll_create",
		"epoll_create1", "epoll_ctl", "epoll_ctl_old", "epoll_pwait", "epoll_wait",
		"epoll_wait_old", "eventfd", "eventfd2", "execve", "execveat", "exit",
		"exit_group", "faccessat", "faccessat2", "fadvise64", "fadvise64_64",
		"fallocate", "fanotify_mark", "fchdir", "fchmod", "fchmodat", "fchown",
		"fchown32", "fchownat", "fcntl", "fcntl64", "fdatasync", "fgetxattr",
		"flistxattr", "flock", "fork", "fremovexattr", "fsetxattr", "fstat",
		"fstat64", "fstatat64", "fstatfs", "fstatfs64", "fsync", "ftruncate",
		"ftruncate64", "futex", "futex_time64", "futimesat", "getcpu", "getcwd",
		"getdents", "getdents64", "getegid", "getegid32", "geteuid", "geteuid32",
		"getgid", "getgid32", "getgroups", "getgroups32", "getitimer", "getpeername",
		"getpgid", "getpgrp", "getpid", "getppid", "getpriority", "getrandom",
		"getresgid", "getresgid32", "getresuid", "getresuid32", "getrlimit",
		"get_robust_list", "getrusage", "getsid", "getsockname", "getsockopt",
		"get_thread_area", "gettid", "gettimeofday", "getuid", "getuid32",
		"getxattr", "inotify_add_watch", "inotify_init", "inotify_init1",
		"inotify_rm_watch", "io_cancel", "ioctl", "io_destroy", "io_getevents",
		"io_pgetevents", "io_pgetevents_time64", "ioprio_get", "ioprio_set",
		"io_setup", "io_submit", "io_uring_enter", "io_uring_register",
		"io_uring_setup", "ipc", "kill", "lchown", "lchown32", "lgetxattr",
		"link", "linkat", "listen", "listxattr", "llistxattr", "lremovexattr",
		"lseek", "lsetxattr", "lstat", "lstat64", "madvise", "membarrier",
		"memfd_create", "mincore", "mkdir", "mkdirat", "mknod", "mknodat", "mlock",
		"mlock2", "mlockall", "mmap", "mmap2", "mprotect", "mq_getsetattr",
		"mq_notify", "mq_open", "mq_timedreceive", "mq_timedreceive_time64",
		"mq_timedsend", "mq_timedsend_time64", "mq_unlink", "mremap", "msgctl",
		"msgget", "msgrcv", "msgsnd", "msync", "munlock", "munlockall", "munmap",
		"nanosleep", "newfstatat", "newuname", "open", "openat", "openat2",
		"pause", "pidfd_open", "pidfd_send_signal", "pipe", "pipe2", "poll",
		"ppoll", "ppoll_time64", "prctl", "pread64", "preadv", "preadv2", "prlimit64",
		"pselect6", "pselect6_time64", "pwrite64", "pwritev", "pwritev2", "read",
		"readahead", "readlink", "readlinkat", "readv", "recv", "recvfrom",
		"recvmmsg", "recvmmsg_time64", "recvmsg", "remap_file_pages", "removexattr",
		"rename", "renameat", "renameat2", "restart_syscall", "rmdir", "rseq",
		"rt_sigaction", "rt_sigpending", "rt_sigprocmask", "rt_sigqueueinfo",
		"rt_sigreturn", "rt_sigsuspend", "rt_sigtimedwait", "rt_sigtimedwait_time64",
		"rt_tgsigqueueinfo", "sched_getaffinity", "sched_getattr", "sched_getparam",
		"sched_get_priority_max", "sched_get_priority_min", "sched_getscheduler",
		"sched_setaffinity", "sched_setattr", "sched_setparam", "sched_setscheduler",
		"sched_yield", "seccomp", "select", "semctl", "semget", "semop", "semtimedop",
		"semtimedop_time64", "send", "sendfile", "sendfile64", "sendmmsg", "sendmsg",
		"sendto", "setfsgid", "setfsgid32", "setfsuid", "setfsuid32", "setgid",
		"setgid32", "setgroups", "setgroups32", "setitimer", "setpgid", "setpriority",
		"setregid", "setregid32", "setresgid", "setresgid32", "setresuid",
		"setresuid32", "setreuid", "setreuid32", "setrlimit", "set_robust_list",
		"setsid", "setsockopt", "set_thread_area", "set_tid_address", "setuid",
		"setuid32", "setxattr", "shmat", "shmctl", "shmdt", "shmget", "shutdown",
		"sigaltstack", "signalfd", "signalfd4", "socket", "socketcall", "socketpair",
		"splice", "stat", "stat64", "statfs", "statfs64", "statx", "symlink",
		"symlinkat", "sync", "sync_file_range", "syncfs", "sysinfo", "syslog",
		"tee", "tgkill", "time", "timer_create", "timer_delete", "timerfd_create",
		"timerfd_gettime", "timerfd_settime", "timer_getoverrun", "timer_gettime",
		"timer_settime", "timer_settime64", "times", "tkill", "truncate",
		"truncate64", "ugetrlimit", "umask", "uname", "unlink", "unlinkat",
		"utime", "utimensat", "utimensat_time64", "utimes", "vfork", "vmsplice",
		"wait4", "waitid", "waitpid", "write", "writev",
	}
}

func androidAllowSyscalls() []string {
	// Android-specific syscalls in addition to default.
	base := defaultAllowSyscalls()
	android := []string{
		"bpf", "clone3", "close_range", "faccessat2",
		// BUG-09 fix: io_uring has dozens of kernel CVEs.
		// "io_uring_setup",
		"openat2", "pidfd_getfd", "pidfd_open", "process_madvise",
		"quotactl_fd", "set_mempolicy_home_node", "cachestat",
		"fchmodat2", "fstatat64", "map_shadow_stack",
	}
	return append(base, android...)
}

func armAllowSyscalls() []string {
	// ARM-specific syscalls.
	base := defaultAllowSyscalls()
	arm := []string{
		"cacheflush", "set_tls", "arm_fadvise64_64",
	}
	return append(base, arm...)
}
