package macos

import (
	"fmt"
	"os/exec"
	"strings"
)

// SandboxBackend uses macOS sandbox-exec to run processes in a
// restricted sandbox profile. This is the lightest isolation backend
// and does not require Hypervisor.framework.
type SandboxBackend struct{}

// newSandboxBackend creates a SandboxBackend.
func newSandboxBackend() *SandboxBackend {
	return &SandboxBackend{}
}

func (b *SandboxBackend) Name() string       { return "sandbox" }
func (b *SandboxBackend) Available() bool    { _, err := exec.LookPath("sandbox-exec"); return err == nil }
func (b *SandboxBackend) MinVersion() string { return "10.15" }

func (b *SandboxBackend) CreateVM(_ *VMConfig) error                { return nil }
func (b *SandboxBackend) StartVM(_ string) error                    { return nil }
func (b *SandboxBackend) StopVM(_ string, _ int) error              { return nil }
func (b *SandboxBackend) DeleteVM(_ string) error                   { return nil }
func (b *SandboxBackend) VMStatus(_ string) (string, error)         { return "native", nil }
func (b *SandboxBackend) ShareHostDir(_, _, _ string, _ bool) error { return nil }
func (b *SandboxBackend) UnshareHostDir(_ string) error             { return nil }
func (b *SandboxBackend) ForwardPort(_, _ int, _ string) error      { return nil }
func (b *SandboxBackend) RemoveForwardPort(_ int, _ string) error   { return nil }
func (b *SandboxBackend) Stats(_ string) (*VMStats, error) {
	return &VMStats{State: "native"}, nil
}

// GenerateSandboxProfile produces a sandbox-exec profile string.
// The profile is tightened: process-exec is scoped to the rootfs
// and system paths (not unrestricted), mach-lookup is restricted to
// named services, and ipc-posix-shm is scoped.
func GenerateSandboxProfile(rootfs string, readOnly bool, network string) string {
	var sb strings.Builder
	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n\n")

	// Read-only system paths (no /private/var to avoid leaking
	// keychain material and user data).
	for _, path := range []string{"/usr", "/System", "/Library",
		"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"} {
		fmt.Fprintf(&sb, "(allow file-read* (subpath %q))\n", path)
	}

	// Rootfs access.
	fmt.Fprintf(&sb, "\n(allow file-read* (subpath %q))\n", rootfs)
	if !readOnly {
		fmt.Fprintf(&sb, "(allow file-write* (subpath %q))\n", rootfs)
		fmt.Fprintf(&sb, "(allow file-write* (subpath \"/private/tmp\"))\n")
	}

	// Network policy.
	switch network {
	case "host":
		sb.WriteString("(allow network*)\n")
	case "none":
		sb.WriteString("(deny network*)\n")
	default:
		sb.WriteString("(allow network-outbound)\n")
		sb.WriteString("(allow network-inbound (local tcp))\n")
	}

	// Process execution: scoped to rootfs and system binary paths
	// rather than unrestricted. This prevents executing arbitrary
	// binaries from user-writable locations.
	sb.WriteString("(allow process-exec (subpath %q))\n")
	sb.WriteString("(allow process-exec (subpath \"/usr/bin\"))\n")
	sb.WriteString("(allow process-exec (subpath \"/bin\"))\n")
	sb.WriteString("(allow process-fork)\n")
	sb.WriteString("(allow signal (target same-sandbox))\n")
	sb.WriteString("(allow sysctl-read)\n")
	// mach-lookup restricted to common safe services instead of
	// unrestricted access to all Mach services.
	sb.WriteString("(allow mach-lookup (global-name \"com.apple.system.logger\"))\n")
	sb.WriteString("(allow mach-lookup (global-name \"com.apple.system.notification_center\"))\n")
	// ipc-posix-shm restricted to system shared memory.
	sb.WriteString("(allow ipc-posix-shm ( ipc-posix-name-regex \"^/apple\" ))\n")

	// Fix the format string for rootfs process-exec.
	result := sb.String()
	result = strings.Replace(result, "(subpath %q))\n", fmt.Sprintf("(subpath %q))\n", rootfs), 1)
	return result
}
