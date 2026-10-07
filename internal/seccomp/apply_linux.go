//go:build linux

package seccomp

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

// seccomp(2) and prctl(2) constants not exported by x/sys/unix.
const (
	seccompSetModeFilter = 1  // SECCOMP_SET_MODE_FILTER
	prctlSetSeccomp      = 22 // PR_SET_SECCOMP
	seccompModeFilter    = 2  // SECCOMP_MODE_FILTER
	maxFilterInsns       = 4096
)

// classic BPF instruction encodings used by seccomp filters.
const (
	bpfLoadWordAbs = 0x20 // BPF_LD | BPF_W | BPF_ABS
	bpfJmpJeqK     = 0x15 // BPF_JMP | BPF_JEQ | BPF_K
	bpfRetK        = 0x06 // BPF_RET | BPF_K
)

// seccomp_data offsets.
const (
	offsetNR   = 0
	offsetArch = 4
)

// native audit architecture identifiers (linux/audit.h).
const (
	auditArchX86_64  = 0xc000003e
	auditArchAARCH64 = 0xc00000b7
	auditArchARM     = 0x40000028
	auditArchI386    = 0x40000003
)

// applied records whether a filter was actually installed by this process, so
// callers can report the real enforcement state instead of guessing.
var applied atomic.Bool

// Applied reports whether this process/thread really runs under a seccomp
// filter installed by this package. Callers must never claim seccomp is active
// when this returns false.
func Applied() bool { return applied.Load() }

// sockFprog mirrors the kernel's struct sock_fprog. Go's field alignment
// matches the C layout on both 32- and 64-bit: the pointer is naturally
// aligned after the uint16 length.
type sockFprog struct {
	length uint16
	filter *unix.SockFilter
}

// retAction converts an SCMP_ACT_* string into a seccomp return value.
func retAction(action string, errno int) (uint32, error) {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "SCMP_ACT_ALLOW", "":
		return unix.SECCOMP_RET_ALLOW, nil
	case "SCMP_ACT_ERRNO":
		if errno == 0 {
			errno = int(unix.EPERM)
		}
		return unix.SECCOMP_RET_ERRNO | (uint32(errno) & 0xffff), nil
	case "SCMP_ACT_KILL", "SCMP_ACT_KILL_THREAD":
		return unix.SECCOMP_RET_KILL, nil
	case "SCMP_ACT_KILL_PROCESS":
		return unix.SECCOMP_RET_KILL_PROCESS, nil
	case "SCMP_ACT_TRAP":
		return unix.SECCOMP_RET_TRAP, nil
	case "SCMP_ACT_LOG":
		return unix.SECCOMP_RET_LOG, nil
	default:
		return 0, fmt.Errorf("unsupported seccomp action %q (supported: SCMP_ACT_ALLOW, SCMP_ACT_ERRNO, SCMP_ACT_KILL, SCMP_ACT_KILL_PROCESS, SCMP_ACT_TRAP, SCMP_ACT_LOG)", action)
	}
}

// nativeAuditArch returns the audit architecture constant for this build.
func nativeAuditArch() (uint32, bool) {
	switch runtime.GOARCH {
	case "amd64":
		return auditArchX86_64, true
	case "arm64":
		return auditArchAARCH64, true
	case "arm":
		return auditArchARM, true
	case "386":
		return auditArchI386, true
	default:
		return 0, false
	}
}

// Compile translates a profile into a classic BPF program for seccomp(2).
//
// Semantics are first-match-wins over the rule order in the profile: each
// syscall named by a rule returns that rule's action; everything else falls
// through to the profile's DefaultAction.
//
// Honest limitations (surfaced as errors or documented skips, never silent):
//   - argument-level filters (ArgRule) are not compiled; a profile that uses
//     them is rejected;
//   - syscall names that do not exist on the current architecture are skipped
//     (they can never be invoked there);
//   - SECCOMP_FILTER_FLAG_TSYNC is ignored: the filter is installed on the
//     calling thread only, which is exactly what the pre-exec shim needs.
func Compile(p *Profile) ([]unix.SockFilter, error) {
	if p == nil {
		return nil, fmt.Errorf("nil seccomp profile")
	}
	if p.DefaultAction == "" {
		return nil, fmt.Errorf("seccomp profile: defaultAction is required")
	}
	defaultRet, err := retAction(p.DefaultAction, 0)
	if err != nil {
		return nil, fmt.Errorf("seccomp profile: %w", err)
	}

	insns := make([]unix.SockFilter, 0, 16+2*len(syscallNumbers))

	// Reject programs from another architecture rather than misinterpreting
	// their syscall numbers.
	if arch, ok := nativeAuditArch(); ok {
		insns = append(insns,
			unix.SockFilter{Code: bpfLoadWordAbs, K: offsetArch},
			unix.SockFilter{Code: bpfJmpJeqK, Jt: 1, Jf: 0, K: arch},
			unix.SockFilter{Code: bpfRetK, K: unix.SECCOMP_RET_KILL_PROCESS},
		)
	}
	// Load the syscall number.
	insns = append(insns, unix.SockFilter{Code: bpfLoadWordAbs, K: offsetNR})

	for _, rule := range p.Syscalls {
		if len(rule.Args) > 0 {
			return nil, fmt.Errorf("seccomp profile: argument filters on %v are not supported", rule.Names)
		}
		ret, err := retAction(rule.Action, rule.Errno)
		if err != nil {
			return nil, fmt.Errorf("seccomp profile: %w", err)
		}
		for _, name := range rule.Names {
			nr, ok := syscallNumbers[strings.ToLower(strings.TrimSpace(name))]
			if !ok {
				// Not present on this architecture's syscall table: skip.
				continue
			}
			// If the syscall number matches, return the rule action; otherwise
			// fall through to the next entry.
			insns = append(insns,
				unix.SockFilter{Code: bpfJmpJeqK, Jt: 0, Jf: 1, K: nr},
				unix.SockFilter{Code: bpfRetK, K: ret},
			)
		}
	}
	insns = append(insns, unix.SockFilter{Code: bpfRetK, K: defaultRet})

	if len(insns) > maxFilterInsns {
		return nil, fmt.Errorf("seccomp profile: filter too large (%d instructions, max %d)", len(insns), maxFilterInsns)
	}
	return insns, nil
}

// ApplyToSelf installs the profile as a seccomp filter on the calling thread.
// The filter is inherited across fork/clone/exec, so the intended use is right
// before exec'ing the container process (see the shim in shim_linux.go).
//
// The call fails honestly when the kernel refuses (no CAP_SYS_ADMIN and no
// no_new_privs, seccomp disabled, or a non-Linux kernel): callers get an error
// and must not report seccomp as active.
func ApplyToSelf(p *Profile) error {
	if p == nil {
		return nil // explicit "unconfined": no filter, no claim
	}
	insns, err := Compile(p)
	if err != nil {
		return err
	}

	// The filter only applies to this thread; pin it so a subsequent exec on
	// the same goroutine keeps running under the filter.
	runtime.LockOSThread()

	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}

	prog := sockFprog{length: uint16(len(insns)), filter: &insns[0]}
	if _, _, errno := unix.Syscall(unix.SYS_SECCOMP, seccompSetModeFilter, 0, uintptr(unsafe.Pointer(&prog))); errno != 0 {
		// Fallback for kernels without seccomp(2) (pre-3.17).
		if _, _, errno2 := unix.Syscall(unix.SYS_PRCTL, prctlSetSeccomp, seccompModeFilter, uintptr(unsafe.Pointer(&prog))); errno2 != 0 {
			return fmt.Errorf("seccomp(SECCOMP_SET_MODE_FILTER): %w (prctl fallback: %v)", errno, errno2)
		}
	}
	applied.Store(true)
	return nil
}

var (
	supportedOnce sync.Once
	supportedVal  bool
)

// Supported probes whether this kernel accepts seccomp filters from this
// process. The probe installs a harmless allow-all filter on a throwaway
// thread, so it has no security effect and no lasting behavioural effect.
func Supported() bool {
	supportedOnce.Do(func() {
		done := make(chan error, 1)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			insns := []unix.SockFilter{{Code: bpfRetK, K: unix.SECCOMP_RET_ALLOW}}
			if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
				done <- err
				return
			}
			prog := sockFprog{length: uint16(len(insns)), filter: &insns[0]}
			_, _, errno := unix.Syscall(unix.SYS_SECCOMP, seccompSetModeFilter, 0, uintptr(unsafe.Pointer(&prog)))
			if errno != 0 {
				done <- errno
				return
			}
			done <- nil
		}()
		supportedVal = <-done == nil
	})
	return supportedVal
}
