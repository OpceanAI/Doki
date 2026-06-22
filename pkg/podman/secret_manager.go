package podman

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// validateSecretName rejects names that could escape the secret store
// directory via path traversal (e.g. "../etc/passwd"). This prevents
// arbitrary file write/delete through symlink creation.
func validateSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("secret name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("secret name %q contains path separator", name)
	}
	if name == "." || name == ".." || strings.Contains(name, "..") {
		return fmt.Errorf("secret name %q contains path traversal sequence", name)
	}
	if !common.ValidContainerName(name) {
		return fmt.Errorf("secret name %q contains invalid characters", name)
	}
	return nil
}

type SecretManager struct {
	mu      sync.RWMutex
	secrets map[string]*Secret
	store   string
	key     []byte
}

func NewSecretManager(root string) (*SecretManager, error) {
	sm := &SecretManager{
		secrets: make(map[string]*Secret),
		store:   filepath.Join(root, "secrets"),
	}
	if err := os.MkdirAll(sm.store, 0700); err != nil {
		return nil, fmt.Errorf("podman: create secret store %s: %w", sm.store, err)
	}
	if err := os.MkdirAll(filepath.Join(sm.store, "names"), 0700); err != nil {
		return nil, fmt.Errorf("podman: create names dir: %w", err)
	}
	if err := sm.loadKey(); err != nil {
		return nil, fmt.Errorf("podman: load secret key: %w", err)
	}
	sm.loadSecrets()
	return sm, nil
}

func (sm *SecretManager) Create(name string, data []byte, driver string, labels map[string]string) (*Secret, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if err := validateSecretName(name); err != nil {
		return nil, err
	}

	for _, s := range sm.secrets {
		if s.Spec.Name == name {
			return nil, fmt.Errorf("secret %q already exists", name)
		}
	}

	encrypted, err := sm.encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}

	secret := &Secret{
		ID:      generateID(),
		Spec:    SecretSpec{Name: name, Labels: labels, Driver: driver},
		Created: time.Now(),
		Updated: time.Now(),
	}

	linkPath := filepath.Join(sm.store, "names", name)
	if _, err := os.Lstat(linkPath); err == nil {
		return nil, fmt.Errorf("secret %q already exists", name)
	}
	if err := os.Symlink(secret.ID+".json", linkPath); err != nil {
		return nil, fmt.Errorf("create name link: %w", err)
	}

	cleanup := func() {
		_ = os.Remove(filepath.Join(sm.store, secret.ID+".enc"))
		_ = os.Remove(filepath.Join(sm.store, secret.ID+".json"))
		_ = os.Remove(linkPath)
	}

	dataPath := filepath.Join(sm.store, secret.ID+".enc")
	if err := os.WriteFile(dataPath, encrypted, 0600); err != nil {
		cleanup()
		return nil, fmt.Errorf("write secret data: %w", err)
	}

	meta, err := json.MarshalIndent(secret, "", "  ")
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("marshal secret metadata: %w", err)
	}
	metaPath := filepath.Join(sm.store, secret.ID+".json")
	if err := os.WriteFile(metaPath, meta, 0600); err != nil {
		cleanup()
		return nil, fmt.Errorf("write secret metadata: %w", err)
	}

	sm.secrets[secret.ID] = secret
	return secret, nil
}

func (sm *SecretManager) Get(nameOrID string) (*Secret, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var s *Secret
	if sec, ok := sm.secrets[nameOrID]; ok {
		s = sec
	} else {
		for _, sec := range sm.secrets {
			if sec.Spec.Name == nameOrID {
				s = sec
				break
			}
		}
	}
	if s == nil {
		return nil, fmt.Errorf("secret %s not found", nameOrID)
	}
	// Return a deep copy to avoid data race when caller marshals or
	// mutates the secret outside the lock.
	cp := *s
	return &cp, nil
}

func (sm *SecretManager) GetData(nameOrID string) ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var secret *Secret
	if s, ok := sm.secrets[nameOrID]; ok {
		secret = s
	} else {
		for _, s := range sm.secrets {
			if s.Spec.Name == nameOrID {
				secret = s
				break
			}
		}
	}
	if secret == nil {
		return nil, fmt.Errorf("secret %s not found", nameOrID)
	}

	encrypted, err := os.ReadFile(filepath.Join(sm.store, secret.ID+".enc"))
	if err != nil {
		return nil, fmt.Errorf("read secret data: %w", err)
	}

	return sm.decrypt(encrypted)
}

func (sm *SecretManager) ListSecrets() []*Secret {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]*Secret, 0, len(sm.secrets))
	for _, s := range sm.secrets {
		cp := *s
		result = append(result, &cp)
	}
	return result
}

func (sm *SecretManager) Remove(nameOrID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var secret *Secret
	if s, ok := sm.secrets[nameOrID]; ok {
		secret = s
	} else {
		for _, s := range sm.secrets {
			if s.Spec.Name == nameOrID {
				secret = s
				break
			}
		}
	}
	if secret == nil {
		return fmt.Errorf("secret %s not found", nameOrID)
	}

	// Validate the stored name before using it in a path, as defense
	// in depth against corrupted on-disk data.
	if err := validateSecretName(secret.Spec.Name); err != nil {
		return fmt.Errorf("remove secret: %w", err)
	}

	if err := os.Remove(filepath.Join(sm.store, secret.ID+".enc")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove secret data: %w", err)
	}
	if err := os.Remove(filepath.Join(sm.store, secret.ID+".json")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove secret metadata: %w", err)
	}
	if err := os.Remove(filepath.Join(sm.store, "names", secret.Spec.Name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove secret name link: %w", err)
	}
	delete(sm.secrets, secret.ID)
	return nil
}

func (sm *SecretManager) Exists(nameOrID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if _, ok := sm.secrets[nameOrID]; ok {
		return true
	}
	for _, s := range sm.secrets {
		if s.Spec.Name == nameOrID {
			return true
		}
	}
	return false
}

func (sm *SecretManager) loadKey() error {
	keyPath := filepath.Join(sm.store, ".key")
	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) == 32 {
		sm.key = data
		return nil
	}
	sm.key = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, sm.key); err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	if err := os.WriteFile(keyPath, sm.key, 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	if info, err := os.Stat(sm.store); err == nil {
		if info.Mode().Perm()&0077 != 0 {
			if err := os.Chmod(sm.store, 0700); err != nil {
				log.Printf("podman: tighten secret store perms: %v", err)
			}
		}
	}
	return nil
}

func (sm *SecretManager) encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(sm.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, data, nil), nil
}

func (sm *SecretManager) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(sm.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (sm *SecretManager) loadSecrets() {
	entries, err := os.ReadDir(sm.store)
	if err != nil {
		log.Printf("podman: read secret store %s: %v", sm.store, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(sm.store, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("podman: read secret file %s: %v", path, err)
			continue
		}
		var secret Secret
		if err := json.Unmarshal(data, &secret); err != nil {
			log.Printf("podman: parse secret file %s: %v", path, err)
			continue
		}
		sm.secrets[secret.ID] = &secret
	}
}
