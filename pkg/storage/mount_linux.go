//go:build linux && !android
// +build linux,!android

package storage

import (
	"syscall"

	"github.com/OpceanAI/Doki/pkg/common"
)

func osMnt(source, target, fstype string, flags uintptr, data string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}

	if err := syscall.Mount(source, target, fstype, flags, data); err != nil {
		// Try with no source for overlay.
		if fstype == "overlay" {
			dataWithSource := "source=overlay," + data
			return syscall.Mount("overlay", target, "overlay", flags, dataWithSource)
		}
		return err
	}

	return nil
}

func osUnmount(target string) error {
	return syscall.Unmount(target, syscall.MNT_DETACH)
}
