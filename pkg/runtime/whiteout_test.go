package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractTarGzWhiteoutFile(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	// Create a regular file first, then a whiteout file.
	origFile := filepath.Join(rootfsDir, "deleted.txt")
	os.WriteFile(origFile, []byte("this will be deleted"), 0644)

	// Create a tar.gz with a .wh.deleted.txt entry.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Whiteout entry for deleted.txt.
	tw.WriteHeader(&tar.Header{
		Name:     ".wh.deleted.txt",
		Typeflag: tar.TypeReg,
		Size:     0,
		Mode:     0644,
	})
	tw.Close()

	// Write as gzip.
	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "layer.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	if _, err := os.Stat(origFile); !os.IsNotExist(err) {
		t.Error("whiteout target should be deleted")
	}
}

func TestExtractTarGzWhiteoutInSubDir(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	subDir := filepath.Join(rootfsDir, "etc", "conf")
	os.MkdirAll(subDir, 0755)
	deletedFile := filepath.Join(subDir, "settings.conf")
	os.WriteFile(deletedFile, []byte("settings"), 0644)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	tw.WriteHeader(&tar.Header{
		Name:     "etc/conf/.wh.settings.conf",
		Typeflag: tar.TypeReg,
		Size:     0,
		Mode:     0644,
	})
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "layer.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	if _, err := os.Stat(deletedFile); !os.IsNotExist(err) {
		t.Error("whiteout target in subdirectory should be deleted")
	}
}

func TestExtractTarGzOpaqueWhiteout(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	tw.WriteHeader(&tar.Header{
		Name:     "usr/lib/.wh..wh..opq",
		Typeflag: tar.TypeReg,
		Size:     0,
		Mode:     0644,
	})
	tw.WriteHeader(&tar.Header{
		Name:     "usr/lib/kept.so",
		Typeflag: tar.TypeReg,
		Size:     3,
		Mode:     0644,
	})
	tw.Write([]byte("lib"))
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "layer.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	// Opaque whiteout doesn't delete files, only marks directory cleanup.
	// The kept.so should still be extracted.
	keptPath := filepath.Join(rootfsDir, "usr", "lib", "kept.so")
	if _, err := os.Stat(keptPath); err != nil {
		t.Errorf("kept.so should exist: %v", err)
	}
}

func TestExtractTarGzPathTraversal(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	tw.WriteHeader(&tar.Header{
		Name:     "../../../etc/passwd",
		Typeflag: tar.TypeReg,
		Size:     0,
		Mode:     0644,
	})
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "traverse.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("error should mention 'path traversal': %v", err)
	}
}

func TestExtractTarGzSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	tw.WriteHeader(&tar.Header{
		Name:     "escape_link",
		Typeflag: tar.TypeSymlink,
		Linkname: "../../etc/shadow",
		Mode:     0777,
	})
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "symlink.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err == nil {
		t.Fatal("expected error for symlink escape")
	}
	if !strings.Contains(err.Error(), "symlink escape") {
		t.Errorf("error should mention 'symlink escape': %v", err)
	}
}

func TestExtractTarGzAbsoluteSymlink(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Absolute symlinks to /usr/bin are allowed (inside container).
	tw.WriteHeader(&tar.Header{
		Name:     "bin/python",
		Typeflag: tar.TypeSymlink,
		Linkname: "/usr/bin/python3",
		Mode:     0777,
	})
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "abs_symlink.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("absolute symlinks should be allowed: %v", err)
	}

	linkPath := filepath.Join(rootfsDir, "bin", "python")
	fi, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink")
	}
}

func TestExtractTarGzRegularFile(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{
		Name:     "hello.txt",
		Typeflag: tar.TypeReg,
		Size:     13,
		Mode:     0644,
	})
	tw.Write([]byte("Hello, World!"))
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "regular.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rootfsDir, "hello.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "Hello, World!" {
		t.Errorf("content = %q, want 'Hello, World!'", string(data))
	}
}

func TestExtractTarGzDirectory(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{
		Name:     "usr/local/bin/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	})
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "dir.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	fi, err := os.Stat(filepath.Join(rootfsDir, "usr", "local", "bin"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !fi.IsDir() {
		t.Error("expected directory")
	}
}

func TestExtractTarGzMissingFile(t *testing.T) {
	dir := t.TempDir()
	err := extractTarGz(filepath.Join(dir, "nonexistent.tar.gz"), dir)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestExtractTarGzEmptyTar(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "empty.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extract empty tar: %v", err)
	}
}

func TestExtractTarGzUncompressed(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{
		Name:     "raw.txt",
		Typeflag: tar.TypeReg,
		Size:     4,
		Mode:     0644,
	})
	tw.Write([]byte("raw!"))
	tw.Close()

	tarPath := filepath.Join(dir, "uncompressed.tar")
	os.WriteFile(tarPath, buf.Bytes(), 0644)

	err := extractTarGz(tarPath, rootfsDir)
	if err != nil {
		t.Fatalf("extract uncompressed tar: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(rootfsDir, "raw.txt"))
	if string(data) != "raw!" {
		t.Errorf("content = %q, want raw!", string(data))
	}
}

func TestExtractTarGzMultipleWhiteouts(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	// Create files that will be whiteout'd.
	os.MkdirAll(filepath.Join(rootfsDir, "app"), 0755)
	os.WriteFile(filepath.Join(rootfsDir, "app", "old.js"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(rootfsDir, "app", "old.css"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(rootfsDir, "app", "keep.html"), []byte("keep"), 0644)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{
		Name:     "app/.wh.old.js",
		Typeflag: tar.TypeReg,
		Size:     0, Mode: 0644,
	})
	tw.WriteHeader(&tar.Header{
		Name:     "app/.wh.old.css",
		Typeflag: tar.TypeReg,
		Size:     0, Mode: 0644,
	})
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "multi.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootfsDir, "app", "old.js")); !os.IsNotExist(err) {
		t.Error("old.js should be deleted")
	}
	if _, err := os.Stat(filepath.Join(rootfsDir, "app", "old.css")); !os.IsNotExist(err) {
		t.Error("old.css should be deleted")
	}
	if _, err := os.Stat(filepath.Join(rootfsDir, "app", "keep.html")); err != nil {
		t.Error("keep.html should still exist")
	}
}

func TestExtractTarGzDotEntry(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{
		Name:     ".",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	})
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "dot.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extract tar with '.' entry: %v", err)
	}
}

func TestExtractTarGzDotSlashEntry(t *testing.T) {
	dir := t.TempDir()
	rootfsDir := filepath.Join(dir, "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{
		Name:     "./",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	})
	tw.Close()

	gzData := gzipCompress(buf.Bytes())
	gzPath := filepath.Join(dir, "dotslash.tar.gz")
	os.WriteFile(gzPath, gzData, 0644)

	err := extractTarGz(gzPath, rootfsDir)
	if err != nil {
		t.Fatalf("extract tar with './' entry: %v", err)
	}
}

func gzipCompress(data []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(data)
	gw.Close()
	return buf.Bytes()
}

func TestParseSignal(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"SIGTERM", 15},
		{"SIGKILL", 9},
		{"SIGINT", 2},
		{"SIGHUP", 1},
		{"UNKNOWN", 15},
	}
	for _, tt := range tests {
		got := parseSignal(tt.input)
		if int(got) != tt.want {
			t.Errorf("parseSignal(%q) = %d, want %d", tt.input, int(got), tt.want)
		}
	}
}
