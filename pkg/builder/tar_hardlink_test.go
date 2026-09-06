package builder

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestExtractTarHardlinkFallbackToCopy verifies that ExtractTar falls back to
// copying the linked file's content when os.Link fails for an OS/filesystem
// reason (e.g. EPERM/EXDEV on filesystems that don't support hardlinks, such
// as Android's /data partition under Termux), instead of failing the build.
func TestExtractTarHardlinkFallbackToCopy(t *testing.T) {
	dest := t.TempDir()

	const (
		srcName = "usr/bin/perl"
		lnkName = "usr/bin/perl5.36.0"
		content = "#!/usr/bin/env perl-binary-content\n"
	)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	headers := []*tar.Header{
		{Name: "usr/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: "usr/bin/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: srcName, Typeflag: tar.TypeReg, Mode: 0755, Size: int64(len(content))},
		{Name: lnkName, Typeflag: tar.TypeLink, Linkname: srcName},
	}
	for _, h := range headers {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Name == srcName {
			if _, err := tw.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate a filesystem/OS that rejects hardlink creation (e.g. EPERM/EXDEV).
	orig := osLink
	osLink = func(oldname, newname string) error {
		return errors.New("simulated: hardlink not supported on this filesystem")
	}
	defer func() { osLink = orig }()

	if err := ExtractTar(&buf, dest); err != nil {
		t.Fatalf("ExtractTar failed, expected fallback-to-copy to succeed: %v", err)
	}

	linkPath := filepath.Join(dest, lnkName)
	data, err := os.ReadFile(linkPath)
	if err != nil {
		t.Fatalf("expected fallback copy to create %s: %v", linkPath, err)
	}
	if string(data) != content {
		t.Fatalf("fallback copy content mismatch: got %q, want %q", data, content)
	}
}

// TestExtractTarHardlinkFallbackBothFail verifies that ExtractTar still
// returns an error when both the hardlink creation AND the fallback copy
// fail (e.g. the link target doesn't actually exist).
func TestExtractTarHardlinkFallbackBothFail(t *testing.T) {
	dest := t.TempDir()

	const lnkName = "usr/bin/missing-link"

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	headers := []*tar.Header{
		{Name: "usr/", Typeflag: tar.TypeDir, Mode: 0755},
		{Name: "usr/bin/", Typeflag: tar.TypeDir, Mode: 0755},
		// Linkname points to a file that was never written into the tar/dest.
		{Name: lnkName, Typeflag: tar.TypeLink, Linkname: "usr/bin/does-not-exist"},
	}
	for _, h := range headers {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	orig := osLink
	osLink = func(oldname, newname string) error {
		return errors.New("simulated: hardlink not supported on this filesystem")
	}
	defer func() { osLink = orig }()

	if err := ExtractTar(&buf, dest); err == nil {
		t.Fatal("expected ExtractTar to fail when both hardlink and fallback copy fail")
	}
}

// TestExtractTarHardlinkTraversalStillBlocked verifies the existing
// hardlink-escape/traversal security check remains intact regardless of the
// fallback-to-copy behavior.
func TestExtractTarHardlinkTraversalStillBlocked(t *testing.T) {
	dest := t.TempDir()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	headers := []*tar.Header{
		{Name: "link", Typeflag: tar.TypeLink, Linkname: "../../etc/passwd"},
	}
	for _, h := range headers {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	err := ExtractTar(&buf, dest)
	if err == nil {
		t.Fatal("expected hardlink traversal to be blocked")
	}
}
