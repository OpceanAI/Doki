package runtime

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

// writeTar builds a raw (uncompressed) tar. extractTarGz sniffs magic bytes, so
// a plain tar is accepted — see symlink_escape_test.go.
func writeTar(t *testing.T, entries []*tar.Header, bodies map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "layer.tar")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	tw := tar.NewWriter(f)
	for _, h := range entries {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if body, ok := bodies[h.Name]; ok {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// A tar directory entry is named WITH a trailing slash. filepath.Dir/Base do not
// strip it, so splitting the raw name puts the entry one level too deep with its
// own basename duplicated ("a/b/" -> "a/b/b").
//
// That was mostly invisible — it littered the rootfs with unused nested
// directories — until a FILE shares its parent directory's name. npm ships
// exactly that shape: node-gyp/gyp/ (directory) containing gyp (file). The
// directory entry created .../gyp/gyp as a directory, then the file entry
// resolved to the same path and os.Create returned EISDIR:
//
//	extract layers: open .../node-gyp/gyp/gyp: is a directory
//
// This is the minimal reproduction of that, with no external fixtures.
func TestExtractTarGzFileSharingParentDirName(t *testing.T) {
	dir := "pkg/gyp/"
	file := "pkg/gyp/gyp"
	tarPath := writeTar(t,
		[]*tar.Header{
			{Name: "pkg/", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: dir, Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: file, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("#!/bin/sh\n"))},
		},
		map[string]string{file: "#!/bin/sh\n"},
	)

	rootfs := filepath.Join(t.TempDir(), "rootfs")
	if err := os.MkdirAll(rootfs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(tarPath, rootfs); err != nil {
		t.Fatalf("extract failed (this is the bug: os.Create hits EISDIR): %v", err)
	}

	// The file must exist AS A FILE at exactly the path the tar named.
	got := filepath.Join(rootfs, file)
	fi, err := os.Lstat(got)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("%s should be a regular file, got mode %v", file, fi.Mode())
	}
	body, err := os.ReadFile(got)
	if err != nil || string(body) != "#!/bin/sh\n" {
		t.Fatalf("content mismatch: %q err=%v", string(body), err)
	}

	// And the duplicated-nesting artefact must not exist.
	if _, err := os.Lstat(filepath.Join(rootfs, "pkg/gyp/gyp/gyp")); err == nil {
		t.Error("pkg/gyp/gyp/gyp exists: directory entries are still extracted one level too deep")
	}
}

// The same defect with no name collision — this is the silent half, and it is why
// alpine and nginx appeared to extract correctly while every directory's mode,
// owner and mtime were being applied to the wrong path.
func TestExtractTarGzDirEntryIsNotDuplicated(t *testing.T) {
	tarPath := writeTar(t,
		[]*tar.Header{
			{Name: "usr/", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: "usr/local/", Typeflag: tar.TypeDir, Mode: 0o700},
		},
		nil,
	)

	rootfs := filepath.Join(t.TempDir(), "rootfs")
	if err := os.MkdirAll(rootfs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(tarPath, rootfs); err != nil {
		t.Fatal(err)
	}

	for _, stray := range []string{"usr/usr", "usr/local/local"} {
		if _, err := os.Lstat(filepath.Join(rootfs, stray)); err == nil {
			t.Errorf("stray duplicated directory %q was created", stray)
		}
	}
	// The real directory must exist, and carry ITS OWN mode rather than a default
	// applied to the wrong path.
	fi, err := os.Lstat(filepath.Join(rootfs, "usr/local"))
	if err != nil {
		t.Fatalf("usr/local missing: %v", err)
	}
	if !fi.IsDir() {
		t.Fatal("usr/local is not a directory")
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("usr/local mode = %o, want 700 (the tar's own mode)", perm)
	}
}

// A layer may legitimately replace a directory with a file. os.Create cannot
// overwrite a directory, so the extractor must remove the stale entry — it
// previously removed only symlinks.
func TestExtractTarGzReplacesDirectoryWithFile(t *testing.T) {
	rootfs := filepath.Join(t.TempDir(), "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfs, "swap/inner"), 0o755); err != nil {
		t.Fatal(err)
	}

	tarPath := writeTar(t,
		[]*tar.Header{{Name: "swap", Typeflag: tar.TypeReg, Mode: 0o644, Size: 2}},
		map[string]string{"swap": "hi"},
	)
	if err := extractTarGz(tarPath, rootfs); err != nil {
		t.Fatalf("replacing a directory with a file failed: %v", err)
	}
	fi, err := os.Lstat(filepath.Join(rootfs, "swap"))
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("swap should now be a regular file, got %v", fi.Mode())
	}
}

// A directory may legitimately be READ-ONLY and still contain entries. Nix store
// paths are 0555 and supabase/postgres is built from them, so this is not exotic.
//
// Applying a directory's mode the moment its entry is read makes every later
// entry inside it fail:
//
//	mkdir .../nix/store/…-nix-fetchers-2.26.2/lib: permission denied
//
// Directory modes must therefore be applied AFTER the whole archive, deepest
// first — which is what Docker and containerd do.
func TestExtractTarGzReadOnlyDirectoryWithContents(t *testing.T) {
	tarPath := writeTar(t,
		[]*tar.Header{
			{Name: "store/", Typeflag: tar.TypeDir, Mode: 0o555},
			{Name: "store/lib/", Typeflag: tar.TypeDir, Mode: 0o555},
			{Name: "store/lib/libfoo.so", Typeflag: tar.TypeReg, Mode: 0o444, Size: 3},
		},
		map[string]string{"store/lib/libfoo.so": "ELF"},
	)

	rootfs := filepath.Join(t.TempDir(), "rootfs")
	if err := os.MkdirAll(rootfs, 0o755); err != nil {
		t.Fatal(err)
	}
	// The extracted tree is deliberately read-only, which t.TempDir's RemoveAll
	// cannot delete when the test runs as an unprivileged user. Restore write
	// permission first so cleanup does not fail the test for the wrong reason.
	t.Cleanup(func() {
		_ = filepath.WalkDir(rootfs, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(p, 0o755)
			}
			return nil
		})
	})

	// NOTE: this only reproduces as a NON-ROOT user. root holds CAP_DAC_OVERRIDE
	// and writes into a 0555 directory regardless, so as root the assertion below
	// passes even on the broken code. dokid on Android runs as the Termux app
	// user, which is where it was found. Verified both ways with `docker run
	// --user 1000:1000`: inline chmod fails here with
	//   mkdir .../store/lib: permission denied
	if err := extractTarGz(tarPath, rootfs); err != nil {
		t.Fatalf("read-only directory with contents failed to extract: %v", err)
	}

	// The file landed...
	if body, err := os.ReadFile(filepath.Join(rootfs, "store/lib/libfoo.so")); err != nil || string(body) != "ELF" {
		t.Fatalf("libfoo.so: %q err=%v", string(body), err)
	}
	// ...and the directories still ended up read-only, so deferring the mode did
	// not quietly discard it.
	for _, d := range []string{"store", "store/lib"} {
		fi, err := os.Lstat(filepath.Join(rootfs, d))
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		if perm := fi.Mode().Perm(); perm != 0o555 {
			t.Errorf("%s mode = %o, want 555", d, perm)
		}
	}
}
