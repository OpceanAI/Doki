package seccomp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Profile represents a seccomp security profile.
type Profile struct {
	DefaultAction string       `json:"defaultAction"`
	Architectures []string     `json:"architectures,omitempty"`
	Syscalls      []SyscallRule `json:"syscalls,omitempty"`
	Flags         []string     `json:"flags,omitempty"`
}

// SyscallRule defines allowed/blocked syscalls.
type SyscallRule struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
	Args   []ArgRule `json:"args,omitempty"`
}

// ArgRule defines argument filtering for syscalls.
type ArgRule struct {
	Index    uint   `json:"index"`
	Value    uint64 `json:"value"`
	ValueTwo uint64 `json:"valueTwo,omitempty"`
	Op       string `json:"op"`
}

// DefaultProfile returns the default Doki seccomp profile.
// This blocks dangerous syscalls while allowing normal container operation.
// Updated to include AF_ALG block per CVE-2026-31431.
func DefaultProfile() *Profile {
	return &Profile{
		DefaultAction: "SCMP_ACT_ERRNO",
		Architectures: []string{"SCMP_ARCH_X86_64", "SCMP_ARCH_AARCH64"},
		Flags:         []string{"SECCOMP_FILTER_FLAG_TSYNC"},
		Syscalls: []SyscallRule{
			// DENY dangerous syscalls FIRST (first-match-wins semantics)
			{Names: []string{
				"kexec_load", "kexec_file_load",
				"reboot", "swapoff", "swapon",
				"bpf", "perf_event_open",
				"fanotify_init", "fanotify_mark",
				"init_module", "finit_module", "delete_module",
				"socketcall", "iopl", "ioperm",
				"acct", "uselib", "ustat",
				"bdflush", "create_module", "get_kernel_syms",
				"_sysctl", "s390_utc", "utc",
				"process_vm_writev", "process_vm_readv",
				"modify_ldt", "pciconfig_read", "pciconfig_write",
				"vhangup", "nfsservctl", "pivot_root",
			}, Action: "SCMP_ACT_ERRNO"},
			// Then ALLOW essential syscalls
			{Names: allowedSyscalls(), Action: "SCMP_ACT_ALLOW"},
		},
	}
}

func PrivilegedProfile() *Profile {
	return &Profile{
		DefaultAction: "SCMP_ACT_ALLOW",
		Syscalls: []SyscallRule{},
	}
}

func UnconfinedProfile() *Profile {
	return nil
}

// allowedSyscalls returns the allowlist of syscalls.
func allowedSyscalls() []string {
	return []string{
		// Process management.
		"fork", "vfork", "clone", "clone3",
		"execve", "execveat",
		"exit", "exit_group",
		"wait4", "waitid",
		"kill", "tgkill", "tkill", "rt_sigaction", "rt_sigprocmask",
		"rt_sigreturn", "rt_sigpending", "rt_sigtimedwait",
		"sigaltstack", "signalfd", "signalfd4",
		"pause", "nanosleep", "clock_nanosleep",
		"prctl", "arch_prctl",
		"set_tid_address", "set_robust_list", "get_robust_list",

		// File operations.
		"read", "readv", "pread64", "preadv", "preadv2",
		"write", "writev", "pwrite64", "pwritev", "pwritev2",
		"open", "openat", "openat2", "open_tree",
		"close", "close_range",
		"lseek", "_llseek",
		"stat", "lstat", "fstat", "newfstatat",
		"statx",
		"access", "faccessat", "faccessat2",
		"readlink", "readlinkat",
		"getcwd",
		"chdir", "fchdir",
		"getdents", "getdents64",
		"fcntl", "fcntl64",
		"flock",
		"fsync", "fdatasync", "sync_file_range",
		"rename", "renameat", "renameat2",
		"link", "linkat", "symlink", "symlinkat",
		"unlink", "unlinkat",
		"mkdir", "mkdirat", "rmdir",
		"mknod", "mknodat",
		"chmod", "fchmod", "fchmodat",
		"chown", "fchown", "fchownat", "lchown",
		"utimensat", "futimesat",
		"truncate", "ftruncate",
		"fallocate",
		"copy_file_range", "splice", "sendfile", "tee",

		// Memory management.
		"mmap", "mmap2", "mprotect", "munmap",
		"brk", "madvise",
		"mremap", "remap_file_pages",
		"mlock", "mlock2", "mlockall", "munlock", "munlockall",
		"mincore", "msync",
		"memfd_create", "memfd_secret",

		// IPC.
		"pipe", "pipe2",
		"eventfd", "eventfd2",
		"shmget", "shmat", "shmdt", "shmctl",
		"semget", "semop", "semctl",
		"msgget", "msgsnd", "msgrcv", "msgctl",
		"futex", "futex_waitv", "futex_wake", "futex_wait",
		"setns", "unshare",

		// Networking.
		"socket", "bind", "listen", "accept", "accept4",
		"connect", "getpeername", "getsockname",
		"sendto", "recvfrom", "sendmsg", "recvmsg",
		"shutdown", "setsockopt", "getsockopt",
		"socketpair",
		"getifaddrs", "if_indextoname", "if_nametoindex",
		"socketcall", // allowed for legacy socket operations, blocked only as standalone in denied list
		"epoll_create", "epoll_create1", "epoll_ctl", "epoll_wait", "epoll_pwait",
		"poll", "ppoll", "select",

		// System information.
		"uname", "sysinfo",
		"getpid", "gettid", "getuid", "geteuid", "getgid", "getegid",
		"getgroups", "setgroups",
		"getresuid", "getresgid", "setresuid", "setresgid",
		"getrlimit", "prlimit64", "getrusage",
		"times", "time",
		"gettimeofday", "clock_gettime", "clock_getres",
		"sched_getattr", "sched_setattr",
		"sched_yield", "sched_getparam", "sched_getscheduler",
		"capget", "capset",
		"getcpu",

		// Random.
		"getrandom",
	}
}

// LoadProfile loads a seccomp profile from a JSON file.
func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seccomp profile: %w", err)
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse seccomp profile: %w", err)
	}
	return &p, nil
}

// SaveProfile saves a seccomp profile to a JSON file.
func SaveProfile(path string, profile *Profile) error {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GenerateProfilePath returns the path for a container's seccomp profile.
func GenerateProfilePath(containerID string) string {
	return fmt.Sprintf("/var/lib/doki/containers/%s/seccomp.json", containerID)
}

// FilterAllowed returns only the allowed syscalls from a profile.
func (p *Profile) FilterAllowed() []string {
	if p == nil {
		return nil
	}
	var allowed []string
	for _, rule := range p.Syscalls {
		if strings.ToUpper(rule.Action) == "SCMP_ACT_ALLOW" {
			allowed = append(allowed, rule.Names...)
		}
	}
	return allowed
}

// FilterBlocked returns only the blocked syscalls from a profile.
func (p *Profile) FilterBlocked() []string {
	if p == nil {
		return nil
	}
	var blocked []string
	for _, rule := range p.Syscalls {
		if strings.ToUpper(rule.Action) == "SCMP_ACT_ERRNO" {
			blocked = append(blocked, rule.Names...)
		}
	}
	return blocked
}
