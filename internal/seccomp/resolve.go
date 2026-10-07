package seccomp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Environment contract of the seccomp shim. The daemon re-executes its own
// binary (/proc/self/exe) with these variables set; the shim's init() hook
// (shim_linux.go) installs the requested filter and then exec()s the target
// command with the variables scrubbed.
const (
	ShimEnv        = "DOKI_SECCOMP_SHIM"         // "1" turns the process into the shim
	ShimEnvProfile = "DOKI_SECCOMP_SHIM_PROFILE" // builtin name, profile path, or inline JSON
	ShimEnvArgv    = "DOKI_SECCOMP_SHIM_ARGV"    // JSON array: argv of the target command
	// ShimEnvNoNewPrivs asks the shim to set NO_NEW_PRIVS via prctl before
	// exec (CRI noNewPrivs / Docker "no-new-privileges"). "1" enables it.
	ShimEnvNoNewPrivs = "DOKI_SECCOMP_SHIM_NONEWPRIVS"
)

// ResolveProfile maps a profile spec to a profile. Accepted specs:
//
//	"" / "unconfined"             no filter (nil, nil)
//	"default" / "runtime/default" DefaultProfile()
//	"privileged"                  PrivilegedProfile()
//	"android" / "arm"             aliases for the default deny-list profile
//	"{...}"                       inline seccomp profile JSON
//	anything else                 a path to a profile JSON file
//
// Unknown specs are an error: callers must never silently downgrade a
// requested profile to "no enforcement".
func ResolveProfile(spec string) (*Profile, error) {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "", "unconfined":
		return nil, nil
	case "default", "runtime/default", "android", "arm":
		return DefaultProfile(), nil
	case "privileged":
		return PrivilegedProfile(), nil
	}
	if strings.HasPrefix(spec, "{") {
		var p Profile
		if err := json.Unmarshal([]byte(spec), &p); err != nil {
			return nil, fmt.Errorf("parse inline seccomp profile: %w", err)
		}
		return &p, nil
	}
	if p, err := LoadProfile(spec); err == nil {
		return p, nil
	}
	return nil, fmt.Errorf("seccomp profile %q is neither a builtin (default|privileged|android|arm|unconfined) nor a readable JSON file", spec)
}
