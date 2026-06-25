package emulation

import (
	"path/filepath"
	"testing"
)

func TestNormalizeMode(t *testing.T) {
	tests := map[string]string{
		"":               "auto",
		"auto":           "auto",
		"qemu-user":      "qemu",
		"FEXInterpreter": "fex",
		"box64":          "box64",
	}
	for input, want := range tests {
		got, ok := NormalizeMode(input)
		if !ok || got != want {
			t.Fatalf("NormalizeMode(%q) = %q,%v want %q,true", input, got, ok, want)
		}
	}
	if _, ok := NormalizeMode("bad"); ok {
		t.Fatal("NormalizeMode(bad) succeeded")
	}
}

func TestSaveLoadPreferredMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := Save(&Config{Preferred: "qemu-user"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Preferred != ModeQEMU {
		t.Fatalf("Preferred = %q, want qemu", cfg.Preferred)
	}
	if ConfigPath() != filepath.Join(home, ".doki", "emulation.json") {
		t.Fatalf("ConfigPath() = %q", ConfigPath())
	}
}

func TestPreferredModeEnvWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DOKI_EMULATION_MODE", "fex")
	if got := PreferredMode(); got != ModeFEX {
		t.Fatalf("PreferredMode() = %q, want fex", got)
	}
}

func TestSelectBest(t *testing.T) {
	if got := SelectBest([]Result{{Name: ModeQEMU, Available: true}}); got != ModeQEMU {
		t.Fatalf("SelectBest(qemu) = %q", got)
	}
	if got := SelectBest(nil); got != ModeAuto {
		t.Fatalf("SelectBest(nil) = %q", got)
	}
}
