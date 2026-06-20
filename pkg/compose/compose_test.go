// Package compose provides Compose file parsing and orchestration.
package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSubstituteEnv(t *testing.T) {
	env := map[string]string{"NAME": "world", "EMPTY": ""}

	tests := []struct {
		input string
		want  string
	}{
		{"hello $NAME", "hello world"},
		{"hello ${NAME}", "hello world"},
		{"hello ${NAME}!", "hello world!"},
		{"${MISSING:-default}", "default"},
		{"${NAME:-default}", "world"},
		{"${EMPTY:-fallback}", "fallback"},
		{"${NAME:+alt}", "alt"},
		{"${MISSING:+alt}", ""},
		{"no vars here", "no vars here"},
		{"$UNDEFINED", "$UNDEFINED"},
	}
	for _, tt := range tests {
		got := SubstituteEnv(tt.input, env)
		if got != tt.want {
			t.Errorf("SubstituteEnv(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSubstituteEnvInMap(t *testing.T) {
	env := map[string]string{"A": "1", "B": "2"}
	input := map[string]string{"x": "$A", "y": "$B", "z": "static"}
	got := SubstituteEnvInMap(input, env)
	if got["x"] != "1" || got["y"] != "2" || got["z"] != "static" {
		t.Errorf("SubstituteEnvInMap = %v", got)
	}
}

func TestSubstituteEnvInSlice(t *testing.T) {
	env := map[string]string{"PORT": "8080"}
	input := []string{"-p", "$PORT:80", "--name", "test"}
	got := SubstituteEnvInSlice(input, env)
	if got[1] != "8080:80" {
		t.Errorf("SubstituteEnvInSlice[1] = %q, want 8080:80", got[1])
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	content := `# Comment
DB_HOST=localhost
DB_PORT=5432
DB_PASS="secret"
EMPTY=
`
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte(content), 0644)

	env, err := LoadEnvFile(path)
	if err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if env["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST = %q, want localhost", env["DB_HOST"])
	}
	if env["DB_PORT"] != "5432" {
		t.Errorf("DB_PORT = %q, want 5432", env["DB_PORT"])
	}
	if env["DB_PASS"] != "secret" {
		t.Errorf("DB_PASS = %q, want secret (quotes stripped)", env["DB_PASS"])
	}
	if env["EMPTY"] != "" {
		t.Errorf("EMPTY = %q, want empty", env["EMPTY"])
	}
}

func TestLoadEnvFileNotFound(t *testing.T) {
	_, err := LoadEnvFile("/nonexistent/.env")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadEnvFileInvalidFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("INVALID_LINE_NO_EQUALS"), 0644)

	_, err := LoadEnvFile(path)
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestMergeEnv(t *testing.T) {
	m1 := map[string]string{"A": "1", "B": "2"}
	m2 := map[string]string{"B": "3", "C": "4"}
	merged := MergeEnv(m1, m2)
	if merged["A"] != "1" {
		t.Errorf("A = %q, want 1", merged["A"])
	}
	if merged["B"] != "3" {
		t.Errorf("B = %q, want 3 (overridden)", merged["B"])
	}
	if merged["C"] != "4" {
		t.Errorf("C = %q, want 4", merged["C"])
	}
}

func TestParseEnvSlice(t *testing.T) {
	env := ParseEnvSlice([]string{"A=1", "B=2", "C="})
	if env["A"] != "1" || env["B"] != "2" || env["C"] != "" {
		t.Errorf("ParseEnvSlice = %v", env)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		file    *ComposeFile
		wantErr bool
	}{
		{
			name: "valid",
			file: &ComposeFile{
				Services: map[string]*Service{
					"web": {Image: "nginx"},
				},
			},
			wantErr: false,
		},
		{
			name:    "nil file",
			file:    nil,
			wantErr: true,
		},
		{
			name:    "no services",
			file:    &ComposeFile{Services: map[string]*Service{}},
			wantErr: true,
		},
		{
			name: "no image or build",
			file: &ComposeFile{
				Services: map[string]*Service{
					"web": {Command: "echo hello"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid restart",
			file: &ComposeFile{
				Services: map[string]*Service{
					"web": {Image: "nginx", Restart: "invalid"},
				},
			},
			wantErr: true,
		},
		{
			name: "valid restart policies",
			file: &ComposeFile{
				Services: map[string]*Service{
					"a": {Image: "nginx", Restart: "no"},
					"b": {Image: "nginx", Restart: "always"},
					"c": {Image: "nginx", Restart: "on-failure"},
					"d": {Image: "nginx", Restart: "unless-stopped"},
				},
			},
			wantErr: false,
		},
		{
			name: "depends_on reference missing",
			file: &ComposeFile{
				Services: map[string]*Service{
					"web": {Image: "nginx", DependsOn: []interface{}{"missing"}},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateVolumeReference(t *testing.T) {
	file := &ComposeFile{
		Services: map[string]*Service{
			"web": {
				Image:   "nginx",
				Volumes: []interface{}{"mydata:/data"},
			},
		},
		Volumes: map[string]*Volume{},
	}
	err := Validate(file)
	if err == nil {
		t.Error("expected error for undefined volume")
	}
}

func TestSecretManager(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "db_password.txt")
	os.WriteFile(secretFile, []byte("s3cret"), 0644)

	sm := NewSecretManager(dir, map[string]*Secret{
		"db_pass": {File: "db_password.txt"},
	})

	path, err := sm.GetSecretPath("db_pass")
	if err != nil {
		t.Fatalf("GetSecretPath: %v", err)
	}
	if path != secretFile {
		t.Errorf("path = %s, want %s", path, secretFile)
	}

	// Mount secret.
	destDir := filepath.Join(dir, "secrets")
	os.MkdirAll(destDir, 0755)
	if err := sm.MountSecret("db_pass", destDir); err != nil {
		t.Fatalf("MountSecret: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destDir, "db_pass"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "s3cret" {
		t.Errorf("secret content = %q, want s3cret", string(data))
	}
}

func TestConfigManager(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "nginx.conf")
	os.WriteFile(configFile, []byte("server {}"), 0644)

	cm := NewConfigManager(dir, map[string]*Config{
		"nginx_conf": {File: "nginx.conf"},
	})

	path, err := cm.GetConfigPath("nginx_conf")
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if path != configFile {
		t.Errorf("path = %s, want %s", path, configFile)
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"256m", 256 * 1024 * 1024},
		{"1g", 1024 * 1024 * 1024},
		{"512k", 512 * 1024},
		{"1024", 1024},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseMemory(tt.input)
		if got != tt.want {
			t.Errorf("parseMemory(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
