// M12 Slice 4 tests: DevshardConfig backward-compat contract (FR-M12-14)
// and round-trip persistence. Pre-M12 state.json files MUST load with
// zero-value DevshardConfig and MUST NOT be rewritten on load.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestState_PreM12Json_LoadsWithEmptyDevshardConfig (TC-STATE-1, AC-M12-8).
// A state.json written by a pre-M12 NOP build (no devshard_config key)
// must load successfully with DevshardConfig at zero value.
func TestState_PreM12Json_LoadsWithEmptyDevshardConfig(t *testing.T) {
	tmpDir := t.TempDir()
	fixture, err := os.ReadFile("testdata/state_pre_m12.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dst := filepath.Join(tmpDir, "state.json")
	if err := os.WriteFile(dst, fixture, 0600); err != nil {
		t.Fatalf("place fixture: %v", err)
	}

	beforeMtime := mtime(t, dst)
	// Small sleep so any rewrite would produce a different mtime
	time.Sleep(10 * time.Millisecond)

	state, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state.DevshardConfig.PostgresHost != "" {
		t.Errorf("PostgresHost = %q, want empty", state.DevshardConfig.PostgresHost)
	}
	if state.DevshardConfig.PostgresPassword != "" {
		t.Errorf("PostgresPassword = %q, want empty", state.DevshardConfig.PostgresPassword)
	}

	// Pre-existing fields must still load
	if state.Network != "mainnet" {
		t.Errorf("Network = %q, want mainnet (existing field load)", state.Network)
	}

	// FR-M12-14: must NOT rewrite the file on load
	if got := mtime(t, dst); !got.Equal(beforeMtime) {
		t.Errorf("state.json was rewritten on load (mtime changed: %v -> %v)", beforeMtime, got)
	}
}

// TestState_M12RoundTrip_PreservesDevshardConfig (TC-STATE-2).
func TestState_M12RoundTrip_PreservesDevshardConfig(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewState(tmpDir)
	s.Network = testNetworkMainnet
	s.KeyWorkflow = testKeyWorkflowQuick
	s.DevshardConfig = DevshardConfig{
		PostgresHost:     "db.example.com",
		PostgresPort:     "5432",
		PostgresDB:       "payloads",
		PostgresUser:     "payloads",
		PostgresPassword: "rt-secret-789",
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.DevshardConfig != s.DevshardConfig {
		t.Errorf("round-trip mismatch:\n  want: %+v\n  got:  %+v", s.DevshardConfig, loaded.DevshardConfig)
	}
}

// TestState_PartialPostgresConfig_StoredAsIs (TC-STATE-3). Only some
// Postgres fields populated should round-trip with the unset fields
// remaining empty (defaults are applied at compose-render time by Slice 5,
// not here).
func TestState_PartialPostgresConfig_StoredAsIs(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewState(tmpDir)
	s.DevshardConfig = DevshardConfig{
		PostgresHost: "db.example.com",
		// Port/DB/User/Password intentionally omitted
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.DevshardConfig.PostgresHost != "db.example.com" {
		t.Errorf("PostgresHost = %q, want db.example.com", loaded.DevshardConfig.PostgresHost)
	}
	if loaded.DevshardConfig.PostgresPort != "" {
		t.Errorf("PostgresPort = %q, want empty (no default applied at storage layer)", loaded.DevshardConfig.PostgresPort)
	}
}

// TestState_SaveFileMode_Is0600 (TC-SEC-7) — state.json mode must be
// 0600 because it contains KeyringPassword and may contain
// PostgresPassword (NFR-M12-1 + R-M12-5).
func TestState_SaveFileMode_Is0600(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewState(tmpDir)
	s.DevshardConfig.PostgresPassword = "sec-mode-test"
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(tmpDir, "state.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("state.json mode = %o, want 0600 (NFR-M12-1, R-M12-5)", got)
	}
}

// TestState_Reset_ClearsDevshardConfig — Reset() must zero out the new
// DevshardConfig along with the rest of the state.
func TestState_Reset_ClearsDevshardConfig(t *testing.T) {
	s := NewState(t.TempDir())
	s.DevshardConfig = DevshardConfig{
		PostgresHost:     "db",
		PostgresPassword: "secret",
	}
	s.Reset()
	if s.DevshardConfig != (DevshardConfig{}) {
		t.Errorf("Reset did not clear DevshardConfig, got %+v", s.DevshardConfig)
	}
}

// TestState_JSON_EmptyDevshardConfig_NoPostgresFieldsLeak — backward
// compatibility for the wire format: an unset DevshardConfig may still
// appear as `"devshard_config":{}` (Go's omitempty does not zero-check
// inner struct types) but MUST NOT contain any populated postgres_*
// fields. Pre-M12 readers ignore the unknown key; we only need to ensure
// no sensitive fields are accidentally serialized at zero state.
func TestState_JSON_EmptyDevshardConfig_NoPostgresFieldsLeak(t *testing.T) {
	s := NewState("/tmp/x")
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(raw)
	for _, leak := range []string{
		`"postgres_host"`, `"postgres_port"`, `"postgres_db"`,
		`"postgres_user"`, `"postgres_password"`,
	} {
		if contains(got, leak) {
			t.Errorf("empty DevshardConfig leaked field %s in JSON: %s", leak, got)
		}
	}
}

// helpers

func mtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime()
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
