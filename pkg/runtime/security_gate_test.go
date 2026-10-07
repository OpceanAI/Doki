package runtime

import (
	"testing"

	"github.com/OpceanAI/Doki/internal/seccomp"
)

// The gate must be honest in every execution mode: empty or unconfined
// requests always pass, "default" never fails (warn+annotate path), explicit
// seccomp profiles pass only in native mode on a supporting kernel, and any
// AppArmor confinement fails (no pre-exec apply path exists).
func TestCheckSecurityRequests(t *testing.T) {
	modes := []ExecutionMode{ModeNative, ModeProot, ModeNamespaces, ModeMicroVM}
	for _, m := range modes {
		if err := checkSecurityRequests(m, nil); err != nil {
			t.Fatalf("mode %q: empty opts must pass: %v", m, err)
		}
		if err := checkSecurityRequests(m, []string{"seccomp=unconfined", "apparmor=unconfined"}); err != nil {
			t.Fatalf("mode %q: unconfined must pass: %v", m, err)
		}
		if err := checkSecurityRequests(m, []string{"seccomp=default"}); err != nil {
			t.Fatalf("mode %q: default must not fail: %v", m, err)
		}
		if err := checkSecurityRequests(m, []string{"apparmor=runtime/default"}); err == nil {
			t.Fatalf("mode %q: apparmor confinement must fail honestly", m)
		}
	}

	if err := checkSecurityRequests(ModeProot, []string{"seccomp=/profiles/p.json"}); err == nil {
		t.Fatal("explicit seccomp profile in proot mode must fail (no enforcement there)")
	}
	if err := checkSecurityRequests(ModeNamespaces, []string{"seccomp=/profiles/p.json"}); err == nil {
		t.Fatal("explicit seccomp profile in namespaces mode must fail (no enforcement there)")
	}

	err := checkSecurityRequests(ModeNative, []string{"seccomp=/profiles/p.json"})
	if seccomp.Supported() && err != nil {
		t.Fatalf("native+explicit profile on supporting kernel must pass: %v", err)
	}
	if !seccomp.Supported() && err == nil {
		t.Fatal("native+explicit profile on non-supporting kernel must fail")
	}
}

func TestApparmorSpecParsing(t *testing.T) {
	if got := apparmorSpec(nil); got != "" {
		t.Fatalf("empty opts: %q", got)
	}
	if got := apparmorSpec([]string{"seccomp=default", "apparmor=my-profile"}); got != "my-profile" {
		t.Fatalf("apparmor spec = %q", got)
	}
	if hasNoNewPrivs([]string{"no-new-privileges"}) != true {
		t.Fatal("bare no-new-privileges must be detected")
	}
	if hasNoNewPrivs([]string{"seccomp=default"}) != false {
		t.Fatal("no-new-privs must not be detected when absent")
	}
}
