package common

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GenerateID generates a random hex ID of the given length.
func GenerateID(length int) string {
	b := make([]byte, length/2)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		// Fallback: use time-based random if crypto/rand fails.
		return fallbackID(length)
	}
	return hex.EncodeToString(b)
}

func fallbackID(length int) string {
	b := make([]byte, length/2)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>(i*4)) ^ byte(i)
	}
	return hex.EncodeToString(b)
}

// ContainerID generates a 64-character container ID.
func ContainerID() string {
	return GenerateID(64)
}

// ShortID truncates an ID to 12 characters.
func ShortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// ValidContainerName validates a container name.
var validContainerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func ValidContainerName(name string) bool {
	return validContainerName.MatchString(name)
}

// TimeToTimestamp converts time.Time to Unix timestamp.
func TimeToTimestamp(t time.Time) int64 {
	return t.Unix()
}

// NowTimestamp returns the current Unix timestamp.
func NowTimestamp() int64 {
	return time.Now().Unix()
}

// ContainsString checks if a slice contains a string.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveString removes a string from a slice.
func RemoveString(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// ValidateEnvVar checks if an environment variable name is valid per POSIX.
func ValidateEnvVar(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// ValidateEnv validates env vars and applies size limits.
func ValidateEnv(env []string) []string {
	result := make([]string, 0, len(env))
	totalSize := 0
	maxSize := 128 * 1024 // 128KB
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 && ValidateEnvVar(parts[0]) && totalSize < maxSize {
			result = append(result, e)
			totalSize += len(e)
		}
	}
	return result
}

// StringIntern is a simple interning pool for path deduplication.
var stringIntern = struct {
	mu   sync.RWMutex
	pool map[string]string
}{pool: make(map[string]string, 256)}

func InternString(s string) string {
	if s == "" {
		return s
	}
	stringIntern.mu.RLock()
	interned, ok := stringIntern.pool[s]
	stringIntern.mu.RUnlock()
	if ok {
		return interned
	}
	stringIntern.mu.Lock()
	stringIntern.pool[s] = s
	stringIntern.mu.Unlock()
	return s
}

// SplitStrSlice splits a string by commas and trims whitespace.
func SplitStrSlice(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// EnsureDir creates a directory if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// PathExists checks if a path exists on the filesystem.
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ResolvePath resolves a path relative to a base directory.
func ResolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

// CommandExists checks if a command exists in PATH.
func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// CopyDir recursively copies a directory.
func CopyDir(src, dst string) error {
	EnsureDir(dst)
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteFileSafe writes content to a file, creating parent directories.
func WriteFileSafe(path, content string, mode os.FileMode) error {
	EnsureDir(filepath.Dir(path))
	return os.WriteFile(path, []byte(content), mode)
}

// ParseEnv splits an environment variable into key and value.
func ParseEnv(env string) (string, string) {
	parts := strings.SplitN(env, "=", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return env, ""
}

// MergeEnv merges two environment slices.
func MergeEnv(base, overrides []string) []string {
	env := make(map[string]string)
	for _, e := range base {
		k, v := ParseEnv(e)
		env[k] = v
	}
	for _, e := range overrides {
		k, v := ParseEnv(e)
		env[k] = v
	}
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

// ParsePortBinding parses a Docker port binding string like "8080:80/tcp".
func ParsePortBinding(binding string) (Port, PortBinding) {
	port := Port{Type: ProtocolTCP}
	bind := PortBinding{HostIP: "0.0.0.0"}
	parts := strings.Split(binding, ":")
	switch len(parts) {
	case 1:
		pp := strings.Split(parts[0], "/")
		p, _ := parsePort(pp[0])
		port.PrivatePort = p
		if len(pp) > 1 {
			if strings.ToLower(pp[1]) == "udp" {
				port.Type = ProtocolUDP
			}
		}
		port.PublicPort = p
	case 2:
		cport := strings.Split(parts[1], "/")
		cp, _ := parsePort(cport[0])
		port.PrivatePort = cp
		hp, _ := parsePort(parts[0])
		port.PublicPort = hp
		if len(cport) > 1 && strings.ToLower(cport[1]) == "udp" {
			port.Type = ProtocolUDP
		}
	case 3:
		hp, _ := parsePort(parts[1])
		port.PublicPort = hp
		cport := strings.Split(parts[2], "/")
		cp, _ := parsePort(cport[0])
		port.PrivatePort = cp
		bind.HostIP = parts[0]
		if len(cport) > 1 && strings.ToLower(cport[1]) == "udp" {
			port.Type = ProtocolUDP
		}
	}
	return port, bind
}

func parsePort(s string) (uint16, error) {
	p, err := strconv.ParseUint(s, 10, 16)
	return uint16(p), err
}

// TrimQuotes strips surrounding quotes from a string.
func TrimQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

// ArgsEscaped checks if command args need shell escaping.
func ArgsEscaped(args []string) bool {
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			return true
		}
	}
	return false
}
