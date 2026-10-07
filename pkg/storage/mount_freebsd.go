//go:build freebsd

package storage

import (
	"os"
	"os/exec"

	"github.com/OpceanAI/Doki/pkg/common"
)

// osMnt mounts source at target. FreeBSD has no overlayfs, so read-only
// layers mount via nullfs (bind-mount semantics) and read-write layers in
// ModeNative are copied by the driver instead, because nullfs provides no
// copy-on-write.
func osMnt(source, target, fstype string, flags uintptr, data string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}
	args := []string{"-t", fstype, source, target}
	cmd := exec.Command("mount", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// osUnmount unmounts target.
func osUnmount(target string) error {
	cmd := exec.Command("umount", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
