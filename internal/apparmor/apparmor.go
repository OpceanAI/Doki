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
const DefaultProfileTemplate = `
#include <tunables/global>

profile doki-{{.Name}} flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>

  # Filesystem access.
  / r,
  /** rw,

  # Deny sensitive paths.
  deny /proc/sysrq-trigger rw,
  deny /proc/irq/** rw,
  deny /proc/bus/** rw,
  deny /sys/kernel/security/** rw,
  deny /sys/firmware/** rw,

  # Network access.
  network inet stream,
  network inet6 stream,
  network inet dgram,
  network inet6 dgram,

  # Capabilities.
  capability setuid,
  capability setgid,
  capability net_bind_service,
  capability chown,
  capability dac_override,
  capability dac_read_search,
  capability fowner,
  capability fsetid,
  capability kill,
  capability setpcap,
  capability sys_ptrace,

  # Signals.
  signal (receive, send) peer=doki-*,

  # Mount restrictions.
  deny mount,
  deny remount,

  # Pivot root.
  deny pivot_root,
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

func stringContains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && 
		(len(s) >= len(substr))
}
