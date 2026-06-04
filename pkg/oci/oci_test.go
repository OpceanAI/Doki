package oci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateConfig(t *testing.T) {
	cfg := &Config{
		ID:       "test-container-001",
		Hostname: "test-host",
		Args:     []string{"/bin/sh", "-c", "echo hello"},
		Env:      []string{"PATH=/usr/bin:/bin", "HOME=/root"},
		Cwd:      "/app",
		User:     "1000:1000",
		Runtime:  "proot",
		ImageRef: "alpine:latest",
		Annotations: map[string]string{
			"custom": "value",
		},
	}

	spec := GenerateConfig(cfg, "/tmp/rootfs")

	// Version.
	if spec.Version != "1.2.0" {
		t.Errorf("Version = %s, want 1.2.0", spec.Version)
	}

	// Hostname.
	if spec.Hostname != "test-host" {
		t.Errorf("Hostname = %s, want test-host", spec.Hostname)
	}

	// Root.
	if spec.Root == nil {
		t.Fatal("Root is nil")
	}
	if spec.Root.Path != "/tmp/rootfs" {
		t.Errorf("Root.Path = %s, want /tmp/rootfs", spec.Root.Path)
	}
	if spec.Root.Readonly {
		t.Error("Root.Readonly should be false")
	}

	// Process.
	if spec.Process == nil {
		t.Fatal("Process is nil")
	}
	if !spec.Process.Terminal {
		// Tty is false by default.
	}
	if spec.Process.Cwd != "/app" {
		t.Errorf("Process.Cwd = %s, want /app", spec.Process.Cwd)
	}
	if spec.Process.User.UID != 1000 || spec.Process.User.GID != 1000 {
		t.Errorf("User = %d:%d, want 1000:1000", spec.Process.User.UID, spec.Process.User.GID)
	}

	// Linux.
	if spec.Linux == nil {
		t.Fatal("Linux is nil")
	}
	if len(spec.Linux.Namespaces) == 0 {
		t.Error("Namespaces is empty")
	}
	if len(spec.Linux.MaskedPaths) == 0 {
		t.Error("MaskedPaths is empty")
	}
	if len(spec.Linux.ReadonlyPaths) == 0 {
		t.Error("ReadonlyPaths is empty")
	}
	if spec.Linux.Seccomp == nil {
		t.Error("Seccomp is nil")
	}
	if len(spec.Linux.Devices) == 0 {
		t.Error("Devices is empty")
	}

	// Annotations.
	if spec.Annotations["com.doki.container.id"] != "test-container-001" {
		t.Error("Missing container ID annotation")
	}
	if spec.Annotations["custom"] != "value" {
		t.Error("Custom annotation not preserved")
	}

	// Mounts.
	if len(spec.Mounts) == 0 {
		t.Error("Mounts is empty")
	}
	hasProc := false
	for _, m := range spec.Mounts {
		if m.Destination == "/proc" && m.Type == "proc" {
			hasProc = true
		}
	}
	if !hasProc {
		t.Error("Missing /proc mount")
	}
}

func TestGenerateConfigPrivileged(t *testing.T) {
	cfg := &Config{
		ID:         "priv-001",
		Args:       []string{"/bin/sh"},
		Privileged: true,
	}

	spec := GenerateConfig(cfg, "/tmp/rootfs")

	// Privileged containers should have all capabilities.
	if spec.Process.Capabilities == nil {
		t.Fatal("Capabilities is nil")
	}
	if len(spec.Process.Capabilities.Bounding) == 0 {
		t.Error("Privileged container should have all capabilities")
	}

	// Should have permissive seccomp.
	if spec.Linux.Seccomp == nil {
		t.Fatal("Seccomp is nil")
	}
	if spec.Linux.Seccomp.DefaultAction != SeccompActAllow {
		t.Errorf("DefaultAction = %s, want SCMP_ACT_ALLOW", spec.Linux.Seccomp.DefaultAction)
	}
}

func TestGenerateConfigHostNetwork(t *testing.T) {
	cfg := &Config{
		ID:          "host-net-001",
		Args:        []string{"/bin/sh"},
		NetworkMode: "host",
	}

	spec := GenerateConfig(cfg, "/tmp/rootfs")

	// Should NOT have network namespace.
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type == NamespaceNetwork {
			t.Error("Host network should not have network namespace")
		}
	}
}

func TestGenerateConfigReadOnly(t *testing.T) {
	cfg := &Config{
		ID:       "ro-001",
		Args:     []string{"/bin/sh"},
		ReadOnly: true,
	}

	spec := GenerateConfig(cfg, "/tmp/rootfs")

	if !spec.Root.Readonly {
		t.Error("Root.Readonly should be true")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    *Spec
		wantErr bool
	}{
		{
			name: "valid",
			spec: &Spec{
				Version: "1.2.0",
				Process: &Process{Args: []string{"/bin/sh"}},
				Root:    &Root{Path: "/tmp/rootfs"},
			},
			wantErr: false,
		},
		{
			name: "missing version",
			spec: &Spec{
				Process: &Process{Args: []string{"/bin/sh"}},
				Root:    &Root{Path: "/tmp/rootfs"},
			},
			wantErr: true,
		},
		{
			name: "missing process",
			spec: &Spec{
				Version: "1.2.0",
				Root:    &Root{Path: "/tmp/rootfs"},
			},
			wantErr: true,
		},
		{
			name: "missing root",
			spec: &Spec{
				Version: "1.2.0",
				Process: &Process{Args: []string{"/bin/sh"}},
			},
			wantErr: true,
		},
		{
			name: "empty args",
			spec: &Spec{
				Version: "1.2.0",
				Process: &Process{Args: []string{}},
				Root:    &Root{Path: "/tmp/rootfs"},
			},
			wantErr: true,
		},
		{
			name: "relative cwd",
			spec: &Spec{
				Version: "1.2.0",
				Process: &Process{Args: []string{"/bin/sh"}, Cwd: "relative/path"},
				Root:    &Root{Path: "/tmp/rootfs"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateNamespaces(t *testing.T) {
	spec := &Spec{
		Version: "1.2.0",
		Process: &Process{Args: []string{"/bin/sh"}},
		Root:    &Root{Path: "/tmp/rootfs"},
		Linux: &Linux{
			Namespaces: []LinuxNamespace{
				{Type: NamespacePID},
				{Type: NamespacePID}, // duplicate
			},
		},
	}
	err := Validate(spec)
	if err == nil {
		t.Error("expected error for duplicate namespace")
	}
}

func TestParseCapability(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{"sys_admin", "CAP_SYS_ADMIN", false},
		{"CAP_SYS_ADMIN", "CAP_SYS_ADMIN", false},
		{"SYS_ADMIN", "CAP_SYS_ADMIN", false},
		{"chown", "CAP_CHOWN", false},
		{"invalid_cap_xyz", "", true},
	}
	for _, tt := range tests {
		got, err := ParseCapability(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("ParseCapability(%q) error = %v, wantErr %v", tt.input, err, tt.err)
		}
		if got != tt.want {
			t.Errorf("ParseCapability(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDefaultCapabilities(t *testing.T) {
	caps := DefaultCapabilities()
	if len(caps) == 0 {
		t.Error("DefaultCapabilities is empty")
	}
	for _, c := range caps {
		if _, err := ParseCapability(c); err != nil {
			t.Errorf("Invalid default capability: %s", c)
		}
	}
}

func TestAllCapabilities(t *testing.T) {
	caps := AllCapabilities()
	if len(caps) < 30 {
		t.Errorf("AllCapabilities has %d caps, expected >= 30", len(caps))
	}
}

func TestMaskedPaths(t *testing.T) {
	paths := MaskedPaths()
	if len(paths) == 0 {
		t.Error("MaskedPaths is empty")
	}
	for _, p := range paths {
		if p[0] != '/' {
			t.Errorf("MaskedPath %s is not absolute", p)
		}
	}
}

func TestReadonlyPaths(t *testing.T) {
	paths := ReadonlyPaths()
	if len(paths) == 0 {
		t.Error("ReadonlyPaths is empty")
	}
}

func TestDefaultSeccompProfile(t *testing.T) {
	p := DefaultSeccompProfile()
	if p == nil {
		t.Fatal("DefaultSeccompProfile returned nil")
	}
	if p.DefaultAction != SeccompActErrno {
		t.Errorf("DefaultAction = %s, want SCMP_ACT_ERRNO", p.DefaultAction)
	}
	if len(p.Syscalls) == 0 {
		t.Error("Syscalls is empty")
	}
	if p.Syscalls[0].Action != SeccompActAllow {
		t.Error("Syscalls[0] should be ALLOW")
	}
}

func TestPrivilegedSeccompProfile(t *testing.T) {
	p := PrivilegedSeccompProfile()
	if p.DefaultAction != SeccompActAllow {
		t.Errorf("DefaultAction = %s, want SCMP_ACT_ALLOW", p.DefaultAction)
	}
}

func TestWriteReadConfig(t *testing.T) {
	dir := t.TempDir()
	spec := &Spec{
		Version: "1.2.0",
		Process: &Process{Args: []string{"/bin/sh"}},
		Root:    &Root{Path: "/tmp/rootfs"},
	}

	if err := WriteConfig(dir, spec); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	// Verify file exists.
	configPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config.json not found: %v", err)
	}

	// Read back.
	loaded, err := ReadConfig(dir)
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if loaded.Version != spec.Version {
		t.Errorf("Version = %s, want %s", loaded.Version, spec.Version)
	}
	if loaded.Process.Args[0] != "/bin/sh" {
		t.Errorf("Args[0] = %s, want /bin/sh", loaded.Process.Args[0])
	}
}

func TestMarshalSpec(t *testing.T) {
	spec := &Spec{
		Version: "1.2.0",
		Process: &Process{Args: []string{"/bin/sh"}},
		Root:    &Root{Path: "/tmp/rootfs"},
	}
	data, err := MarshalSpec(spec)
	if err != nil {
		t.Fatalf("MarshalSpec: %v", err)
	}
	// Should be valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if parsed["ociVersion"] != "1.2.0" {
		t.Errorf("ociVersion = %v, want 1.2.0", parsed["ociVersion"])
	}
}

func TestLinuxNamespaceTypes(t *testing.T) {
	types := []LinuxNamespaceType{
		NamespacePID, NamespaceNetwork, NamespaceMount,
		NamespaceIPC, NamespaceUTS, NamespaceUser,
		NamespaceCgroup, NamespaceTime,
	}
	for _, ns := range types {
		if ns == "" {
			t.Error("empty namespace type")
		}
	}
}

func TestGenerateConfigWithImageConfig(t *testing.T) {
	cfg := &Config{
		ID:   "img-001",
		Args: []string{}, // empty args, should use image entrypoint
		ImageConfig: &ImageConfig{
			Entrypoint: []string{"/docker-entrypoint.sh"},
			Cmd:        []string{"nginx", "-g", "daemon off;"},
			WorkingDir: "/usr/share/nginx",
			Env:        []string{"NGINX_VERSION=1.25"},
		},
	}

	spec := GenerateConfig(cfg, "/tmp/rootfs")

	// Should use image entrypoint + cmd.
	if len(spec.Process.Args) == 0 {
		t.Error("Args should not be empty")
	}
	if spec.Process.Args[0] != "/docker-entrypoint.sh" {
		t.Errorf("Args[0] = %s, want /docker-entrypoint.sh", spec.Process.Args[0])
	}

	// Cwd should come from image.
	if spec.Process.Cwd != "/usr/share/nginx" {
		t.Errorf("Cwd = %s, want /usr/share/nginx", spec.Process.Cwd)
	}
}
