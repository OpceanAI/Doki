package macos

import (
	"fmt"
	"os/exec"
	"strings"
)

type SandboxBackend struct{}

func NewSandboxBackend() *SandboxBackend {
	return &SandboxBackend{}
}

func (b *SandboxBackend) Name() string         { return "sandbox" }
func (b *SandboxBackend) Available() bool       { _, err := exec.LookPath("sandbox-exec"); return err == nil }
func (b *SandboxBackend) MinVersion() string    { return "10.15" }

func (b *SandboxBackend) CreateVM(_ *VMConfig) error  { return nil }
func (b *SandboxBackend) StartVM(_ string) error        { return nil }
func (b *SandboxBackend) StopVM(_ string, _ int) error  { return nil }
func (b *SandboxBackend) DeleteVM(_ string) error       { return nil }
func (b *SandboxBackend) VMStatus(_ string) (string, error) { return "native", nil }
func (b *SandboxBackend) ShareHostDir(_, _, _ string, _ bool) error { return nil }
func (b *SandboxBackend) UnshareHostDir(_ string) error  { return nil }
func (b *SandboxBackend) ForwardPort(_, _ int, _ string) error { return nil }
func (b *SandboxBackend) RemoveForwardPort(_ int, _ string) error { return nil }
func (b *SandboxBackend) Stats(_ string) (*VMStats, error) {
	return &VMStats{State: "native"}, nil
}

func GenerateSandboxProfile(rootfs string, readOnly bool, network string) string {
	var sb strings.Builder
	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n\n")

	for _, path := range []string{"/usr", "/System", "/Library", "/private/var",
		"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"} {
		fmt.Fprintf(&sb, "(allow file-read* (subpath %q))\n", path)
	}

	fmt.Fprintf(&sb, "\n(allow file-read* (subpath %q))\n", rootfs)
	if !readOnly {
		fmt.Fprintf(&sb, "(allow file-write* (subpath %q))\n", rootfs)
		fmt.Fprintf(&sb, "(allow file-write* (subpath \"/private/tmp\"))\n")
	}

	switch network {
	case "host":
		sb.WriteString("(allow network*)\n")
	case "none":
		sb.WriteString("(deny network*)\n")
	default:
		sb.WriteString("(allow network-outbound)\n")
		sb.WriteString("(allow network-inbound (local tcp))\n")
	}

	sb.WriteString("(allow process-exec)\n")
	sb.WriteString("(allow process-fork)\n")
	sb.WriteString("(allow signal (target same-sandbox))\n")
	sb.WriteString("(allow sysctl-read)\n")
	sb.WriteString("(allow mach-lookup)\n")
	sb.WriteString("(allow ipc-posix-shm)\n")

	return sb.String()
}
