//go:build linux

package landlock

import (
	"fmt"
	"os"
	"runtime"
	"golang.org/x/sys/unix"
)

const (
	LANDLOCK_ACCESS_FS_EXECUTE      = 1 << 0
	LANDLOCK_ACCESS_FS_WRITE_FILE   = 1 << 1
	LANDLOCK_ACCESS_FS_READ_FILE    = 1 << 2
	LANDLOCK_ACCESS_FS_READ_DIR     = 1 << 3
	LANDLOCK_ACCESS_FS_REMOVE_DIR   = 1 << 4
	LANDLOCK_ACCESS_FS_REMOVE_FILE  = 1 << 5
	LANDLOCK_ACCESS_FS_MAKE_CHAR    = 1 << 6
	LANDLOCK_ACCESS_FS_MAKE_DIR     = 1 << 7
	LANDLOCK_ACCESS_FS_MAKE_REG     = 1 << 8
	LANDLOCK_ACCESS_FS_MAKE_SOCK    = 1 << 9
	LANDLOCK_ACCESS_FS_MAKE_FIFO    = 1 << 10
	LANDLOCK_ACCESS_FS_MAKE_BLOCK   = 1 << 11
	LANDLOCK_ACCESS_FS_MAKE_SYM     = 1 << 12
	LANDLOCK_ACCESS_FS_REFER        = 1 << 13
	LANDLOCK_ACCESS_FS_TRUNCATE     = 1 << 14
	LANDLOCK_ACCESS_FS_IOCTL_DEV    = 1 << 15
	LANDLOCK_ACCESS_FS_RESOLVE_UNIX = 1 << 16

	LANDLOCK_ACCESS_NET_BIND_TCP    = 1 << 0
	LANDLOCK_ACCESS_NET_CONNECT_TCP = 1 << 1

	LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET = 1 << 0
	LANDLOCK_SCOPE_SIGNAL               = 1 << 1

	LANDLOCK_RULE_PATH_BENEATH = 1
	LANDLOCK_RULE_NET_PORT     = 2
)

type RulesetAttr struct {
	HandledAccessFS   uint64
	HandledAccessNet  uint32
	HandledScopeIPC   uint32
}

type PathBeneathAttr struct {
	AllowedAccess uint64
	ParentFd      int32
}

type NetPortAttr struct {
	AllowedAccess uint32
	Port          uint64
}

type FSRule struct {
	Path    string
	Access  uint64
}

type NetRule struct {
	Port   uint16
	Access uint32
}

type IPCRule struct {
	Scope uint32
}

type SandboxConfig struct {
	FSRules  []FSRule
	NetRules []NetRule
	IPCRules []IPCRule
}

func GetABI() int {
	err := unix.Prctl(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil {
		return 0
	}
	return detectABI()
}

func detectABI() int {
	for abi := 9; abi >= 1; abi-- {
		attr := RulesetAttr{}
		switch {
		case abi >= 9:
			attr.HandledAccessFS = LANDLOCK_ACCESS_FS_RESOLVE_UNIX
			fallthrough
		case abi >= 8:
			attr.HandledAccessFS |= LANDLOCK_ACCESS_FS_IOCTL_DEV
			fallthrough
		case abi >= 7:
			attr.HandledAccessFS |= LANDLOCK_ACCESS_FS_TRUNCATE
			fallthrough
		case abi >= 6:
			attr.HandledScopeIPC = LANDLOCK_SCOPE_SIGNAL
			fallthrough
		case abi >= 5:
			attr.HandledScopeIPC = LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET
			fallthrough
		case abi >= 4:
			attr.HandledAccessNet = LANDLOCK_ACCESS_NET_BIND_TCP | LANDLOCK_ACCESS_NET_CONNECT_TCP
			fallthrough
		case abi >= 3:
			attr.HandledAccessFS |= LANDLOCK_ACCESS_FS_REFER
			fallthrough
		case abi >= 2:
			attr.HandledAccessFS |= LANDLOCK_ACCESS_FS_MAKE_SYM
			fallthrough
		case abi >= 1:
			attr.HandledAccessFS |= LANDLOCK_ACCESS_FS_EXECUTE |
				LANDLOCK_ACCESS_FS_WRITE_FILE |
				LANDLOCK_ACCESS_FS_READ_FILE |
				LANDLOCK_ACCESS_FS_READ_DIR
		}

		fd, err := landlockCreateRuleset(attr)
		if err == nil {
			_ = unix.Close(fd)
			return abi
		}
	}
	return 0
}

func landlockCreateRuleset(attr RulesetAttr) (int, error) {
	type landlockRulesetAttr struct {
		handledAccessFS   uint64
		handledAccessNet  uint32
		handledScopeIPC   uint32
	}
	raw := landlockRulesetAttr{
		handledAccessFS:  attr.HandledAccessFS,
		handledAccessNet: attr.HandledAccessNet,
		handledScopeIPC:  attr.HandledScopeIPC,
	}
	fd, _, errno := unix.Syscall(444, uintptr(0), 0, 0)
	if errno != 0 {
		return 0, fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	_ = raw
	return int(fd), nil
}

func ApplySandbox(cfg *SandboxConfig) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	abi := GetABI()
	if abi == 0 {
		return fmt.Errorf("landlock not available")
	}

	attr := RulesetAttr{}
	if abi >= 1 {
		attr.HandledAccessFS = LANDLOCK_ACCESS_FS_EXECUTE |
			LANDLOCK_ACCESS_FS_WRITE_FILE |
			LANDLOCK_ACCESS_FS_READ_FILE |
			LANDLOCK_ACCESS_FS_READ_DIR |
			LANDLOCK_ACCESS_FS_REMOVE_DIR |
			LANDLOCK_ACCESS_FS_REMOVE_FILE |
			LANDLOCK_ACCESS_FS_MAKE_CHAR |
			LANDLOCK_ACCESS_FS_MAKE_DIR |
			LANDLOCK_ACCESS_FS_MAKE_REG |
			LANDLOCK_ACCESS_FS_MAKE_SOCK |
			LANDLOCK_ACCESS_FS_MAKE_FIFO |
			LANDLOCK_ACCESS_FS_MAKE_BLOCK |
			LANDLOCK_ACCESS_FS_MAKE_SYM |
			LANDLOCK_ACCESS_FS_REFER |
			LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 4 {
		attr.HandledAccessNet = LANDLOCK_ACCESS_NET_BIND_TCP | LANDLOCK_ACCESS_NET_CONNECT_TCP
	}
	if abi >= 6 {
		attr.HandledScopeIPC = LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | LANDLOCK_SCOPE_SIGNAL
	}

	rulesetFd, err := landlockCreateRuleset(attr)
	if err != nil {
		return fmt.Errorf("create ruleset: %w", err)
	}
	defer func() { _ = unix.Close(rulesetFd) }()

	for _, rule := range cfg.FSRules {
		fd, err := unix.Open(rule.Path, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			return fmt.Errorf("open %s: %w", rule.Path, err)
		}
		pathBeneath := PathBeneathAttr{
			AllowedAccess: rule.Access,
			ParentFd:      int32(fd),
		}
		err = addRule(rulesetFd, LANDLOCK_RULE_PATH_BENEATH, &pathBeneath)
		_ = unix.Close(fd)
		if err != nil {
			return fmt.Errorf("add fs rule for %s: %w", rule.Path, err)
		}
	}

	for _, rule := range cfg.NetRules {
		netPort := NetPortAttr{
			AllowedAccess: rule.Access,
			Port:          uint64(rule.Port),
		}
		err := addRule(rulesetFd, LANDLOCK_RULE_NET_PORT, &netPort)
		if err != nil {
			return fmt.Errorf("add net rule for port %d: %w", rule.Port, err)
		}
	}

	_ = unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)

	_, _, errno := unix.Syscall(446, uintptr(rulesetFd), 0, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}

	return nil
}

func addRule(rulesetFd int, ruleType int, attr interface{}) error {
	_, _, errno := unix.Syscall(445, uintptr(rulesetFd), uintptr(ruleType), 0)
	if errno != 0 {
		return fmt.Errorf("landlock_add_rule: %w", errno)
	}
	return nil
}

func DefaultContainerSandbox(rootfs string, ports []uint16) *SandboxConfig {
	cfg := &SandboxConfig{
		FSRules: []FSRule{
			{Path: rootfs, Access: LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR | LANDLOCK_ACCESS_FS_EXECUTE},
			{Path: rootfs, Access: LANDLOCK_ACCESS_FS_WRITE_FILE | LANDLOCK_ACCESS_FS_MAKE_REG | LANDLOCK_ACCESS_FS_MAKE_DIR | LANDLOCK_ACCESS_FS_REMOVE_FILE | LANDLOCK_ACCESS_FS_REMOVE_DIR},
			{Path: "/dev/null", Access: LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_WRITE_FILE},
			{Path: "/dev/zero", Access: LANDLOCK_ACCESS_FS_READ_FILE},
			{Path: "/dev/random", Access: LANDLOCK_ACCESS_FS_READ_FILE},
			{Path: "/dev/urandom", Access: LANDLOCK_ACCESS_FS_READ_FILE},
			{Path: "/proc", Access: LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR},
			{Path: "/sys", Access: LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR},
			{Path: "/tmp", Access: LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_WRITE_FILE | LANDLOCK_ACCESS_FS_MAKE_REG | LANDLOCK_ACCESS_FS_REMOVE_FILE},
		},
	}

	for _, port := range ports {
		cfg.NetRules = append(cfg.NetRules, NetRule{
			Port:   port,
			Access: LANDLOCK_ACCESS_NET_BIND_TCP | LANDLOCK_ACCESS_NET_CONNECT_TCP,
		})
	}

	cfg.IPCRules = []IPCRule{
		{Scope: LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | LANDLOCK_SCOPE_SIGNAL},
	}

	return cfg
}

func Available() bool {
	return GetABI() > 0
}

func MustAllowPath(cfg *SandboxConfig, path string, access uint64) {
	cfg.FSRules = append(cfg.FSRules, FSRule{Path: path, Access: access})
}

func MustAllowTCPPort(cfg *SandboxConfig, port uint16) {
	cfg.NetRules = append(cfg.NetRules, NetRule{
		Port:   port,
		Access: LANDLOCK_ACCESS_NET_BIND_TCP | LANDLOCK_ACCESS_NET_CONNECT_TCP,
	})
}

func init() {
	if _, err := os.Stat("/proc/sys/kernel/osrelease"); err == nil {
		_ = GetABI()
	}
}
