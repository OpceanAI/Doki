package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveContainerPathClampsSymlink verifies CRIT-3: a symlink planted
// inside the container rootfs (e.g. "/escape -> /") cannot redirect docker cp
// onto the host filesystem. resolveContainerPath must clamp every result to the
// rootfs.
func TestResolveContainerPathClampsSymlink(t *testing.T) {
	rootfs := t.TempDir()
	// Plant "escape -> /" inside the rootfs.
	if err := os.Symlink("/", filepath.Join(rootfs, "escape")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	// Also create a sensitive host file to try to reach.
	hostSecret := filepath.Join(t.TempDir(), "shadow")
	_ = os.WriteFile(hostSecret, []byte("secret"), 0600)

	cases := []string{
		"/escape/etc/shadow",
		"/escape" + hostSecret,
		"/../../../../etc/shadow",
		"escape/../../../etc/passwd",
	}
	for _, in := range cases {
		got, err := resolveContainerPath(rootfs, in)
		if err != nil {
			continue // rejected outright is fine
		}
		if !strings.HasPrefix(got, filepath.Clean(rootfs)) {
			t.Errorf("path %q escaped rootfs: got %q", in, got)
		}
	}
}

// TestResolveContainerPathEmptyRootfs verifies MED-1: without a rootfs the
// resolver must refuse rather than operate on the host root.
func TestResolveContainerPathEmptyRootfs(t *testing.T) {
	if _, err := resolveContainerPath("", "/etc/shadow"); err == nil {
		t.Fatal("expected error for empty rootfs")
	}
}
