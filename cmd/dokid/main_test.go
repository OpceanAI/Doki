package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpceanAI/Doki/pkg/common"
)

func TestLoadConfigEnvOverridesFile(t *testing.T) {
	oldSocketPath, oldConfigPath, oldLogLevel := socketPath, configPath, logLevel
	defer func() {
		socketPath, configPath, logLevel = oldSocketPath, oldConfigPath, oldLogLevel
	}()
	socketPath, configPath, logLevel = "", "", ""

	home := t.TempDir()
	t.Setenv("HOME", home)
	fileDataDir := filepath.Join(home, "from-file")
	envDataDir := filepath.Join(home, "from-env")

	cfg := common.DefaultConfig()
	cfg.DataDir = fileDataDir
	cfg.Root = fileDataDir
	cfg.ExecRoot = filepath.Join(fileDataDir, "runtimes")
	if err := common.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	t.Setenv("DOKI_DATA_DIR", envDataDir)

	loaded := loadConfig()
	if loaded.DataDir != envDataDir {
		t.Fatalf("DataDir = %q, want env override %q", loaded.DataDir, envDataDir)
	}
	if loaded.Root != envDataDir {
		t.Fatalf("Root = %q, want env override %q", loaded.Root, envDataDir)
	}
	if loaded.ExecRoot != filepath.Join(envDataDir, "runtimes") {
		t.Fatalf("ExecRoot = %q, want runtimes under env data dir", loaded.ExecRoot)
	}
}

func TestApplyConfigOverrides(t *testing.T) {
	oldSocketPath, oldLogLevel := socketPath, logLevel
	defer func() { socketPath, logLevel = oldSocketPath, oldLogLevel }()

	cfg := common.DefaultConfig()
	overrideDir := filepath.Join(t.TempDir(), "data")
	socketPath = filepath.Join(t.TempDir(), "doki.sock")
	logLevel = "debug"
	t.Setenv("DOKI_DATA_DIR", overrideDir)
	t.Setenv("DOKI_STORAGE_DRIVER", "overlayfs")

	applyConfigOverrides(cfg)
	if cfg.SocketPath != socketPath {
		t.Fatalf("SocketPath = %q, want %q", cfg.SocketPath, socketPath)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.DataDir != overrideDir || cfg.Root != overrideDir {
		t.Fatalf("data/root overrides not applied: data=%q root=%q", cfg.DataDir, cfg.Root)
	}
	if cfg.StorageDriver != "overlayfs" {
		t.Fatalf("StorageDriver = %q, want overlayfs", cfg.StorageDriver)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
