//go:build linux

package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/OpceanAI/Doki/internal/seccomp"
)

// A profile that cannot be resolved is an error, never a silent no-op: a
// caller that asked for enforcement must not believe it got it.
func TestApplySeccompUnresolvedProfileIsHonest(t *testing.T) {
	if err := ApplySeccomp(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ApplySeccomp with a missing profile must fail, not silently succeed")
	}
	if err := ApplySeccomp("{not json"); err == nil {
		t.Fatal("ApplySeccomp with invalid inline JSON must fail")
	}
	if SeccompEnforced() {
		t.Fatal("SeccompEnforced must stay false after failed applications")
	}
}

// "unconfined" is the documented explicit no-op and returns nil without
// claiming enforcement.
func TestApplySeccompUnconfined(t *testing.T) {
	if err := ApplySeccomp("unconfined"); err != nil {
		t.Fatalf("unconfined: %v", err)
	}
}

// GetSecurityStatus must never report enforcement that did not happen.
func TestSecurityStatusHonest(t *testing.T) {
	st := GetSecurityStatus()
	if st.SeccompEnforced && !st.SeccompSupported {
		t.Error("seccomp cannot be enforced when it is not supported")
	}
	if st.AppArmorEnforced && !st.AppArmorSupported {
		t.Error("apparmor cannot be enforced when it is not supported")
	}
	t.Logf("security status: %+v", st)
}

// AppArmor application must either really confine the target or fail with an
// error — the old implementation returned nil without touching any process.
func TestApplyAppArmorHonest(t *testing.T) {
	err := ApplyAppArmor(os.Getpid(), "doki-apptest")
	if err == nil && !AppArmorEnforced() {
		t.Fatal("ApplyAppArmor returned nil but no profile is recorded as applied")
	}
	if err != nil {
		t.Logf("apparmor unavailable (honest error): %v", err)
	}
}

// The seccomp shim must really confine the exec'd process: a denied syscall
// returns the profile's errno instead of reaching the kernel. If the kernel
// refuses seccomp entirely, the shim must fail loudly with its own error
// message — either outcome is acceptable, silently running unconfined is not.
//
// Note: the denied syscall must be one the host actually permits (Android's
// zygote seccomp policy SIGSYS-kills some syscalls for every process, e.g.
// acct), so "uname" is used as the victim.
func TestSeccompShimEnforcesFilter(t *testing.T) {
	profile := `{
		"defaultAction": "SCMP_ACT_ALLOW",
		"syscalls": [{"names": ["uname"], "action": "SCMP_ACT_ERRNO", "errno": 1000}]
	}`
	profPath := filepath.Join(t.TempDir(), "deny-acct.json")
	if err := os.WriteFile(profPath, []byte(profile), 0600); err != nil {
		t.Fatal(err)
	}

	self, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{self, "-test.run=^TestSeccompProbeChild$"}
	argvJSON, err := json.Marshal(argv)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(self, "-test.run=^TestSeccompShimEnforcesFilter$")
	cmd.Env = append(os.Environ(),
		seccomp.ShimEnv+"=1",
		seccomp.ShimEnvProfile+"="+profPath,
		seccomp.ShimEnvArgv+"="+string(argvJSON),
		"DOKI_SECCOMP_PROBE=1",
	)
	outBytes, _ := cmd.CombinedOutput()
	out := string(outBytes)

	switch {
	case strings.Contains(out, "SECCOMP_PROBE:BLOCKED:1000"):
		// Real enforcement: the denied syscall got the profile errno.
	case strings.Contains(out, "doki seccomp shim:"):
		// Honest failure: this kernel/process cannot install filters.
		t.Logf("shim refused (honest): %s", strings.TrimSpace(out))
	default:
		t.Fatalf("seccomp neither enforced nor refused honestly; output:\n%s", out)
	}
}

// TestSeccompProbeChild is the payload executed under the seccomp filter by
// TestSeccompShimEnforcesFilter. It reports the errno of the syscall the
// profile denies (uname) and proves an allowed syscall still works. It skips
// in normal test runs.
func TestSeccompProbeChild(t *testing.T) {
	if os.Getenv("DOKI_SECCOMP_PROBE") != "1" {
		t.Skip("helper for TestSeccompShimEnforcesFilter")
	}
	var uts unix.Utsname
	err := unix.Uname(&uts)
	errno := unix.Errno(0)
	if err != nil {
		errno = err.(unix.Errno)
	}
	if pid := unix.Getpid(); pid <= 0 {
		_, _ = os.Stdout.WriteString("SECCOMP_PROBE:BROKEN:allowed syscall failed\n")
		return
	}
	if errno == 1000 {
		_, _ = os.Stdout.WriteString("SECCOMP_PROBE:BLOCKED:1000\n")
	} else {
		_, _ = os.Stdout.WriteString(fmt.Sprintf("SECCOMP_PROBE:UNBLOCKED:%d\n", errno))
	}
}

// Modes without an implemented start path must fail with an honest, actionable
// error instead of silently running the container unconfined.
func TestStartProcessExperimentalModeFailsHonestly(t *testing.T) {
	rt := newTestRuntime(t, t.TempDir())
	rt.mode = ModeGVisor
	cfg := &Config{ID: "exp-1", Args: []string{"/bin/true"}}
	_, _, err := rt.startProcess(cfg, t.TempDir(), nil)
	if err == nil {
		t.Fatal("experimental mode without a registry runner must fail")
	}
	if !strings.Contains(err.Error(), "experimental") {
		t.Fatalf("error must say the mode is experimental, got: %v", err)
	}
}
