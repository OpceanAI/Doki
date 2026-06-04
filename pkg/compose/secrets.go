package compose

import (
	"fmt"
	"os"
	"path/filepath"
)

// SecretManager manages compose secrets.
type SecretManager struct {
	projectDir string
	secrets    map[string]*Secret
}

// NewSecretManager creates a new secret manager.
func NewSecretManager(projectDir string, secrets map[string]*Secret) *SecretManager {
	return &SecretManager{
		projectDir: projectDir,
		secrets:    secrets,
	}
}

// GetSecretPath returns the path to a secret file.
func (sm *SecretManager) GetSecretPath(name string) (string, error) {
	secret, ok := sm.secrets[name]
	if !ok {
		return "", fmt.Errorf("secret %q not defined", name)
	}
	if secret.External {
		// External secrets are managed outside compose.
		return "", fmt.Errorf("secret %q is external", name)
	}
	if secret.File == "" {
		return "", fmt.Errorf("secret %q has no file defined", name)
	}
	path := secret.File
	if !filepath.IsAbs(path) {
		path = filepath.Join(sm.projectDir, path)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("secret %q file not found: %s", name, path)
	}
	return path, nil
}

// MountSecret mounts a secret into a container directory.
func (sm *SecretManager) MountSecret(name, destDir string) error {
	srcPath, err := sm.GetSecretPath(name)
	if err != nil {
		return err
	}
	destPath := filepath.Join(destDir, name)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read secret %s: %w", name, err)
	}
	return os.WriteFile(destPath, data, 0400)
}

// ConfigManager manages compose configs.
type ConfigManager struct {
	projectDir string
	configs    map[string]*Config
}

// NewConfigManager creates a new config manager.
func NewConfigManager(projectDir string, configs map[string]*Config) *ConfigManager {
	return &ConfigManager{
		projectDir: projectDir,
		configs:    configs,
	}
}

// GetConfigPath returns the path to a config file.
func (cm *ConfigManager) GetConfigPath(name string) (string, error) {
	cfg, ok := cm.configs[name]
	if !ok {
		return "", fmt.Errorf("config %q not defined", name)
	}
	if cfg.External {
		return "", fmt.Errorf("config %q is external", name)
	}
	if cfg.File == "" {
		return "", fmt.Errorf("config %q has no file defined", name)
	}
	path := cfg.File
	if !filepath.IsAbs(path) {
		path = filepath.Join(cm.projectDir, path)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("config %q file not found: %s", name, path)
	}
	return path, nil
}

// MountConfig mounts a config into a container directory.
func (cm *ConfigManager) MountConfig(name, destDir string) error {
	srcPath, err := cm.GetConfigPath(name)
	if err != nil {
		return err
	}
	destPath := filepath.Join(destDir, name)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read config %s: %w", name, err)
	}
	return os.WriteFile(destPath, data, 0444)
}

// MountAllSecrets mounts all secrets for a service.
func (sm *SecretManager) MountAllSecrets(svcSecrets []string, destDir string) error {
	for _, name := range svcSecrets {
		if err := sm.MountSecret(name, destDir); err != nil {
			return err
		}
	}
	return nil
}

// MountAllConfigs mounts all configs for a service.
func (cm *ConfigManager) MountAllConfigs(svcConfigs []string, destDir string) error {
	for _, name := range svcConfigs {
		if err := cm.MountConfig(name, destDir); err != nil {
			return err
		}
	}
	return nil
}
