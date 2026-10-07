package namespaces

import "testing"

func TestCapabilitiesArch(t *testing.T) {
	r := New(t.TempDir())
	archs := map[string]bool{}
	for _, a := range r.Capabilities().Arch {
		archs[a] = true
	}
	for _, want := range []string{"arm64", "armv7", "amd64", "386"} {
		if !archs[want] {
			t.Errorf("Capabilities().Arch missing %q, got %v", want, r.Capabilities().Arch)
		}
	}
}
