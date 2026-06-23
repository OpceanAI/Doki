package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigWritesPrivateFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	cfg.DataDir = filepath.Join(home, "data")
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	configPath := filepath.Join(home, DefaultConfigDir, ConfigFileName)
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("config mode = %v, want 0600", got)
	}
	if _, err := os.Stat(configPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary config file still exists: %v", err)
	}
}

func TestDefaultConfigUsesSharedPlatformPaths(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DataDir != AppDataDir() {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, AppDataDir())
	}
	if cfg.Root != cfg.DataDir {
		t.Fatalf("Root = %q, want DataDir %q", cfg.Root, cfg.DataDir)
	}
	if cfg.ExecRoot != filepath.Join(cfg.DataDir, "runtimes") {
		t.Fatalf("ExecRoot = %q, want runtimes under DataDir", cfg.ExecRoot)
	}
	if cfg.SocketPath == "" {
		t.Fatal("SocketPath must not be empty")
	}
}
