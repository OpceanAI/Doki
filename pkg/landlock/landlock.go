//go:build linux

package landlock

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
)

const (
	LandlockAccessFSExecute     = 1 << 0
	LandlockAccessFSWriteFile   = 1 << 1
	LandlockAccessFSReadFile    = 1 << 2
	LandlockAccessFSReadDir     = 1 << 3
	LandlockAccessFSRemoveDir   = 1 << 4
	LandlockAccessFSRemoveFile  = 1 << 5
	LandlockAccessFSMakeChar    = 1 << 6
	LandlockAccessFSMakeDir     = 1 << 7
	LandlockAccessFSMakeReg     = 1 << 8
	LandlockAccessFSMakeSock    = 1 << 9
	LandlockAccessFSMakeFIFO    = 1 << 10
	LandlockAccessFSMakeBlock   = 1 << 11
	LandlockAccessFSMakeSym     = 1 << 12
	LandlockAccessFSRefer       = 1 << 13
	LandlockAccessFSTruncate    = 1 << 14
	LandlockAccessFSIoctlDev    = 1 << 15
	LandlockAccessFSResolveUnix = 1 << 16

	LandlockAccessNetBindTCP    = 1 << 0
	LandlockAccessNetConnectTCP = 1 << 1

	LandlockScopeAbstractUnixSocket = 1 << 0
	LandlockScopeSignal             = 1 << 1

	LandlockRulePathBeneath = 1
	LandlockRuleNetPort     = 2
)

type RulesetAttr struct {
	HandledAccessFS  uint64
	HandledAccessNet uint32
	HandledScopeIPC  uint32
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
	Path   string
	Access uint64
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
			attr.HandledAccessFS = LandlockAccessFSResolveUnix
			fallthrough
		case abi >= 8:
			attr.HandledAccessFS |= LandlockAccessFSIoctlDev
			fallthrough
		case abi >= 7:
			attr.HandledAccessFS |= LandlockAccessFSTruncate
			fallthrough
		case abi >= 6:
			attr.HandledScopeIPC = LandlockScopeSignal
			fallthrough
		case abi >= 5:
			attr.HandledScopeIPC = LandlockScopeAbstractUnixSocket
			fallthrough
		case abi >= 4:
			attr.HandledAccessNet = LandlockAccessNetBindTCP | LandlockAccessNetConnectTCP
			fallthrough
		case abi >= 3:
			attr.HandledAccessFS |= LandlockAccessFSRefer
			fallthrough
		case abi >= 2:
			attr.HandledAccessFS |= LandlockAccessFSMakeSym
			fallthrough
		case abi >= 1:
			attr.HandledAccessFS |= LandlockAccessFSExecute |
				LandlockAccessFSWriteFile |
				LandlockAccessFSReadFile |
				LandlockAccessFSReadDir
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
		handledAccessFS  uint64
		handledAccessNet uint32
		handledScopeIPC  uint32
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
		attr.HandledAccessFS = LandlockAccessFSExecute |
			LandlockAccessFSWriteFile |
			LandlockAccessFSReadFile |
			LandlockAccessFSReadDir |
			LandlockAccessFSRemoveDir |
			LandlockAccessFSRemoveFile |
			LandlockAccessFSMakeChar |
			LandlockAccessFSMakeDir |
			LandlockAccessFSMakeReg |
			LandlockAccessFSMakeSock |
			LandlockAccessFSMakeFIFO |
			LandlockAccessFSMakeBlock |
			LandlockAccessFSMakeSym |
			LandlockAccessFSRefer |
			LandlockAccessFSTruncate
	}
	if abi >= 4 {
		attr.HandledAccessNet = LandlockAccessNetBindTCP | LandlockAccessNetConnectTCP
	}
	if abi >= 6 {
		attr.HandledScopeIPC = LandlockScopeAbstractUnixSocket | LandlockScopeSignal
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
		err = addRule(rulesetFd, LandlockRulePathBeneath, &pathBeneath)
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
		err := addRule(rulesetFd, LandlockRuleNetPort, &netPort)
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
			{Path: rootfs, Access: LandlockAccessFSReadFile | LandlockAccessFSReadDir | LandlockAccessFSExecute},
			{Path: rootfs, Access: LandlockAccessFSWriteFile | LandlockAccessFSMakeReg | LandlockAccessFSMakeDir | LandlockAccessFSRemoveFile | LandlockAccessFSRemoveDir},
			{Path: "/dev/null", Access: LandlockAccessFSReadFile | LandlockAccessFSWriteFile},
			{Path: "/dev/zero", Access: LandlockAccessFSReadFile},
			{Path: "/dev/random", Access: LandlockAccessFSReadFile},
			{Path: "/dev/urandom", Access: LandlockAccessFSReadFile},
			{Path: "/proc", Access: LandlockAccessFSReadFile | LandlockAccessFSReadDir},
			{Path: "/sys", Access: LandlockAccessFSReadFile | LandlockAccessFSReadDir},
			{Path: "/tmp", Access: LandlockAccessFSReadFile | LandlockAccessFSWriteFile | LandlockAccessFSMakeReg | LandlockAccessFSRemoveFile},
		},
	}

	for _, port := range ports {
		cfg.NetRules = append(cfg.NetRules, NetRule{
			Port:   port,
			Access: LandlockAccessNetBindTCP | LandlockAccessNetConnectTCP,
		})
	}

	cfg.IPCRules = []IPCRule{
		{Scope: LandlockScopeAbstractUnixSocket | LandlockScopeSignal},
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
		Access: LandlockAccessNetBindTCP | LandlockAccessNetConnectTCP,
	})
}

func init() {
	if _, err := os.Stat("/proc/sys/kernel/osrelease"); err == nil {
		_ = GetABI()
	}
}
