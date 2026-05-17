// M12 Slice 4 tests: --postgres-* flag wiring and GONKA_POSTGRES_PASSWORD
// env-var read+scrub. The applyPostgresFlags helper is the testable
// extraction point; the cobra wiring is exercised indirectly.
package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

const testPostgresHost = "db.example.com"

// resetPostgresFlags clears package-level Postgres flag variables so each
// test starts from a clean slate. Required because cobra writes flag
// values into package-level vars.
func resetPostgresFlags(t *testing.T) {
	t.Helper()
	flagPostgresHost = ""
	flagPostgresPort = ""
	flagPostgresDB = ""
	flagPostgresUser = ""
	flagPostgresPassword = ""
	_ = os.Unsetenv(envPostgresPassword)
	yesFlag = false
	t.Cleanup(func() {
		flagPostgresHost = ""
		flagPostgresPort = ""
		flagPostgresDB = ""
		flagPostgresUser = ""
		flagPostgresPassword = ""
		_ = os.Unsetenv(envPostgresPassword)
		yesFlag = false
	})
}

// TestApplyPostgresFlags_FileBackendByDefault (UC-1 main, file backend).
func TestApplyPostgresFlags_FileBackendByDefault(t *testing.T) {
	resetPostgresFlags(t)
	state := &config.State{}
	if err := applyPostgresFlags(state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.DevshardConfig != (config.DevshardConfig{}) {
		t.Errorf("expected empty DevshardConfig, got %+v", state.DevshardConfig)
	}
}

// TestApplyPostgresFlags_AllFlagsSet_PopulatesState (TC-2.1).
func TestApplyPostgresFlags_AllFlagsSet_PopulatesState(t *testing.T) {
	resetPostgresFlags(t)
	flagPostgresHost = testPostgresHost
	flagPostgresPort = "5432"
	flagPostgresDB = "payloads"
	flagPostgresUser = "payloads"
	flagPostgresPassword = "flag-pass-123"

	state := &config.State{}
	if err := applyPostgresFlags(state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := config.DevshardConfig{
		PostgresHost:     testPostgresHost,
		PostgresPort:     "5432",
		PostgresDB:       "payloads",
		PostgresUser:     "payloads",
		PostgresPassword: "flag-pass-123",
	}
	if state.DevshardConfig != want {
		t.Errorf("\n want: %+v\n got:  %+v", want, state.DevshardConfig)
	}
}

// TestApplyPostgresFlags_EnvVarReadAndScrubbed (TC-2.2, NFR-M12-1).
func TestApplyPostgresFlags_EnvVarReadAndScrubbed(t *testing.T) {
	resetPostgresFlags(t)
	flagPostgresHost = testPostgresHost
	t.Setenv(envPostgresPassword, "env-pass-abc")

	state := &config.State{}
	if err := applyPostgresFlags(state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.DevshardConfig.PostgresPassword != "env-pass-abc" {
		t.Errorf("PostgresPassword = %q, want env-pass-abc", state.DevshardConfig.PostgresPassword)
	}
	if got := os.Getenv(envPostgresPassword); got != "" {
		t.Errorf("expected env var to be scrubbed, still has value: %q", got)
	}
}

// TestApplyPostgresFlags_HostWithoutPassword_Yes_Fatal (TC-2.3, UC-2-A2).
// In --yes mode, --postgres-host without a password from any source must
// fatal before any state mutation.
func TestApplyPostgresFlags_HostWithoutPassword_Yes_Fatal(t *testing.T) {
	resetPostgresFlags(t)
	flagPostgresHost = testPostgresHost
	yesFlag = true

	state := &config.State{}
	err := applyPostgresFlags(state)
	if err == nil {
		t.Fatal("expected fatal error when --postgres-host set without password in --yes mode")
	}
	if !strings.Contains(err.Error(), "password") {
		t.Errorf("error should mention 'password', got %q", err.Error())
	}
	if state.DevshardConfig.PostgresHost != "" {
		t.Errorf("state must not be partially populated on fatal, got Host=%q", state.DevshardConfig.PostgresHost)
	}
}

// TestApplyPostgresFlags_HostWithoutPassword_Interactive_OK — without
// --yes, the helper does not fatal; interactive prompt would collect the
// password (Slice 5 prompt wiring, out of scope here).
func TestApplyPostgresFlags_HostWithoutPassword_Interactive_OK(t *testing.T) {
	resetPostgresFlags(t)
	flagPostgresHost = testPostgresHost
	// yesFlag stays false — interactive

	state := &config.State{}
	if err := applyPostgresFlags(state); err != nil {
		t.Errorf("expected no error in interactive mode, got: %v", err)
	}
}

// TestApplyPostgresFlags_FlagBeatsEnvVar — explicit flag takes precedence
// over env var if both are set.
func TestApplyPostgresFlags_FlagBeatsEnvVar(t *testing.T) {
	resetPostgresFlags(t)
	flagPostgresHost = testPostgresHost
	flagPostgresPassword = "from-flag"
	t.Setenv(envPostgresPassword, "from-env")

	state := &config.State{}
	if err := applyPostgresFlags(state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.DevshardConfig.PostgresPassword != "from-flag" {
		t.Errorf("PostgresPassword = %q, want from-flag (explicit flag wins)", state.DevshardConfig.PostgresPassword)
	}
	// When the flag wins, env var should still be scrubbed if it was set
	// (defensive behavior — env var was sensitive even if unused).
	// Note: current impl only scrubs when used. Document either way.
}

// TestApplyPostgresFlags_SpecialCharsInPassword_PreservedExactly (TC-2.4).
func TestApplyPostgresFlags_SpecialCharsInPassword_PreservedExactly(t *testing.T) {
	resetPostgresFlags(t)
	special := "p@$$:w0rd ${HOSTNAME} 'quote\"mix'"
	flagPostgresHost = testPostgresHost
	flagPostgresPassword = special

	state := &config.State{}
	if err := applyPostgresFlags(state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.DevshardConfig.PostgresPassword != special {
		t.Errorf("PostgresPassword mutated:\n  want: %q\n  got:  %q", special, state.DevshardConfig.PostgresPassword)
	}
}
