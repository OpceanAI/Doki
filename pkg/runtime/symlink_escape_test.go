package runtime

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeTarGz writes the given headers+bodies to a gzip-compressed tar at path.
func writeTarGz(t *testing.T, path string, entries []struct {
	hdr  *tar.Header
	body []byte
}) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		if err := tw.WriteHeader(e.hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if len(e.body) > 0 {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatalf("write body: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	// extractTarGz sniffs magic bytes; a raw tar (no gzip) is fine too.
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write tar file: %v", err)
	}
}

// TestExtractTarGzAbsoluteSymlinkParentEscape verifies that a malicious layer
// cannot escape the extraction root by creating an absolute symlink and then
// writing a regular file *through* that symlink (CVE-2018-15664 class).
func TestExtractTarGzAbsoluteSymlinkParentEscape(t *testing.T) {
	tmp := t.TempDir()
	// The "host" area the attacker wants to reach — a sibling of the rootfs.
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "pwned")

	rootfs := filepath.Join(tmp, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		t.Fatal(err)
	}

	tarPath := filepath.Join(tmp, "layer.tar")
	writeTarGz(t, tarPath, []struct {
		hdr  *tar.Header
		body []byte
	}{
		// 1) absolute symlink "escape" -> <outside dir> (host absolute path)
		{&tar.Header{Name: "escape", Typeflag: tar.TypeSymlink, Linkname: outside, Mode: 0777}, nil},
		// 2) regular file written *through* the symlink
		{&tar.Header{Name: "escape/pwned", Typeflag: tar.TypeReg, Mode: 0644, Size: 5}, []byte("hello")},
	})

	// Extraction may error (that's fine); what matters is no host write escaped.
	_ = extractTarGz(tarPath, rootfs)

	if _, err := os.Lstat(sentinel); err == nil {
		t.Fatalf("SECURITY: file escaped extraction root and was written to %s", sentinel)
	}
}

// TestExtractTarGzRelativeSymlinkParentEscape covers the relative-symlink variant.
func TestExtractTarGzRelativeSymlinkParentEscape(t *testing.T) {
	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "pwned")

	rootfs := filepath.Join(tmp, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		t.Fatal(err)
	}

	tarPath := filepath.Join(tmp, "layer.tar")
	writeTarGz(t, tarPath, []struct {
		hdr  *tar.Header
		body []byte
	}{
		// relative symlink climbing out of rootfs into ../outside
		{&tar.Header{Name: "escape", Typeflag: tar.TypeSymlink, Linkname: "../outside", Mode: 0777}, nil},
		{&tar.Header{Name: "escape/pwned", Typeflag: tar.TypeReg, Mode: 0644, Size: 5}, []byte("hello")},
	})

	_ = extractTarGz(tarPath, rootfs)

	if _, err := os.Lstat(sentinel); err == nil {
		t.Fatalf("SECURITY: file escaped extraction root via relative symlink to %s", sentinel)
	}
}
