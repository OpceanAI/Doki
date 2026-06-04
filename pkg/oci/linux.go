package oci

import (
	"fmt"
	"strings"
)

// Known capabilities on Linux.
// Reference: include/uapi/linux/capability.h
var knownCapabilities = map[string]bool{
	"CAP_AUDIT_CONTROL":      true,
	"CAP_AUDIT_READ":         true,
	"CAP_AUDIT_WRITE":        true,
	"CAP_BLOCK_SUSPEND":      true,
	"CAP_CHOWN":              true,
	"CAP_DAC_OVERRIDE":       true,
	"CAP_DAC_READ_SEARCH":    true,
	"CAP_FOWNER":             true,
	"CAP_FSETID":             true,
	"CAP_IPC_LOCK":           true,
	"CAP_IPC_OWNER":          true,
	"CAP_KILL":               true,
	"CAP_LEASE":              true,
	"CAP_LINUX_IMMUTABLE":    true,
	"CAP_MAC_ADMIN":          true,
	"CAP_MAC_OVERRIDE":       true,
	"CAP_MKNOD":              true,
	"CAP_NET_ADMIN":          true,
	"CAP_NET_BIND_SERVICE":   true,
	"CAP_NET_BROADCAST":      true,
	"CAP_NET_RAW":            true,
	"CAP_SETFCAP":            true,
	"CAP_SETGID":             true,
	"CAP_SETPCAP":            true,
	"CAP_SETUID":             true,
	"CAP_SYS_ADMIN":          true,
	"CAP_SYS_BOOT":           true,
	"CAP_SYS_CHROOT":         true,
	"CAP_SYS_MODULE":         true,
	"CAP_SYS_NICE":           true,
	"CAP_SYS_PACCT":          true,
	"CAP_SYS_PTRACE":         true,
	"CAP_SYS_RAWIO":          true,
	"CAP_SYS_RESOURCE":       true,
	"CAP_SYS_TIME":           true,
	"CAP_SYS_TTY_CONFIG":     true,
	"CAP_SYSLOG":             true,
	"CAP_WAKE_ALARM":         true,
	"CAP_PERFMON":            true,
	"CAP_BPF":                true,
	"CAP_CHECKPOINT_RESTORE": true,
}

// DefaultNamespaces returns the default set of namespaces for a container.
func DefaultNamespaces(rootless bool) []LinuxNamespace {
	ns := []LinuxNamespace{
		{Type: NamespacePID},
		{Type: NamespaceMount},
		{Type: NamespaceUTS},
		{Type: NamespaceIPC},
		{Type: NamespaceNetwork},
	}
	if rootless {
		ns = append(ns, LinuxNamespace{Type: NamespaceUser})
	}
	return ns
}

// ParseCapability normalizes a capability name.
// Handles: "cap_sys_admin", "SYS_ADMIN", "CAP_SYS_ADMIN", "sys_admin".
func ParseCapability(name string) (string, error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if !strings.HasPrefix(name, "CAP_") {
		name = "CAP_" + name
	}
	if !knownCapabilities[name] {
		return "", fmt.Errorf("unknown capability: %s", name)
	}
	return name, nil
}

// ParseCapabilities normalizes a list of capability names.
func ParseCapabilities(caps []string) ([]string, error) {
	result := make([]string, 0, len(caps))
	for _, c := range caps {
		parsed, err := ParseCapability(c)
		if err != nil {
			return nil, err
		}
		result = append(result, parsed)
	}
	return result, nil
}

// DefaultCapabilities returns the default container capabilities.
// These are the capabilities a non-privileged container gets.
func DefaultCapabilities() []string {
	return []string{
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_FSETID",
		"CAP_FOWNER",
		"CAP_MKNOD",
		"CAP_NET_RAW",
		"CAP_SETGID",
		"CAP_SETUID",
		"CAP_SETFCAP",
		"CAP_SETPCAP",
		"CAP_NET_BIND_SERVICE",
		"CAP_SYS_CHROOT",
		"CAP_KILL",
		"CAP_AUDIT_WRITE",
	}
}

// AllCapabilities returns all known capabilities.
func AllCapabilities() []string {
	caps := make([]string, 0, len(knownCapabilities))
	for c := range knownCapabilities {
		caps = append(caps, c)
	}
	return caps
}

// MaskedPaths returns the default masked paths for a container.
// These paths are mounted as tmpfs to prevent reading sensitive host data.
func MaskedPaths() []string {
	return []string{
		"/proc/kcore",
		"/proc/sysrq-trigger",
		"/proc/latency_stats",
		"/proc/timer_list",
		"/proc/timer_stats",
		"/proc/sched_debug",
		"/proc/scsi",
		"/sys/firmware",
		"/sys/devices/virtual/powercap",
	}
}

// ReadonlyPaths returns the default read-only paths for a container.
func ReadonlyPaths() []string {
	return []string{
		"/proc/asound",
		"/proc/bus",
		"/proc/fs",
		"/proc/irq",
		"/proc/sys",
		"/proc/sysrq-trigger",
	}
}

// DefaultSeccompProfile returns the default seccomp profile for a container.
func DefaultSeccompProfile() *LinuxSeccomp {
	return &LinuxSeccomp{
		DefaultAction: SeccompActErrno,
		Architectures: []Arch{ArchX86_64, ArchAARCH64, ArchX86, ArchARM},
		Syscalls: []LinuxSyscall{
			{Names: defaultAllowSyscalls(), Action: SeccompActAllow},
		},
	}
}

// PrivilegedSeccompProfile returns a permissive seccomp profile.
func PrivilegedSeccompProfile() *LinuxSeccomp {
	return &LinuxSeccomp{
		DefaultAction: SeccompActAllow,
	}
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
