//go:build android
// +build android

package storage

import (
	"os"
	"os/exec"

	"github.com/OpceanAI/Doki/pkg/common"
)

func osMnt(source, target, fstype string, flags uintptr, data string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}

	args := []string{"-t", fstype, "-o", data, source, target}
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
