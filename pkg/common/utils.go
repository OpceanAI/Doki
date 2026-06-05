package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
		// Fallback to weak random if crypto/rand fails
		for i := range b {
			b[i] = byte(time.Now().UnixNano() & 0xff)
			time.Sleep(1) // ensure nanosecond changes
		}
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

// termuxAndroidDenyList is the set of environment variable names that must be
// stripped from the guest environment when running on Termux / Android.
//
// These variables either:
//   - break proot's ptrace-based execve translation (LD_PRELOAD, LD_LIBRARY_PATH, …)
//   - leak Termux-specific paths that do not exist inside the guest (PREFIX, HOME, …)
//   - conflict with the guest distro's own conventions (TERMUX__PREFIX, ANDROID_DATA, …)
//
// See OpceanAI/Doki#4 for the post-mortem that produced this list.
var termuxAndroidDenyList = []string{
	// proot / ptrace killers — Termux-injected libtermux-exec.so LD_PRELOAD
	// intercepts execve and races with proot, producing ENOSYS on Android 15+.
	"LD_PRELOAD", "LD_PRELOAD32", "LD_PRELOAD64",
	"LD_LIBRARY_PATH", "LD_SHOW_AUXV",

	// Termux-specific vars that point to paths the guest can't see
	"TERMUX_VERSION",
	"TERMUX__PREFIX", "TERMUX__ROOTFS", "TERMUX__HOME",
	"PREFIX",

	// Android system vars that leak host metadata into the guest
	"ANDROID_ROOT", "ANDROID_DATA", "ANDROID_STORAGE",
	"ANDROID_PROPERTY_WORKSPACE",

	// Temp paths that point to Termux/Android locations
	"TMPDIR", "TMP", "TEMP",

	// HOME points at /data/data/com.termux/files/home in Termux; the guest
	// should default to /root. The AndroidEnv() helper sets the correct one.
	"HOME",
}

func isHostEnvDenied(name string) bool {
	for _, deny := range termuxAndroidDenyList {
		if name == deny {
			return true
		}
	}
	return false
}

// StripHostEnv removes host-specific environment variables that interfere with
// proot (ptrace-based container runtime). Notably LD_PRELOAD from Termux
// hooks execve and breaks proot's ptrace mechanism, and several other
// Termux/Android variables leak host paths into the guest.
//
// The full deny-list lives in termuxAndroidDenyList. On non-Android hosts the
// deny-list is harmless because none of the variable names are typically set.
func StripHostEnv(env []string) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		eq := strings.IndexByte(e, '=')
		var name string
		if eq > 0 {
			name = e[:eq]
		} else {
			name = e
		}
		if isHostEnvDenied(name) {
			continue
		}
		result = append(result, e)
	}
	return result
}

// StripHostEnvFromOS is a convenience that calls StripHostEnv on the current
// process environment. It also unsets the denied variables in the process
// itself so that any child process spawned via exec.Command inherits a clean
// environment unless cmd.Env is explicitly set.
func StripHostEnvFromOS() []string {
	for _, deny := range termuxAndroidDenyList {
		_ = os.Unsetenv(deny)
	}
	return StripHostEnv(os.Environ())
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
	if err := EnsureDir(dst); err != nil {
		return err
	}
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
func ParsePortBinding(binding string) (Port, PortBinding, error) {
	port := Port{Type: ProtocolTCP}
	bind := PortBinding{HostIP: "0.0.0.0"}

	if strings.Contains(binding, "..") {
		return port, bind, fmt.Errorf("invalid port binding: %s", binding)
	}

	parts := strings.Split(binding, ":")
	if len(parts) > 3 {
		return port, bind, fmt.Errorf("invalid port binding format: %s", binding)
	}

	switch len(parts) {
	case 1:
		pp := strings.Split(parts[0], "/")
		p, err := parsePort(pp[0])
		if err != nil {
			return port, bind, fmt.Errorf("invalid port: %s", pp[0])
		}
		if p == 0 {
			return port, bind, fmt.Errorf("invalid port: %s", pp[0])
		}
		port.PrivatePort = p
		port.PublicPort = p
		if len(pp) > 1 {
			if strings.ToLower(pp[1]) == "udp" {
				port.Type = ProtocolUDP
			}
		}
	case 2:
		cport := strings.Split(parts[1], "/")
		cp, err := parsePort(cport[0])
		if err != nil {
			return port, bind, fmt.Errorf("invalid container port: %s", cport[0])
		}
		if cp == 0 {
			return port, bind, fmt.Errorf("invalid container port: %s", cport[0])
		}
		port.PrivatePort = cp

		hp, err := parsePort(parts[0])
		if err != nil {
			return port, bind, fmt.Errorf("invalid host port: %s", parts[0])
		}
		port.PublicPort = hp
		bind.HostPort = fmt.Sprintf("%d", hp)
		if len(cport) > 1 && strings.ToLower(cport[1]) == "udp" {
			port.Type = ProtocolUDP
		}
	case 3:
		bind.HostIP = parts[0]

		hp, err := parsePort(parts[1])
		if err != nil {
			return port, bind, fmt.Errorf("invalid host port: %s", parts[1])
		}
		port.PublicPort = hp
		bind.HostPort = fmt.Sprintf("%d", hp)

		cport := strings.Split(parts[2], "/")
		cp, err := parsePort(cport[0])
		if err != nil {
			return port, bind, fmt.Errorf("invalid container port: %s", cport[0])
		}
		if cp == 0 {
			return port, bind, fmt.Errorf("invalid container port: %s", cport[0])
		}
		port.PrivatePort = cp
		if len(cport) > 1 && strings.ToLower(cport[1]) == "udp" {
			port.Type = ProtocolUDP
		}
	}
	return port, bind, nil
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
