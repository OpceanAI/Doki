package security

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AppArmorProfile represents an AppArmor profile.
type AppArmorProfile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Mode    string `json:"mode"` // enforce, complain, unconfined
}

// AppArmorManager manages AppArmor profiles.
type AppArmorManager struct {
	profileDir string
}

// NewAppArmorManager creates a new AppArmor manager.
func NewAppArmorManager() *AppArmorManager {
	return &AppArmorManager{
		profileDir: "/etc/apparmor.d",
	}
}

// IsAvailable checks if AppArmor is available on the system.
func (m *AppArmorManager) IsAvailable() bool {
	_, err := exec.LookPath("apparmor_parser")
	if err != nil {
		return false
	}
	// Check if AppArmor is enabled.
	data, err := os.ReadFile("/sys/module/apparmor/parameters/enabled")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "Y"
}

// LoadProfile loads an AppArmor profile into the kernel.
func (m *AppArmorManager) LoadProfile(profile *AppArmorProfile) error {
	if !m.IsAvailable() {
		return fmt.Errorf("apparmor not available")
	}

	profilePath := filepath.Join(m.profileDir, profile.Name)
	if err := os.WriteFile(profilePath, []byte(profile.Content), 0644); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}

	cmd := exec.Command("apparmor_parser", "-Kr", profilePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("load profile: %s: %w", string(out), err)
	}

	return nil
}

// UnloadProfile unloads an AppArmor profile from the kernel.
func (m *AppArmorManager) UnloadProfile(name string) error {
	if !m.IsAvailable() {
		return fmt.Errorf("apparmor not available")
	}

	profilePath := filepath.Join(m.profileDir, name)
	cmd := exec.Command("apparmor_parser", "-R", profilePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("unload profile: %s: %w", string(out), err)
	}

	return nil
}

// ApplyProfile applies an AppArmor profile to the current process.
func (m *AppArmorManager) ApplyProfile(name string) error {
	attrPath := "/proc/self/attr/current"
	if err := os.WriteFile(attrPath, []byte(name), 0); err != nil {
		return fmt.Errorf("apply profile: %w", err)
	}
	return nil
}

// DefaultProfile returns the default AppArmor profile for containers.
func DefaultAppArmorProfile() *AppArmorProfile {
	return &AppArmorProfile{
		Name: "doki-default",
		Content: `#include <tunables/global>

profile doki-default flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>

  # Network access
  network inet stream,
  network inet dgram,
  network inet6 stream,
  network inet6 dgram,
  network unix stream,
  network unix dgram,

  # File access
  / r,
  /** r,
  /tmp/** rw,
  /var/tmp/** rw,
  /dev/null rw,
  /dev/zero rw,
  /dev/full rw,
  /dev/random rw,
  /dev/urandom rw,
  /dev/tty rw,
  /dev/console rw,
  /dev/pts/** rw,
  /dev/shm/** rw,
  /dev/mqueue/** rw,
  /proc/** r,
  /sys/** r,

  # Capabilities
  capability chown,
  capability dac_override,
  capability fsetid,
  capability fowner,
  capability mknod,
  capability net_raw,
  capability setgid,
  capability setuid,
  capability setfcap,
  capability setpcap,
  capability net_bind_service,
  capability sys_chroot,
  capability kill,
  capability audit_write,

  # Signals
  signal (receive) set=(term, kill, quit, alrm, hup),

  # ptrace
  ptrace (read, readby) peer=doki-default,

  # mount
  mount,
  umount,

  # pivot_root
  pivot_root,

  # Executions
  /bin/** ix,
  /usr/bin/** ix,
  /usr/local/bin/** ix,
  /sbin/** ix,
  /usr/sbin/** ix,

  # Deny dangerous operations
  deny /proc/sysrq-trigger rw,
  deny /proc/latency_stats rw,
  deny /proc/kcore rw,
  deny /sys/firmware/** rw,
  deny /sys/devices/virtual/powercap/** rw,
  deny /proc/acpi/** rw,
  deny /proc/sysrq-trigger rw,
}`,
		Mode: "enforce",
	}
}

// UnconfinedProfile returns an unconfined AppArmor profile.
func UnconfinedAppArmorProfile() *AppArmorProfile {
	return &AppArmorProfile{
		Name: "doki-unconfined",
		Content: `#include <tunables/global>

profile doki-unconfined flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  /**
}`,
		Mode: "unconfined",
	}
}

// CustomProfile creates a custom AppArmor profile with specific rules.
func CustomProfile(name string, rules []string) *AppArmorProfile {
	var content strings.Builder
	content.WriteString("#include <tunables/global>\n\n")
	content.WriteString(fmt.Sprintf("profile %s flags=(attach_disconnected,mediate_deleted) {\n", name))
	content.WriteString("  #include <abstractions/base>\n\n")
	for _, rule := range rules {
		content.WriteString("  " + rule + "\n")
	}
	content.WriteString("}\n")

	return &AppArmorProfile{
		Name:    name,
		Content: content.String(),
		Mode:    "enforce",
	}
}

// LoadAppArmorProfile loads an AppArmor profile from a file.
func LoadAppArmorProfile(path string) (*AppArmorProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read apparmor profile: %w", err)
	}
	return &AppArmorProfile{
		Name:    filepath.Base(path),
		Content: string(data),
		Mode:    "enforce",
	}, nil
}

// SaveAppArmorProfile saves an AppArmor profile to a file.
func SaveAppArmorProfile(profile *AppArmorProfile, path string) error {
	return os.WriteFile(path, []byte(profile.Content), 0644)
}
