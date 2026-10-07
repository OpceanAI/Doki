package legacy32

import (
	"testing"

	"github.com/OpceanAI/Doki/internal/proot"
)

func TestQEMUArchName(t *testing.T) {
	cases := map[string]string{
		"armv7": "arm", "armv6": "arm", "arm": "arm",
		"386": "i386", "i386": "i386",
		"arm64": "aarch64", "aarch64": "aarch64",
		"amd64": "x86_64",
	}
	for in, want := range cases {
		if got := qemuArchName(in); got != want {
			t.Errorf("qemuArchName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIs32Bit(t *testing.T) {
	for _, a := range []string{"armv7", "arm", "386", "i386"} {
		if !is32Bit(a) {
			t.Errorf("is32Bit(%q) = false, want true", a)
		}
	}
	for _, a := range []string{"arm64", "amd64"} {
		if is32Bit(a) {
			t.Errorf("is32Bit(%q) = true, want false", a)
		}
	}
}

func TestDetectHonest(t *testing.T) {
	r := New(t.TempDir())
	// Detect must agree with its inputs: compat || qemu || binfmt || proot.
	want := r.canCompat || haveQEMUEmulator() ||
		binfmtRegistered("arm") || binfmtRegistered("i386") ||
		proot.IsAvailable()
	if got := r.Detect(); got != want {
		t.Errorf("Detect() = %v, want %v", got, want)
	}
}
