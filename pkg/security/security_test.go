package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultProfile(t *testing.T) {
	p := DefaultProfile()
	if p == nil {
		t.Fatal("DefaultProfile returned nil")
	}
	if p.DefaultAction != ActErrno {
		t.Errorf("DefaultAction = %s, want %s", p.DefaultAction, ActErrno)
	}
	if len(p.Syscalls) == 0 {
		t.Error("Syscalls is empty")
	}
	if len(p.Architectures) == 0 {
		t.Error("Architectures is empty")
	}
}

func TestPrivilegedProfile(t *testing.T) {
	p := PrivilegedProfile()
	if p.DefaultAction != ActAllow {
		t.Errorf("DefaultAction = %s, want %s", p.DefaultAction, ActAllow)
	}
}

func TestAndroidProfile(t *testing.T) {
	p := AndroidProfile()
	if p == nil {
		t.Fatal("AndroidProfile returned nil")
	}
	if p.DefaultAction != ActErrno {
		t.Errorf("DefaultAction = %s, want %s", p.DefaultAction, ActErrno)
	}
	// Should have ARM architectures.
	hasARM := false
	for _, arch := range p.Architectures {
		if arch == ArchAARCH64 || arch == ArchARM {
			hasARM = true
		}
	}
	if !hasARM {
		t.Error("AndroidProfile should have ARM architectures")
	}
}

func TestARMProfile(t *testing.T) {
	p := ARMProfile()
	if p == nil {
		t.Fatal("ARMProfile returned nil")
	}
	if len(p.Architectures) != 2 {
		t.Errorf("Architectures = %d, want 2", len(p.Architectures))
	}
}

func TestValidateProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile *SeccompProfile
		wantErr bool
	}{
		{"valid", DefaultProfile(), false},
		{"nil", nil, true},
		{"invalid action", &SeccompProfile{DefaultAction: "INVALID"}, true},
		{"empty syscalls", &SeccompProfile{DefaultAction: ActAllow}, false},
		{"invalid syscall action", &SeccompProfile{
			DefaultAction: ActErrno,
			Syscalls:      []SeccompSyscall{{Names: []string{"read"}, Action: "INVALID"}},
		}, true},
		{"empty names", &SeccompProfile{
			DefaultAction: ActErrno,
			Syscalls:      []SeccompSyscall{{Names: []string{}, Action: ActAllow}},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProfile(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProfile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveLoadProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-profile.json")

	profile := DefaultProfile()
	if err := SaveProfile(profile, path); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	loaded, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	if loaded.DefaultAction != profile.DefaultAction {
		t.Errorf("DefaultAction = %s, want %s", loaded.DefaultAction, profile.DefaultAction)
	}
	if len(loaded.Syscalls) != len(profile.Syscalls) {
		t.Errorf("Syscalls = %d, want %d", len(loaded.Syscalls), len(profile.Syscalls))
	}
}

func TestLoadProfileNotFound(t *testing.T) {
	_, err := LoadProfile("/nonexistent/profile.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMergeProfiles(t *testing.T) {
	p1 := &SeccompProfile{
		DefaultAction: ActErrno,
		Syscalls: []SeccompSyscall{
			{Names: []string{"read"}, Action: ActAllow},
		},
	}
	p2 := &SeccompProfile{
		DefaultAction: ActErrno,
		Syscalls: []SeccompSyscall{
			{Names: []string{"write"}, Action: ActAllow},
			{Names: []string{"read"}, Action: ActLog}, // Override
		},
	}
	merged := MergeProfiles(p1, p2)
	if len(merged.Syscalls) != 2 {
		t.Errorf("Syscalls = %d, want 2", len(merged.Syscalls))
	}
}

func TestProfileToFromJSON(t *testing.T) {
	profile := DefaultProfile()
	jsonStr, err := ProfileToJSON(profile)
	if err != nil {
		t.Fatalf("ProfileToJSON: %v", err)
	}
	if jsonStr == "" {
		t.Error("ProfileToJSON returned empty")
	}

	loaded, err := ProfileFromJSON(jsonStr)
	if err != nil {
		t.Fatalf("ProfileFromJSON: %v", err)
	}
	if loaded.DefaultAction != profile.DefaultAction {
		t.Errorf("DefaultAction = %s, want %s", loaded.DefaultAction, profile.DefaultAction)
	}
}

func TestProfileString(t *testing.T) {
	profile := DefaultProfile()
	s := profile.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestDefaultArchitectures(t *testing.T) {
	archs := DefaultArchitectures()
	if len(archs) == 0 {
		t.Error("DefaultArchitectures is empty")
	}
}

func TestCustomAppArmorProfile(t *testing.T) {
	rules := []string{
		"/tmp/** rw",
		"network inet stream,",
	}
	profile := CustomProfile("test-profile", rules)
	if profile.Name != "test-profile" {
		t.Errorf("Name = %s, want test-profile", profile.Name)
	}
	if profile.Content == "" {
		t.Error("Content is empty")
	}
}

func TestDefaultAppArmorProfile(t *testing.T) {
	profile := DefaultAppArmorProfile()
	if profile == nil {
		t.Fatal("DefaultAppArmorProfile returned nil")
	}
	if profile.Name != "doki-default" {
		t.Errorf("Name = %s, want doki-default", profile.Name)
	}
	if profile.Content == "" {
		t.Error("Content is empty")
	}
}

func TestAppArmorManagerIsAvailable(t *testing.T) {
	m := NewAppArmorManager()
	// This will return false in most test environments.
	available := m.IsAvailable()
	_ = available
}

func TestSaveAppArmorProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-profile")

	profile := DefaultAppArmorProfile()
	if err := SaveAppArmorProfile(profile, path); err != nil {
		t.Fatalf("SaveAppArmorProfile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("saved file is empty")
	}
}
