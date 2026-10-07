package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/OpceanAI/Doki/internal/seccomp"
)

// seccompSpec returns the seccomp profile requested through SecurityOpt
// ("seccomp=<spec>"), or "" when the container did not request one.
func seccompSpec(opts []string) string {
	for _, o := range opts {
		if v, ok := strings.CutPrefix(o, "seccomp="); ok {
			return v
		}
	}
	return ""
}

// seccompShimCommand builds the command that starts the container under a real
// seccomp filter: a re-exec of the daemon binary in shim mode (internal/seccomp),
// which installs the profile and then exec()s argv. The filter survives exec,
// so the container process genuinely runs confined — this replaces the old
// ApplySeccomp no-op that never touched the container.
//
// The wrapper command's stdio, working directory and credentials are wired by
// the caller exactly like the real command's; the shim exec()s in place, so the
// container sees the same fds and cwd.
func seccompShimCommand(spec string, argv, env []string, dir string) (*exec.Cmd, error) {
	return confineShimCommand(spec, false, argv, env, dir)
}

// confineShimCommand is seccompShimCommand with an additional noNewPrivs flag
// (CRI noNewPrivs / Docker "no-new-privileges"): when true the shim sets
// PR_SET_NO_NEW_PRIVS via prctl before exec, so the target can never gain new
// privileges even without a seccomp profile.
func confineShimCommand(spec string, noNewPrivs bool, argv, env []string, dir string) (*exec.Cmd, error) {
	argvJSON, err := json.Marshal(argv)
	if err != nil {
		return nil, fmt.Errorf("seccomp: encode argv: %w", err)
	}
	base := env
	if base == nil {
		base = os.Environ()
	}
	nnb := "0"
	if noNewPrivs {
		nnb = "1"
	}
	cmd := exec.Command("/proc/self/exe")
	cmd.Dir = dir
	cmd.Env = append(append([]string{}, base...),
		seccomp.ShimEnv+"=1",
		seccomp.ShimEnvProfile+"="+spec,
		seccomp.ShimEnvArgv+"="+string(argvJSON),
		seccomp.ShimEnvNoNewPrivs+"="+nnb,
	)
	return cmd, nil
}

// apparmorSpec returns the AppArmor profile requested through SecurityOpt
// ("apparmor=<spec>"), or "" when the container did not request one.
func apparmorSpec(opts []string) string {
	for _, o := range opts {
		if v, ok := strings.CutPrefix(o, "apparmor="); ok {
			return v
		}
	}
	return ""
}

// hasNoNewPrivs reports whether SecurityOpt requests no-new-privileges
// (Docker "no-new-privileges" / CRI noNewPrivs translation).
func hasNoNewPrivs(opts []string) bool {
	for _, o := range opts {
		if o == "no-new-privileges" || o == "no-new-privileges=true" || o == "no-new-privileges=1" {
			return true
		}
	}
	return false
}

// CheckSecurityRequests rejects container security requests that this
// execution mode cannot honestly honor. It runs at CreateContainer (fail
// fast) and again at Start (covering non-CRI callers that set SecurityOpt
// directly):
//
//   - an explicit seccomp profile (localhost path or inline JSON — anything
//     but "default"/"unconfined") is enforced only in native mode through the
//     re-exec shim; elsewhere it is an error rather than a silent no-op. The
//     "default" profile degrades to warn+annotation outside native mode (it
//     is what Kubernetes sends for every pod) and to real enforcement inside
//     it; callers that cannot enforce must not claim they do (see Status.Info).
//   - any AppArmor confinement is an error: the runtime has no pre-exec apply
//     path for container processes, so entering the profile name would lie
//     about confinement that never happens. Use seccomp=... instead.
func CheckSecurityRequests(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	return checkSecurityRequests(ModeNative, cfg.SecurityOpt)
}

// checkSecurityRequests is the mode-parameterized core of
// CheckSecurityRequests so tests can exercise every mode without a Runtime.
func checkSecurityRequests(mode ExecutionMode, opts []string) error {
	if spec := seccompSpec(opts); spec != "" && spec != "unconfined" && spec != "default" {
		if mode != ModeNative {
			return fmt.Errorf("seccomp profile %q requested, but explicit seccomp profiles are enforced only in native mode (current mode %q): use seccomp=default or seccomp=unconfined",
				spec, mode)
		}
		if !seccomp.Supported() {
			return fmt.Errorf("seccomp profile %q requested, but this kernel/process cannot install seccomp filters", spec)
		}
	}
	if spec := apparmorSpec(opts); spec != "" && spec != "unconfined" {
		return fmt.Errorf("apparmor profile %q requested, but the doki runtime does not confine container processes with AppArmor (no pre-exec apply path): use apparmor=unconfined or seccomp=... instead",
			spec)
	}
	return nil
}
