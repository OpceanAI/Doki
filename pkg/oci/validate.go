package oci

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidationError is a validation error with a field path.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("oci: %s: %s", e.Field, e.Message)
}

// ValidationErrors is a list of validation errors.
type ValidationErrors []*ValidationError

func (ve ValidationErrors) Error() string {
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// Validate validates an OCI spec. Returns all validation errors found.
func Validate(spec *Spec) error {
	var errs ValidationErrors

	if spec.Version == "" {
		errs = append(errs, &ValidationError{"ociVersion", "must not be empty"})
	}

	if spec.Process == nil {
		errs = append(errs, &ValidationError{"process", "must not be nil"})
	} else {
		errs = append(errs, validateProcess(spec.Process)...)
	}

	if spec.Root == nil {
		errs = append(errs, &ValidationError{"root", "must not be nil"})
	} else {
		errs = append(errs, validateRoot(spec.Root)...)
	}

	if spec.Linux != nil {
		errs = append(errs, validateLinux(spec.Linux)...)
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validateProcess(p *Process) ValidationErrors {
	var errs ValidationErrors

	if len(p.Args) == 0 && len(p.Entrypoint) == 0 {
		errs = append(errs, &ValidationError{"process.args", "must not be empty (no args or entrypoint)"})
	}

	if p.Cwd != "" {
		if !filepath.IsAbs(p.Cwd) {
			errs = append(errs, &ValidationError{"process.cwd", "must be an absolute path"})
		}
	}

	if p.OOMScoreAdj != nil {
		if *p.OOMScoreAdj < -1000 || *p.OOMScoreAdj > 1000 {
			errs = append(errs, &ValidationError{"process.oomScoreAdj", "must be between -1000 and 1000"})
		}
	}

	for i, rlimit := range p.Rlimits {
		if rlimit.Type == "" {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("process.rlimits[%d].type", i),
				Message: "must not be empty",
			})
		}
		if rlimit.Hard < rlimit.Soft {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("process.rlimits[%d]", i),
				Message: "hard limit must be >= soft limit",
			})
		}
	}

	return errs
}

func validateRoot(r *Root) ValidationErrors {
	var errs ValidationErrors

	if r.Path == "" {
		errs = append(errs, &ValidationError{"root.path", "must not be empty"})
	}

	return errs
}

func validateLinux(l *Linux) ValidationErrors {
	var errs ValidationErrors

	// Validate namespaces.
	seen := make(map[LinuxNamespaceType]bool)
	for i, ns := range l.Namespaces {
		if ns.Type == "" {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("linux.namespaces[%d].type", i),
				Message: "must not be empty",
			})
		}
		validTypes := map[LinuxNamespaceType]bool{
			NamespacePID: true, NamespaceNetwork: true, NamespaceMount: true,
			NamespaceIPC: true, NamespaceUTS: true, NamespaceUser: true,
			NamespaceCgroup: true, NamespaceTime: true,
		}
		if !validTypes[ns.Type] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("linux.namespaces[%d].type", i),
				Message: fmt.Sprintf("invalid namespace type: %s", ns.Type),
			})
		}
		if seen[ns.Type] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("linux.namespaces[%d].type", i),
				Message: fmt.Sprintf("duplicate namespace type: %s", ns.Type),
			})
		}
		seen[ns.Type] = true
	}

	// Validate UID/GID mappings.
	for i, m := range l.UIDMappings {
		if m.Size == 0 {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("linux.uidMappings[%d].size", i),
				Message: "must not be 0",
			})
		}
	}
	for i, m := range l.GIDMappings {
		if m.Size == 0 {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("linux.gidMappings[%d].size", i),
				Message: "must not be 0",
			})
		}
	}

	// Validate seccomp.
	if l.Seccomp != nil {
		errs = append(errs, validateSeccomp(l.Seccomp)...)
	}

	// Validate resources.
	if l.Resources != nil {
		errs = append(errs, validateResources(l.Resources)...)
	}

	return errs
}

func validateSeccomp(s *LinuxSeccomp) ValidationErrors {
	var errs ValidationErrors

	validActions := map[LinuxSeccompAction]bool{
		SeccompActKill: true, SeccompActKillProcess: true,
		SeccompActTrap: true, SeccompActErrno: true,
		SeccompActTrace: true, SeccompActAllow: true,
		SeccompActLog: true,
	}
	if !validActions[s.DefaultAction] {
		errs = append(errs, &ValidationError{
			Field:   "linux.seccomp.defaultAction",
			Message: fmt.Sprintf("invalid action: %s", s.DefaultAction),
		})
	}

	for i, sc := range s.Syscalls {
		if len(sc.Names) == 0 {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("linux.seccomp.syscalls[%d].names", i),
				Message: "must not be empty",
			})
		}
		if !validActions[sc.Action] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("linux.seccomp.syscalls[%d].action", i),
				Message: fmt.Sprintf("invalid action: %s", sc.Action),
			})
		}
	}

	return errs
}

func validateResources(r *LinuxResources) ValidationErrors {
	var errs ValidationErrors

	if r.Memory != nil {
		if r.Memory.Limit != nil && r.Memory.Reservation != nil {
			if *r.Memory.Reservation > *r.Memory.Limit {
				errs = append(errs, &ValidationError{
					Field:   "linux.resources.memory",
					Message: "reservation must be <= limit",
				})
			}
		}
	}

	if r.CPU != nil {
		if r.CPU.Quota != nil && *r.CPU.Quota < 0 {
			errs = append(errs, &ValidationError{
				Field:   "linux.resources.cpu.quota",
				Message: "must be >= 0",
			})
		}
	}

	if r.Pids != nil {
		if r.Pids.Limit < 0 {
			errs = append(errs, &ValidationError{
				Field:   "linux.resources.pids.limit",
				Message: "must be >= 0",
			})
		}
	}

	return errs
}

// ValidateConfig validates a container config and returns validation errors.
// This is a convenience wrapper around Validate that builds a minimal Spec.
func ValidateConfig(args []string, rootfsPath string) error {
	var errs ValidationErrors

	if len(args) == 0 {
		errs = append(errs, &ValidationError{"args", "must not be empty"})
	}

	if rootfsPath == "" {
		errs = append(errs, &ValidationError{"rootfs", "must not be empty"})
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}
