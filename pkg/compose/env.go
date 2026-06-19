package compose

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// envVarPattern matches ${VAR}, ${VAR:-default}, ${VAR:+alt}, $VAR.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// SubstituteEnv replaces ${VAR}, ${VAR:-default}, ${VAR:+alt}, and $VAR
// in the input string with values from the provided environment map.
// If a variable is not found in the map, it falls back to os.Getenv.
func SubstituteEnv(input string, env map[string]string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		var varName, defaultVal, altVal string
		isDefault := false
		isAlt := false

		if match[0] == '$' && match[1] == '{' {
			// ${VAR} or ${VAR:-default} or ${VAR:+alt}
			inner := match[2 : len(match)-1]
			if idx := strings.Index(inner, ":-"); idx >= 0 {
				varName = inner[:idx]
				defaultVal = inner[idx+2:]
				isDefault = true
			} else if idx := strings.Index(inner, ":+"); idx >= 0 {
				varName = inner[:idx]
				altVal = inner[idx+2:]
				isAlt = true
			} else {
				varName = inner
			}
		} else {
			// $VAR
			varName = match[1:]
		}

		// Look up in env map first, then os.Getenv.
		val, ok := env[varName]
		if !ok {
			val = os.Getenv(varName)
			ok = val != ""
		}

		if isAlt {
			if ok && val != "" {
				return altVal
			}
			return ""
		}
		if isDefault {
			if ok && val != "" {
				return val
			}
			return defaultVal
		}
		if ok {
			return val
		}
		return match // Leave unresolved variables as-is.
	})
}

// SubstituteEnvInMap applies SubstituteEnv to all values in a map.
func SubstituteEnvInMap(m map[string]string, env map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = SubstituteEnv(v, env)
	}
	return result
}

// SubstituteEnvInSlice applies SubstituteEnv to all strings in a slice.
func SubstituteEnvInSlice(s []string, env map[string]string) []string {
	result := make([]string, len(s))
	for i, v := range s {
		result[i] = SubstituteEnv(v, env)
	}
	return result
}

// LoadEnvFile loads environment variables from a file.
// Format: KEY=VALUE (one per line). Lines starting with # are comments.
// Empty lines are ignored. Quotes around values are stripped.
func LoadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("env file %s line %d: invalid format (expected KEY=VALUE)", path, lineNum)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Strip surrounding quotes.
		val = stripQuotes(val)
		env[key] = val
	}
	return env, scanner.Err()
}

// LoadEnvFiles loads multiple env files and merges them.
// Later files override earlier ones.
func LoadEnvFiles(paths []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, path := range paths {
		env, err := LoadEnvFile(path)
		if err != nil {
			return nil, err
		}
		for k, v := range env {
			result[k] = v
		}
	}
	return result, nil
}

// MergeEnv merges multiple environment maps.
// Later maps override earlier ones.
func MergeEnv(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// ParseEnvSlice converts a slice of "KEY=VALUE" strings to a map.
func ParseEnvSlice(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		} else {
			result[parts[0]] = ""
		}
	}
	return result
}

// ToEnvSlice converts a map to a slice of "KEY=VALUE" strings.
func ToEnvSlice(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+"="+v)
	}
	return result
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
