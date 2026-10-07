package cri

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// unenforcedAnnotation records security requests the runtime accepted
// best-effort but does not enforce. It is stamped onto the container's
// annotations so auditors see the gap instead of assuming confinement that
// is not there.
const unenforcedAnnotation = "doki.io/unenforced-security"

// securitySettings is the enforceable subset of a CRI
// LinuxContainerSecurityContext, mapped onto runtime knobs. Fields the
// runtime cannot honor are reported through the unenforced annotation rather
// than silently dropped.
type securitySettings struct {
	capAdd        []string
	capDrop       []string
	privileged    bool
	readOnly      bool
	user          string
	securityOpt   []string
	hostNetwork   bool
	unconfinedApp bool
}

// applySecurityContext translates cfg's Linux.SecurityContext into runtime
// settings. It covers all 13 SecurityContext fields:
//
//   - capabilities add/drop -> CapAdd/CapDrop (ambient capabilities have no
//     plumbing anywhere in the start paths, so they are an honest error);
//   - privileged -> Privileged (per spec, seccomp/apparmor then have no
//     effect and are skipped);
//   - namespaceOptions: network=NODE -> host network; anything else that
//     demands isolation the runtime cannot provide is annotated, never
//     silently claimed;
//   - selinuxOptions -> honest error (no SELinux enforcement path exists);
//   - runAsUser/runAsGroup -> numeric "uid[:gid]"; group without user is an
//     error per spec; runAsUsername cannot be resolved against the image
//     passwd, so it is an honest error directing to numeric UIDs;
//   - readonlyRootfs -> ReadOnly;
//   - supplementalGroups / policy -> annotated (no group plumbing in the
//     start paths; Strict is additionally advertised as unsupported in
//     Status.Features);
//   - noNewPrivs -> "no-new-privileges" SecurityOpt (enforced via the
//     re-exec shim in native mode, annotated elsewhere);
//   - maskedPaths / readonlyPaths -> annotated (no OCI spec application in
//     the start paths);
//   - seccomp / apparmor SecurityProfiles -> "seccomp="/"apparmor="
//     SecurityOpt entries; enforceability per mode/host is decided by the
//     runtime gate at Start (explicit localhost profiles fail instead of
//     running unconfined).
func applySecurityContext(cfg *v1.ContainerConfig) (*securitySettings, error) {
	out := &securitySettings{}
	sc := cfg.GetLinux().GetSecurityContext()
	if sc == nil {
		return out, nil
	}
	var unenforced []string
	note := func(s string) { unenforced = append(unenforced, s) }
	name := cfg.GetMetadata().GetName()

	if caps := sc.GetCapabilities(); caps != nil {
		if ambient := caps.GetAddAmbientCapabilities(); len(ambient) > 0 {
			return nil, status.Errorf(codes.InvalidArgument,
				"ambient capabilities %v are not supported by the doki runtime (no file-capability plumbing in the start paths)", ambient)
		}
		out.capAdd = append(out.capAdd, caps.GetAddCapabilities()...)
		out.capDrop = append(out.capDrop, caps.GetDropCapabilities()...)
	}
	out.privileged = sc.GetPrivileged()

	if ns := sc.GetNamespaceOptions(); ns != nil {
		switch ns.GetNetwork() {
		case v1.NamespaceMode_NODE:
			out.hostNetwork = true
		case v1.NamespaceMode_POD:
			// Default pod-network sharing: nothing to do.
		default:
			note("namespace-network=" + ns.GetNetwork().String())
		}
		if pid := ns.GetPid(); pid != v1.NamespaceMode_POD {
			note("pid-namespace=" + pid.String() + " (containers share the daemon's pid visibility outside namespaces mode)")
		}
		if ipc := ns.GetIpc(); ipc != v1.NamespaceMode_POD {
			note("ipc-namespace=" + ipc.String())
		}
		if userns := ns.GetUsernsOptions(); userns != nil {
			note("userns=" + userns.GetMode().String() + " (user namespaces only in rootless namespaces mode)")
		}
	}

	if sel := sc.GetSelinuxOptions(); sel != nil &&
		(sel.GetUser() != "" || sel.GetRole() != "" || sel.GetType() != "" || sel.GetLevel() != "") {
		return nil, status.Errorf(codes.InvalidArgument,
			"SELinux options (user=%q role=%q type=%q level=%q) are not supported by the doki runtime: no SELinux enforcement path exists, refusing to run mislabeled",
			sel.GetUser(), sel.GetRole(), sel.GetType(), sel.GetLevel())
	}

	if u := sc.GetRunAsUser(); u != nil {
		if g := sc.GetRunAsGroup(); g != nil {
			out.user = strconv.FormatInt(u.GetValue(), 10) + ":" + strconv.FormatInt(g.GetValue(), 10)
		} else {
			out.user = strconv.FormatInt(u.GetValue(), 10)
		}
	} else if sc.GetRunAsGroup() != nil {
		return nil, status.Errorf(codes.InvalidArgument, "run_as_group requires run_as_user (CRI spec)")
	}
	if username := sc.GetRunAsUsername(); username != "" {
		return nil, status.Errorf(codes.InvalidArgument,
			"run_as_username %q is not supported: the runtime cannot resolve names against the image /etc/passwd; use run_as_user with a numeric UID", username)
	}

	out.readOnly = sc.GetReadonlyRootfs()

	if groups := sc.GetSupplementalGroups(); len(groups) > 0 {
		note(fmt.Sprintf("supplemental-groups=%v (no supplementary-group plumbing in the start paths)", groups))
	}
	if sc.GetSupplementalGroupsPolicy() == v1.SupplementalGroupsPolicy_Strict {
		note("supplemental-groups-policy=Strict (advertised as unsupported in Status.Features)")
	}
	if sc.GetNoNewPrivs() {
		out.securityOpt = append(out.securityOpt, "no-new-privileges")
		note("no-new-privs (enforced via re-exec shim in native mode only)")
	}
	for _, p := range sc.GetMaskedPaths() {
		note("masked-path=" + p)
	}
	for _, p := range sc.GetReadonlyPaths() {
		note("readonly-path=" + p)
	}

	// Per spec, privileged mode implies seccomp/apparmor restrictions are not
	// applied, so requested profiles are skipped rather than recorded.
	if !out.privileged {
		if p := sc.GetSeccomp(); p != nil {
			spec, err := seccompProfileSpec(p)
			if err != nil {
				return nil, err
			}
			switch spec {
			case "", "unconfined":
				// Nothing to enforce.
			case "default":
				out.securityOpt = append(out.securityOpt, "seccomp=default")
				note("seccomp=runtime-default (enforced via shim in native mode only; see Status.Info)")
			default:
				out.securityOpt = append(out.securityOpt, "seccomp="+spec)
			}
		}
		if p := sc.GetApparmor(); p != nil {
			spec, err := apparmorProfileSpec(p)
			if err != nil {
				return nil, err
			}
			if spec != "" && spec != "unconfined" {
				out.securityOpt = append(out.securityOpt, "apparmor="+spec)
			} else {
				out.unconfinedApp = true
			}
		}
	}

	if len(unenforced) > 0 {
		slog.Warn("cri: security requests accepted best-effort (not enforced)",
			"container", name, "unenforced", strings.Join(unenforced, ","))
		ann := cfg.GetAnnotations()
		if ann == nil {
			ann = make(map[string]string)
			cfg.Annotations = ann
		}
		if prev, ok := ann[unenforcedAnnotation]; ok && prev != "" {
			ann[unenforcedAnnotation] = prev + "," + strings.Join(unenforced, ",")
		} else {
			ann[unenforcedAnnotation] = strings.Join(unenforced, ",")
		}
	}
	return out, nil
}

// seccompProfileSpec maps a CRI seccomp SecurityProfile onto the runtime's
// "seccomp=<spec>" vocabulary: RuntimeDefault -> "default" (the runtime's
// builtin default, enforced via the shim in native mode), Localhost -> the
// profile file path (must be absolute per spec), Unconfined -> "unconfined".
func seccompProfileSpec(p *v1.SecurityProfile) (string, error) {
	switch p.GetProfileType() {
	case v1.SecurityProfile_Unconfined:
		return "unconfined", nil
	case v1.SecurityProfile_Localhost:
		ref := p.GetLocalhostRef()
		if ref == "" {
			return "", status.Errorf(codes.InvalidArgument, "seccomp localhost profile requires localhost_ref")
		}
		if !strings.HasPrefix(ref, "/") {
			return "", status.Errorf(codes.InvalidArgument,
				"seccomp localhost profile must be an absolute path (got %q)", ref)
		}
		return ref, nil
	default: // RuntimeDefault (zero value) and any future default-ish type.
		return "default", nil
	}
}

// apparmorProfileSpec maps a CRI AppArmor SecurityProfile onto the runtime's
// "apparmor=<spec>" vocabulary: RuntimeDefault -> "runtime/default",
// Localhost -> the profile name, Unconfined -> "unconfined".
func apparmorProfileSpec(p *v1.SecurityProfile) (string, error) {
	switch p.GetProfileType() {
	case v1.SecurityProfile_Unconfined:
		return "unconfined", nil
	case v1.SecurityProfile_Localhost:
		ref := p.GetLocalhostRef()
		if ref == "" {
			return "", status.Errorf(codes.InvalidArgument, "apparmor localhost profile requires localhost_ref (profile name)")
		}
		return ref, nil
	default:
		return "runtime/default", nil
	}
}
