package cri

import (
	"strings"
	"testing"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func testConfig(sc *v1.LinuxContainerSecurityContext) *v1.ContainerConfig {
	return &v1.ContainerConfig{
		Metadata: &v1.ContainerMetadata{Name: "test"},
		Linux:    &v1.LinuxContainerConfig{SecurityContext: sc},
	}
}

func TestApplySecurityContextNil(t *testing.T) {
	out, err := applySecurityContext(&v1.ContainerConfig{Metadata: &v1.ContainerMetadata{Name: "x"}})
	if err != nil {
		t.Fatalf("nil context: %v", err)
	}
	if out.privileged || out.readOnly || len(out.securityOpt) != 0 {
		t.Fatalf("nil context must map to empty settings: %+v", out)
	}
}

func TestApplySecurityContextCapsUserReadonly(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{
		Capabilities:   &v1.Capability{AddCapabilities: []string{"NET_ADMIN"}, DropCapabilities: []string{"MKNOD"}},
		RunAsUser:      &v1.Int64Value{Value: 1000},
		RunAsGroup:     &v1.Int64Value{Value: 100},
		ReadonlyRootfs: true,
	})
	out, err := applySecurityContext(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.capAdd) != 1 || out.capAdd[0] != "NET_ADMIN" {
		t.Fatalf("capAdd = %v", out.capAdd)
	}
	if len(out.capDrop) != 1 || out.capDrop[0] != "MKNOD" {
		t.Fatalf("capDrop = %v", out.capDrop)
	}
	if out.user != "1000:100" {
		t.Fatalf("user = %q", out.user)
	}
	if !out.readOnly {
		t.Fatal("readOnly must be true")
	}
}

func TestApplySecurityContextSeccompProfiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		prof *v1.SecurityProfile
		want string // expected seccomp= entry, "" = none
	}{
		{"default", &v1.SecurityProfile{ProfileType: v1.SecurityProfile_RuntimeDefault}, "seccomp=default"},
		{"localhost", &v1.SecurityProfile{ProfileType: v1.SecurityProfile_Localhost, LocalhostRef: "/profiles/p.json"}, "seccomp=/profiles/p.json"},
		{"unconfined", &v1.SecurityProfile{ProfileType: v1.SecurityProfile_Unconfined}, ""},
	} {
		cfg := testConfig(&v1.LinuxContainerSecurityContext{Seccomp: tc.prof})
		out, err := applySecurityContext(cfg)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		found := ""
		for _, o := range out.securityOpt {
			if strings.HasPrefix(o, "seccomp=") {
				found = o
			}
		}
		if found != tc.want {
			t.Fatalf("%s: seccomp opt = %q, want %q", tc.name, found, tc.want)
		}
	}
}

func TestApplySecurityContextSeccompRelativePathRejected(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{
		Seccomp: &v1.SecurityProfile{ProfileType: v1.SecurityProfile_Localhost, LocalhostRef: "relative/p.json"},
	})
	if _, err := applySecurityContext(cfg); err == nil {
		t.Fatal("relative localhost_ref must be rejected")
	}
}

func TestApplySecurityContextSELinuxRejected(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{
		SelinuxOptions: &v1.SELinuxOption{User: "u", Role: "r", Type: "t", Level: "s0"},
	})
	if _, err := applySecurityContext(cfg); err == nil {
		t.Fatal("SELinux options must fail honestly (no enforcement path)")
	}
}

func TestApplySecurityContextAmbientCapsRejected(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{
		Capabilities: &v1.Capability{AddAmbientCapabilities: []string{"NET_ADMIN"}},
	})
	if _, err := applySecurityContext(cfg); err == nil {
		t.Fatal("ambient capabilities must fail honestly")
	}
}

func TestApplySecurityContextGroupWithoutUserRejected(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{
		RunAsGroup: &v1.Int64Value{Value: 100},
	})
	if _, err := applySecurityContext(cfg); err == nil {
		t.Fatal("run_as_group without run_as_user must fail per CRI spec")
	}
}

func TestApplySecurityContextUsernameRejected(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{RunAsUsername: "nobody"})
	if _, err := applySecurityContext(cfg); err == nil {
		t.Fatal("run_as_username must fail honestly (no image passwd resolution)")
	}
}

func TestApplySecurityContextPrivilegedSkipsProfiles(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{
		Privileged: true,
		Seccomp:    &v1.SecurityProfile{ProfileType: v1.SecurityProfile_Localhost, LocalhostRef: "/p.json"},
		Apparmor:   &v1.SecurityProfile{ProfileType: v1.SecurityProfile_Localhost, LocalhostRef: "prof"},
	})
	out, err := applySecurityContext(cfg)
	if err != nil {
		t.Fatalf("privileged must not fail on profiles: %v", err)
	}
	if !out.privileged {
		t.Fatal("privileged must propagate")
	}
	for _, o := range out.securityOpt {
		if strings.HasPrefix(o, "seccomp=") || strings.HasPrefix(o, "apparmor=") {
			t.Fatalf("privileged must skip profiles, got %q", o)
		}
	}
}

func TestApplySecurityContextUnenforcedAnnotated(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{
		SupplementalGroups:       []int64{1000},
		MaskedPaths:              []string{"/proc/kcore"},
		ReadonlyPaths:            []string{"/proc/sys"},
		NoNewPrivs:               true,
		SupplementalGroupsPolicy: v1.SupplementalGroupsPolicy_Strict,
		NamespaceOptions:         &v1.NamespaceOption{Pid: v1.NamespaceMode_CONTAINER},
	})
	out, err := applySecurityContext(cfg)
	if err != nil {
		t.Fatalf("best-effort fields must not fail: %v", err)
	}
	ann := cfg.GetAnnotations()[unenforcedAnnotation]
	for _, want := range []string{"supplemental-groups", "masked-path", "readonly-path", "no-new-privs", "Strict", "pid-namespace"} {
		if !strings.Contains(ann, want) {
			t.Fatalf("annotation %q missing %q", ann, want)
		}
	}
	foundNNP := false
	for _, o := range out.securityOpt {
		if o == "no-new-privileges" {
			foundNNP = true
		}
	}
	if !foundNNP {
		t.Fatal("noNewPrivs must map to no-new-privileges SecurityOpt")
	}
}

func TestApplySecurityContextHostNetwork(t *testing.T) {
	cfg := testConfig(&v1.LinuxContainerSecurityContext{
		NamespaceOptions: &v1.NamespaceOption{Network: v1.NamespaceMode_NODE},
	})
	out, err := applySecurityContext(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.hostNetwork {
		t.Fatal("network=NODE must map to host network")
	}
}
