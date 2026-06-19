package podman

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SecretManager struct {
	mu      sync.RWMutex
	secrets map[string]*Secret
	store   string
	key     []byte
}

func NewSecretManager(root string) *SecretManager {
	sm := &SecretManager{
		secrets: make(map[string]*Secret),
		store:   filepath.Join(root, "secrets"),
	}
	_ = os.MkdirAll(sm.store, 0700)
	_ = os.MkdirAll(filepath.Join(sm.store, "names"), 0700)
	sm.loadKey()
	sm.loadSecrets()
	return sm
}

func (sm *SecretManager) Create(name string, data []byte, driver string, labels map[string]string) (*Secret, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.secrets[name]; exists {
		return nil, fmt.Errorf("secret %q already exists", name)
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

	if err := os.WriteFile(filepath.Join(sm.store, secret.ID+".enc"), encrypted, 0600); err != nil {
		return nil, fmt.Errorf("write secret data: %w", err)
	}

	meta, _ := json.MarshalIndent(secret, "", "  ")
	if err := os.WriteFile(filepath.Join(sm.store, secret.ID+".json"), meta, 0600); err != nil {
		return nil, fmt.Errorf("write secret metadata: %w", err)
	}

	_ = os.Symlink(secret.ID+".json", filepath.Join(sm.store, "names", name))

	sm.secrets[secret.ID] = secret
	return secret, nil
}

func (sm *SecretManager) Get(nameOrID string) (*Secret, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if s, ok := sm.secrets[nameOrID]; ok {
		return s, nil
	}
	for _, s := range sm.secrets {
		if s.Spec.Name == nameOrID {
			return s, nil
		}
	}
	return nil, fmt.Errorf("secret %s not found", nameOrID)
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
		result = append(result, s)
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

	_ = os.Remove(filepath.Join(sm.store, secret.ID+".enc"))
	_ = os.Remove(filepath.Join(sm.store, secret.ID+".json"))
	_ = os.Remove(filepath.Join(sm.store, "names", secret.Spec.Name))
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

func (sm *SecretManager) loadKey() {
	keyPath := filepath.Join(sm.store, ".key")
	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) == 32 {
		sm.key = data
		return
	}
	sm.key = make([]byte, 32)
	rand.Read(sm.key)
	_ = os.WriteFile(keyPath, sm.key, 0600)
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
		return
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sm.store, entry.Name()))
		if err != nil {
			continue
		}
		var secret Secret
		if err := json.Unmarshal(data, &secret); err != nil {
			continue
		}
		sm.secrets[secret.ID] = &secret
	}
}
