package common

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

// EnsureDir creates a directory if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// PathExists checks if a path exists.
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ResolvePath resolves a path relative to the Doki data directory.
func ResolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

// CommandExists checks if a command is available in PATH.
func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// TrimQuotes removes surrounding quotes from a string.
func TrimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// CopyDir copies a directory recursively.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return EnsureDir(target)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, _ := d.Info()
		mode := os.FileMode(0644)
		if info != nil {
			mode = info.Mode()
		}
		return os.WriteFile(target, data, mode)
	})
}

// WriteFileSafe writes data to a file, creating parent directories.
func WriteFileSafe(path string, data []byte, mode os.FileMode) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

// ParseEnv parses a key=value environment variable.
func ParseEnv(env string) (key, value string) {
	parts := strings.SplitN(env, "=", 2)
	key = parts[0]
	if len(parts) > 1 {
		value = parts[1]
	}
	return
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
		if v == "" {
			delete(env, k)
		} else {
			env[k] = v
		}
	}
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

// ParseExtraHost parses an extra host entry.
func ParseExtraHost(entry string) (string, string) {
	parts := strings.SplitN(entry, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// ParsePortBinding parses a port binding string.
func ParsePortBinding(binding string) (Port, PortBinding) {
	var p Port
	var pb PortBinding
	p.Type = ProtocolTCP

	// Strip protocol suffix: "80/tcp" → port=80, proto=tcp
	rest := binding
	if idx := strings.Index(rest, "/"); idx > 0 {
		proto := rest[idx+1:]
		rest = rest[:idx]
		switch strings.ToLower(proto) {
		case "udp":
			p.Type = ProtocolUDP
		case "sctp":
			p.Type = ProtocolSCTP
		}
	}

	parts := strings.Split(rest, ":")
	switch len(parts) {
	case 1:
		p.PrivatePort = parsePort(parts[0])
	case 2:
		pb.HostPort = parts[0]
		p.PrivatePort = parsePort(parts[1])
	case 3:
		pb.HostIP = parts[0]
		pb.HostPort = parts[1]
		p.PrivatePort = parsePort(parts[2])
	}

	if pb.HostPort == "" {
		pb.HostPort = "0"
	}

	return p, pb
}

func parsePort(s string) uint16 {
	var port uint16
	for _, c := range s {
		if c >= '0' && c <= '9' {
			port = port*10 + uint16(c-'0')
		} else {
			break
		}
	}
	return port
}

// ArgsEscaped checks if command-line args are already escaped.
func ArgsEscaped(args []string) bool {
	for _, arg := range args {
		if strings.Contains(arg, " ") {
			return true
		}
	}
	return false
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
