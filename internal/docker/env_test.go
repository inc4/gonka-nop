package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	content := `# This is a comment
CHAIN_ID=gonka-testnet
MODEL_NAME="Qwen/Qwen3-235B-A22B-Instruct-2507-FP8"
IS_TEST_NET=true

# Another comment
PORT=8000
SINGLE_QUOTED='hello world'
EMPTY_VALUE=
WITH_EQUALS=key=value=more
`
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, "config.env")
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	env, err := ParseEnvFile(envPath)
	if err != nil {
		t.Fatalf("ParseEnvFile() error: %v", err)
	}

	expected := map[string]string{
		"CHAIN_ID":      "gonka-testnet",
		"MODEL_NAME":    "Qwen/Qwen3-235B-A22B-Instruct-2507-FP8",
		"IS_TEST_NET":   "true",
		"PORT":          "8000",
		"SINGLE_QUOTED": "hello world",
		"EMPTY_VALUE":   "",
		"WITH_EQUALS":   "key=value=more",
	}

	parsed := envToMap(env)
	for key, want := range expected {
		got, ok := parsed[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		if got != want {
			t.Errorf("key %q: got %q, want %q", key, got, want)
		}
	}

	// Should NOT contain comment lines
	for _, e := range env {
		if strings.HasPrefix(e, "#") {
			t.Errorf("comment included in output: %q", e)
		}
	}
}

func TestParseEnvFile_NotFound(t *testing.T) {
	_, err := ParseEnvFile("/nonexistent/config.env")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestParseEnvFile_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, "config.env")
	if err := os.WriteFile(envPath, []byte(""), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	env, err := ParseEnvFile(envPath)
	if err != nil {
		t.Fatalf("ParseEnvFile() error: %v", err)
	}
	if len(env) != 0 {
		t.Errorf("expected 0 entries, got %d", len(env))
	}
}

func TestParseEnvFile_CommentsOnly(t *testing.T) {
	content := "# comment1\n# comment2\n"
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, "config.env")
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	env, err := ParseEnvFile(envPath)
	if err != nil {
		t.Fatalf("ParseEnvFile() error: %v", err)
	}
	if len(env) != 0 {
		t.Errorf("expected 0 entries for comments-only file, got %d", len(env))
	}
}

func TestParseEnvFile_NoEquals(t *testing.T) {
	content := "INVALID_LINE\nVALID=yes\n"
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, "config.env")
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	env, err := ParseEnvFile(envPath)
	if err != nil {
		t.Fatalf("ParseEnvFile() error: %v", err)
	}
	if len(env) != 1 {
		t.Errorf("expected 1 entry, got %d", len(env))
	}
}

func TestMergeEnv(t *testing.T) {
	fileEnv := []string{
		"CUSTOM_VAR=hello",
		"PATH=/custom/path",
	}

	merged := MergeEnv(fileEnv)

	m := envToMap(merged)

	// File vars should be present
	if m["CUSTOM_VAR"] != "hello" {
		t.Errorf("CUSTOM_VAR: got %q, want %q", m["CUSTOM_VAR"], "hello")
	}

	// File PATH should override system PATH
	if m["PATH"] != "/custom/path" {
		t.Errorf("PATH should be overridden: got %q, want %q", m["PATH"], "/custom/path")
	}

	// System env vars should still be present (HOME should exist on any system)
	if _, ok := m["HOME"]; !ok {
		// HOME might not exist in all CI environments, so just check we have some system vars
		if len(merged) < len(fileEnv) {
			t.Error("merged env should include system vars")
		}
	}
}

func TestMergeEnv_Empty(t *testing.T) {
	merged := MergeEnv(nil)
	if len(merged) == 0 {
		t.Error("merged env with nil fileEnv should still contain system env")
	}
}

func TestStripQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"hello"`, "hello"},
		{`'hello'`, "hello"},
		{`hello`, "hello"},
		{`""`, ""},
		{`''`, ""},
		{`"`, `"`},
		{``, ``},
		{`"mixed'`, `"mixed'`},
	}

	for _, tt := range tests {
		got := stripQuotes(tt.input)
		if got != tt.want {
			t.Errorf("stripQuotes(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// envToMap converts []string{"K=V"} to map[string]string.
func envToMap(env []string) map[string]string {
	m := make(map[string]string)
	for _, e := range env {
		if idx := strings.Index(e, "="); idx >= 0 {
			m[e[:idx]] = e[idx+1:]
		}
	}
	return m
}
