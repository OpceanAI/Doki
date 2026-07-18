package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecureJoinClampsAbsoluteSymlink(t *testing.T) {
	root := t.TempDir()
	// "link" is an absolute symlink pointing at the host /etc.
	if err := os.Symlink("/etc", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := SecureJoin(root, "link/passwd")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "etc", "passwd")
	if got != want {
		t.Fatalf("absolute symlink not clamped: got %q want %q", got, want)
	}
}

func TestSecureJoinClampsRelativeSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("../../../../etc", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := SecureJoin(root, "link/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if !isWithin(got, root) {
		t.Fatalf("relative symlink escaped root: %q not within %q", got, root)
	}
}

func TestSecureJoinClampsDotDot(t *testing.T) {
	root := t.TempDir()
	got, err := SecureJoin(root, "a/../../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if !isWithin(got, root) {
		t.Fatalf(".. escaped root: %q not within %q", got, root)
	}
}

func TestSecureJoinNonexistentTail(t *testing.T) {
	root := t.TempDir()
	got, err := SecureJoin(root, "does/not/exist/yet")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "does", "not", "exist", "yet")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func isWithin(path, root string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	return path == root || len(path) > len(root) && path[:len(root)+1] == root+string(os.PathSeparator)
}
