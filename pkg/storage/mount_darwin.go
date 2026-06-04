//go:build darwin
// +build darwin

package storage

import (
	"os"
	"os/exec"
)

// On macOS, container runtimes do not support overlayfs, pivot_root, or
// user namespaces. The exec.Command("mount", ...) path is the universal
// fallback for every platform without raw syscall.Mount access, so we use
// the same approach as mount_android.go.
func osMnt(source, target, fstype string, flags uintptr, data string) error {
	args := []string{"-t", fstype, source, target}
	cmd := exec.Command("mount", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func osUnmount(target string) error {
	cmd := exec.Command("umount", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
