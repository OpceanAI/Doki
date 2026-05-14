package apparmor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

// Profile represents an AppArmor profile.
type Profile struct {
	Name    string
	Content string
}

// DefaultProfileTemplate is the default Doki AppArmor profile template.
// AE9: Restricts file access, network, capabilities, and mounts.
const DefaultProfileTemplate = `
#include <tunables/global>

profile doki-{{.Name}} flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  #include <abstractions/nameservice>

  # Filesystem: restrict to container root, deny sensitive paths.
  / r,
  /** rw,
  /proc/ r,
  /proc/sysrq-trigger r,
  /proc/[0-9]*/ r,
  /proc/[0-9]*/** rw,
  /proc/sys/ r,
  /proc/sys/kernel/ r,
  /proc/sys/net/ r,
  /proc/sys/net/core/ r,
  /proc/sys/net/ipv4/ r,
  /sys/ r,

  # Deny sensitive system paths.
  deny /proc/sysrq-trigger rw,
  deny /proc/irq/** rw,
  deny /proc/bus/** rw,
  deny /sys/kernel/security/** rw,
  deny /sys/firmware/** rw,
  deny /sys/kernel/debug/** rw,
  deny /sys/module/** rw,

  # Deny access to kernel and boot files.
  deny /boot/** rw,
  deny /vmlinuz* r,
  deny /initrd* r,

  # Mount restrictions (deny all mounts).
  deny mount,
  deny remount,
  deny umount,

  # Pivot root denied.
  deny pivot_root,

  # Network: allow only TCP/UDP streams and dgrams (no raw sockets).
  network inet stream,
  network inet6 stream,
  network inet dgram,
  network inet6 dgram,
  deny network raw,
  deny network packet,

  # Minimal capabilities for non-privileged containers.
  capability setuid,
  capability setgid,
  capability chown,
  capability dac_override,
  capability dac_read_search,
  capability fowner,
  capability fsetid,
  capability kill,
  capability setpcap,

  # Deny dangerous capabilities.
  deny capability sys_admin,
  deny capability sys_module,
  deny capability sys_rawio,
  deny capability sys_boot,
  deny capability sys_time,
  deny capability net_admin,
  deny capability mac_admin,
  deny capability mac_override,
  deny capability syslog,
  deny capability sys_ptrace,

  # Signal restrictions.
  signal (receive, send) peer=doki-*,

  # Ptrace restrictions.
  deny ptrace (read, trace) peer=unconfined,
}`

// NewProfile creates a new AppArmor profile for a container.
func NewProfile(containerName string) (*Profile, error) {
	tmpl, err := template.New("apparmor").Parse(DefaultProfileTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	writer := &bufferWriter{buf: make([]byte, 0, 4096)}
	if err := tmpl.Execute(writer, map[string]string{"Name": containerName}); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return &Profile{
		Name:    "doki-" + containerName,
		Content: string(writer.buf),
	}, nil
}

type bufferWriter struct {
	buf []byte
}

func (w *bufferWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

// IsEnabled checks if AppArmor is available on the system.
func IsEnabled() bool {
	_, err := os.Stat("/sys/kernel/security/apparmor")
	if err == nil {
		return true
	}
	_, err = os.Stat("/sys/module/apparmor")
	return err == nil
}

// IsAppArmorAvailable checks specifically for AppArmor (not SELinux).
func IsAppArmorAvailable() bool {
	_, err := os.Stat("/sys/kernel/security/apparmor")
	if err == nil {
		return true
	}
	_, err = os.Stat("/sys/module/apparmor")
	return err == nil
}

// IsSELinuxAvailable checks if SELinux is available (Android fallback).
func IsSELinuxAvailable() bool {
	_, err := os.Stat("/sys/fs/selinux")
	return err == nil
}

// LoadProfile loads an AppArmor profile into the kernel.
func LoadProfile(profile *Profile) error {
	if !IsEnabled() {
		return fmt.Errorf("apparmor not available")
	}
	tmpDir := "/tmp/doki-apparmor"
	os.MkdirAll(tmpDir, 0755)
	profilePath := filepath.Join(tmpDir, profile.Name)
	if err := os.WriteFile(profilePath, []byte(profile.Content), 0644); err != nil {
		return err
	}
	cmd := exec.Command("apparmor_parser", "-Kr", profilePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("load profile: %w (output: %s)", err, string(out))
	}
	os.Remove(profilePath)
	return nil
}

// UnloadProfile removes an AppArmor profile from the kernel.
func UnloadProfile(name string) error {
	if !IsEnabled() {
		return nil
	}
	cmd := exec.Command("apparmor_parser", "-R", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unload profile: %w (output: %s)", err, string(out))
	}
	return nil
}

