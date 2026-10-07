package storage

import (
	"os"
	"testing"

	"github.com/OpceanAI/Doki/pkg/common"
)

func TestCanUseOverlay2Rootless(t *testing.T) {
	// Rootless can never use kernel overlay2 (no mount privs, no modprobe).
	if os.Geteuid() != 0 && canUseOverlay2() {
		t.Fatal("canUseOverlay2() = true as non-root, want false")
	}
	// Termux can never use kernel overlay2 either.
	if common.IsTermux() && canUseOverlay2() {
		t.Fatal("canUseOverlay2() = true on Termux, want false")
	}
}

func TestDetectBestDriverRootlessNoOverlay2(t *testing.T) {
	if os.Geteuid() != 0 {
		if got := DetectBestDriver(t.TempDir()); got == DriverOverlay2 {
			t.Fatalf("DetectBestDriver() = overlay2 as non-root, want fuse-overlayfs/vfs fallback")
		}
	}
}
