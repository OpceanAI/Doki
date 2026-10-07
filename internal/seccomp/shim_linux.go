//go:build linux

package seccomp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// init intercepts shim mode: when the process was spawned as a seccomp shim
// (see the environment contract in resolve.go), it must never fall through to
// the daemon's main(). The shim installs the requested filter and exec()s the
// real container command, which then runs under the filter.
func init() {
	if os.Getenv(ShimEnv) == "1" {
		// Shim mode: never fall through to the daemon's main().
		os.Exit(RunShim(os.Stderr))
	}
}

// RunShim installs the requested seccomp profile and exec()s the target
// command. It returns a process exit code and only returns on failure.
func RunShim(errw io.Writer) int {
	var argv []string
	if err := json.Unmarshal([]byte(os.Getenv(ShimEnvArgv)), &argv); err != nil || len(argv) == 0 {
		fmt.Fprintf(errw, "doki seccomp shim: missing or invalid %s: %v\n", ShimEnvArgv, err)
		return 127
	}

	profile, err := ResolveProfile(os.Getenv(ShimEnvProfile))
	if err != nil {
		fmt.Fprintf(errw, "doki seccomp shim: %v\n", err)
		return 127
	}
	if profile != nil {
		if err := ApplyToSelf(profile); err != nil {
			fmt.Fprintf(errw, "doki seccomp shim: %v\n", err)
			return 127
		}
	}

	// CRI noNewPrivs / Docker "no-new-privileges": once set, the exec'd
	// target (and its children) can never gain new privileges via setuid
	// binaries or file capabilities. Must precede the exec below.
	if os.Getenv(ShimEnvNoNewPrivs) == "1" {
		if err := setNoNewPrivs(); err != nil {
			fmt.Fprintf(errw, "doki seccomp shim: no_new_privs: %v\n", err)
			return 127
		}
	}

	if err := syscall.Exec(argv[0], argv, scrubShimEnv(os.Environ())); err != nil {
		fmt.Fprintf(errw, "doki seccomp shim: exec %s: %v\n", argv[0], err)
		return 127
	}
	return 0
}

// setNoNewPrivs sets PR_SET_NO_NEW_PRIVS so the exec'd target (and its
// children) can never gain new privileges via setuid binaries or file
// capabilities. The bit is irreversible once set.
func setNoNewPrivs() error {
	return unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
}

// scrubShimEnv removes the shim's own control variables so the exec'd target
// cannot re-trigger the shim (and does not inherit confusing internals).
func scrubShimEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if key == ShimEnv || key == ShimEnvProfile || key == ShimEnvArgv || key == ShimEnvNoNewPrivs {
			continue
		}
		out = append(out, kv)
	}
	return out
}
