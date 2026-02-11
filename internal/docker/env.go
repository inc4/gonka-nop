package docker

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ParseEnvFile reads a config.env file and returns key=value pairs.
// Ignores comments (#) and empty lines. Strips surrounding quotes.
func ParseEnvFile(path string) ([]string, error) {
	f, err := os.Open(path) // #nosec G304 - path from trusted config
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var env []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Must contain = to be a valid env var
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}

		key := line[:idx]
		value := line[idx+1:]

		// Strip surrounding quotes (single or double)
		value = stripQuotes(value)

		env = append(env, key+"="+value)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading env file: %w", err)
	}

	return env, nil
}

// MergeEnv merges parsed env vars with os.Environ(),
// with file vars taking precedence.
func MergeEnv(fileEnv []string) []string {
	// Build map from os env
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		if idx := strings.Index(e, "="); idx >= 0 {
			envMap[e[:idx]] = e[idx+1:]
		}
	}

	// Override with file env
	for _, e := range fileEnv {
		if idx := strings.Index(e, "="); idx >= 0 {
			envMap[e[:idx]] = e[idx+1:]
		}
	}

	// Convert back to slice
	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, k+"="+v)
	}
	return result
}

// stripQuotes removes surrounding single or double quotes from a value.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
