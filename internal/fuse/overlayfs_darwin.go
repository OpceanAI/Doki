//go:build darwin
// +build darwin

// Package fuse is a no-op stub on macOS. The mount/overlayfs operations it
// would normally perform on Linux are not available on Darwin, and ModeNative
// is the only execution mode supported on macOS (no isolation is required for
// it to function). Every function in this file returns an error or does the
// minimal work that makes sense without root privileges and kernel namespaces.
package fuse

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// CopyDir recursively copies src to dst. This is a simple Go-level copy used
// for VFS storage driver (the only driver available on macOS).
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = copyBuf(in, out)
	return err
}

// PrepareRootfs is a no-op on macOS.
func PrepareRootfs(rootfsDir string, files map[string][]byte, user string) error {
	for name, content := range files {
		path := filepath.Join(rootfsDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// GenerateHostname returns a hostname string.
func GenerateHostname() string { return "doki" }

// GenerateHosts returns a minimal /etc/hosts content.
func GenerateHosts(hostname string, extraHosts []string) []byte {
	body := "127.0.0.1 localhost\n::1 localhost\n"
	for _, h := range extraHosts {
		body += h + "\n"
	}
	return []byte(body)
}

// GenerateResolvConf returns a minimal /etc/resolv.conf content.
func GenerateResolvConf(nameservers, search, options []string, dnsAddr string) []byte {
	body := ""
	if dnsAddr != "" {
		body += "nameserver " + dnsAddr + "\n"
	} else {
		body += "nameserver 8.8.8.8\nnameserver 1.1.1.1\n"
	}
	for _, s := range search {
		body += "search " + s + "\n"
	}
	for _, o := range options {
		body += "options " + o + "\n"
	}
	return []byte(body)
}

// Mount operations are not supported on macOS.
func ProcMount(target string) error   { return errUnsupported("proc") }
func SysMount(target string) error    { return errUnsupported("sysfs") }
func DevMount(target string) error    { return errUnsupported("dev") }
func DevPtsMount(target string) error { return errUnsupported("devpts") }
func ShmMount(target string, size int64) error {
	return errUnsupported("tmpfs")
}
func BindMount(source, target string, readOnly bool) error {
	return errUnsupported("bind")
}
func TmpfsMount(target string, size int64, mode os.FileMode) error {
	return errUnsupported("tmpfs")
}
func CleanupMounts(rootfs string) error { return nil }

func errUnsupported(name string) error {
	return fmt.Errorf("fuse: %s mount not supported on darwin", name)
}

// Helper: minimal copy that avoids importing io just for one call.
func copyBuf(in, out *os.File) (int64, error) {
	const bufSize = 32 * 1024
	var total int64
	buf := make([]byte, bufSize)
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			w, werr := out.Write(buf[:n])
			if werr != nil {
				return total, werr
			}
			total += int64(w)
		}
		if rerr != nil {
			if rerr.Error() == "EOF" {
				return total, nil
			}
			return total, rerr
		}
	}
}
