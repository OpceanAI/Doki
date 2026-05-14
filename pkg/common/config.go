package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
)

const (
	// DefaultConfigDir is where Doki config is stored.
	DefaultConfigDir = ".doki"
	// ConfigFileName is the name of the config file.
	ConfigFileName = "config.json"
	// DefaultSocketName is the default unix socket name.
	DefaultSocketName = "doki.sock"
)

// LoadConfig loads the Doki configuration from disk.
func LoadConfig() (*DokiConfig, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}

	configPath := filepath.Join(home, DefaultConfigDir, ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveConfig saves the Doki configuration to disk.
func SaveConfig(cfg *DokiConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, DefaultConfigDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// DataDir returns the Doki data directory.
func DataDir() string {
	if dir := os.Getenv("DOKI_DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, DefaultConfigDir, "data")
}

// ImageDir returns the Doki image directory.
func ImageDir() string {
	return filepath.Join(DataDir(), "images")
}

// ContainerDir returns the Doki container directory.
func ContainerDir() string {
	return filepath.Join(DataDir(), "containers")
}

// VolumeDir returns the Doki volume directory.
func VolumeDir() string {
	return filepath.Join(DataDir(), "volumes")
}

// NetworkDir returns the Doki network directory.
func NetworkDir() string {
	return filepath.Join(DataDir(), "networks")
}

// RuntimeDir returns the Doki runtime directory.
func RuntimeDir() string {
	return filepath.Join(DataDir(), "runtimes")
}

// OSType returns the operating system type.
func OSType() string {
	return goruntime.GOOS
}

func isAndroid() bool {
	_, err := os.Stat("/system/build.prop")
	return err == nil
}

func isMacOS() bool {
	return goruntime.GOOS == "darwin"
}

func isLinux() bool {
	return goruntime.GOOS == "linux"
}

// HasSystemd checks if the system uses systemd as init.
func HasSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// SystemdListenFDs returns file descriptors passed by systemd socket activation.
func SystemdListenFDs() []*os.File {
	pid, _ := strconv.Atoi(os.Getenv("LISTEN_PID"))
	if pid != os.Getpid() {
		return nil
	}
	nfds, _ := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if nfds <= 0 {
		return nil
	}
	fds := make([]*os.File, nfds)
	for i := 0; i < nfds; i++ {
		fds[i] = os.NewFile(uintptr(3+i), fmt.Sprintf("systemd-fd-%d", i))
	}
	return fds
}
