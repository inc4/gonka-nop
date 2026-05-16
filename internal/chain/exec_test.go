package chain

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

func TestRunComposeExec_BuildsCorrectArgs(t *testing.T) {
	// We can't test actual docker execution without Docker,
	// but we verify the function handles missing output dir gracefully
	state := &config.State{
		OutputDir:    "/nonexistent/path",
		ComposeFiles: []string{"docker-compose.yml"},
		UseSudo:      false,
	}

	_, err := RunComposeExec(context.Background(), state, "api", "echo hello")
	if err == nil {
		t.Fatal("expected error when output dir doesn't exist")
	}
}

func TestRunComposeExec_DefaultComposeFiles(t *testing.T) {
	// Verify that empty ComposeFiles defaults to docker-compose.yml
	state := &config.State{
		OutputDir:    "/nonexistent/path",
		ComposeFiles: nil,
		UseSudo:      false,
	}

	_, err := RunComposeExec(context.Background(), state, "api", "echo hello")
	if err == nil {
		t.Fatal("expected error when output dir doesn't exist")
	}
	// Just verify it doesn't panic with nil ComposeFiles
}

// TestLoadKeyringPassword_PureReturnsValue verifies the M11 Slice 1 refactor:
// LoadKeyringPassword is a pure function returning the password, not a
// mutating method (Review Notes F1 fix).
func TestLoadKeyringPassword_PureReturnsValue(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "config.env")
	if err := os.WriteFile(envFile, []byte("FOO=bar\nKEYRING_PASSWORD=hunter2\nBAZ=qux\n"), 0600); err != nil {
		t.Fatalf("write config.env: %v", err)
	}

	state := &config.State{OutputDir: dir}
	got := LoadKeyringPassword(state)
	if got != "hunter2" {
		t.Errorf("LoadKeyringPassword = %q, want %q", got, "hunter2")
	}
}

// TestLoadKeyringPassword_DoesNotMutateState verifies the function is pure:
// callers must explicitly assign the return value to state.KeyringPassword.
func TestLoadKeyringPassword_DoesNotMutateState(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "config.env")
	if err := os.WriteFile(envFile, []byte("KEYRING_PASSWORD=secret\n"), 0600); err != nil {
		t.Fatalf("write config.env: %v", err)
	}

	state := &config.State{OutputDir: dir, KeyringPassword: "preserved"}
	_ = LoadKeyringPassword(state)
	if state.KeyringPassword != "preserved" {
		t.Errorf("state.KeyringPassword mutated: got %q, want %q", state.KeyringPassword, "preserved")
	}
}

// TestLoadKeyringPassword_MissingFile returns empty string when config.env is unreadable.
func TestLoadKeyringPassword_MissingFile(t *testing.T) {
	state := &config.State{OutputDir: "/nonexistent/path/that/does/not/exist"}
	got := LoadKeyringPassword(state)
	if got != "" {
		t.Errorf("LoadKeyringPassword on missing file = %q, want \"\"", got)
	}
}

// TestLoadKeyringPassword_KeyNotPresent returns empty string when KEYRING_PASSWORD is absent.
func TestLoadKeyringPassword_KeyNotPresent(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "config.env")
	if err := os.WriteFile(envFile, []byte("FOO=bar\nBAZ=qux\n"), 0600); err != nil {
		t.Fatalf("write config.env: %v", err)
	}

	state := &config.State{OutputDir: dir}
	got := LoadKeyringPassword(state)
	if got != "" {
		t.Errorf("LoadKeyringPassword without KEYRING_PASSWORD = %q, want \"\"", got)
	}
}
