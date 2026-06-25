package emulation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

const (
	ModeAuto  = "auto"
	ModeQEMU  = "qemu"
	ModeFEX   = "fex"
	ModeBox64 = "box64"
)

// Config stores the user's preferred cross-architecture emulator.
type Config struct {
	Preferred string    `json:"preferred"`
	Selected  string    `json:"selected,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Results   []Result  `json:"results,omitempty"`
}

// Result describes one detected emulator backend.
type Result struct {
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ConfigPath returns the persistent emulation preference path.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(common.DataDir(), "emulation.json")
	}
	return filepath.Join(home, common.DefaultConfigDir, "emulation.json")
}

// NormalizeMode canonicalizes user input.
func NormalizeMode(mode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeAuto:
		return ModeAuto, true
	case ModeQEMU, "qemu-user", "qemuuser":
		return ModeQEMU, true
	case ModeFEX, "fex-emu", "fexemu", "fexinterpreter":
		return ModeFEX, true
	case ModeBox64:
		return ModeBox64, true
	default:
		return "", false
	}
}

// Load reads the saved emulation configuration. Missing config returns auto.
func Load() (*Config, error) {
	cfg := &Config{Preferred: ModeAuto}
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if normalized, ok := NormalizeMode(cfg.Preferred); ok {
		cfg.Preferred = normalized
	} else {
		cfg.Preferred = ModeAuto
	}
	return cfg, nil
}

// Save persists the emulation configuration with private permissions.
func Save(cfg *Config) error {
	if cfg == nil {
		cfg = &Config{Preferred: ModeAuto}
	}
	mode, ok := NormalizeMode(cfg.Preferred)
	if !ok {
		return fmt.Errorf("unknown emulation mode %q", cfg.Preferred)
	}
	cfg.Preferred = mode
	cfg.UpdatedAt = time.Now().UTC()
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(path, 0600)
}

// PreferredMode returns the effective user preference. Env wins over disk.
func PreferredMode() string {
	for _, key := range []string{"DOKI_EMULATION_MODE", "DOKI_EMULATOR"} {
		if value := os.Getenv(key); value != "" {
			if mode, ok := NormalizeMode(value); ok {
				return mode
			}
		}
	}
	cfg, err := Load()
	if err != nil || cfg == nil {
		return ModeAuto
	}
	return cfg.Preferred
}

// Detect checks available QEMU/FEX/Box64 backends using PATH.
func Detect(ctx context.Context) []Result {
	candidates := []struct {
		name string
		bins []string
	}{
		{ModeQEMU, []string{"qemu-x86_64-static", "qemu-aarch64-static", "qemu-arm-static", "qemu-i386-static"}},
		{ModeFEX, []string{"FEXInterpreter"}},
		{ModeBox64, []string{"box64"}},
	}
	out := make([]Result, 0, len(candidates))
	for _, candidate := range candidates {
		res := Result{Name: candidate.name}
		for _, bin := range candidate.bins {
			path, err := exec.LookPath(bin)
			if err != nil {
				continue
			}
			res.Path = path
			res.Available = true
			res.Version, res.Error = version(ctx, path)
			break
		}
		if !res.Available {
			res.Error = "not found in PATH"
		}
		out = append(out, res)
	}
	return out
}

// SelectBest chooses a safe default for this host from detection results.
func SelectBest(results []Result) string {
	available := map[string]bool{}
	for _, r := range results {
		available[r.Name] = r.Available
	}
	if runtime.GOARCH == "arm64" {
		if available[ModeFEX] {
			return ModeFEX
		}
		if available[ModeBox64] {
			return ModeBox64
		}
	}
	if available[ModeQEMU] {
		return ModeQEMU
	}
	return ModeAuto
}

func version(ctx context.Context, path string) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	data, err := cmd.CombinedOutput()
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	if err != nil {
		if line != "" {
			return line, err.Error()
		}
		return "", err.Error()
	}
	return line, ""
}
