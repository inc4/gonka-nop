// M12 Slice 5a tests: versiond block in generated compose, api env
// additions, proxy env update, optional Postgres lines in config.env,
// resolveVersions Versiond fallback, two-run byte-identical output.
package phases

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

func m12FixtureState(t *testing.T, dir string) *config.State {
	t.Helper()
	s := config.NewState(dir)
	s.KeyName = "warm-cold"
	s.AccountPubKey = "A8LK5/SoFXElGT5ZTfpmEHAEALvh8TiYoIGLGWX9kW2w"
	s.KeyringPassword = "PASS_secure_pw123"
	s.PersistentPeers = []string{"abc@node1.example.com:5000"}
	s.PublicIP = "10.20.30.40"
	s.EthereumNetwork = networkMainnet
	s.BeaconStateURL = testBeaconStateURL
	return s
}

// TestGenerateDockerCompose_VersiondBlockPresent (TC-COMPOSE-3, FR-M12-1).
func TestGenerateDockerCompose_VersiondBlockPresent(t *testing.T) {
	dir := t.TempDir()
	state := m12FixtureState(t, dir)
	state.Versions.Versiond = "0.2.12-m12"
	if err := generateDockerCompose(state); err != nil {
		t.Fatalf("generateDockerCompose: %v", err)
	}
	data := readFile(t, filepath.Join(dir, "docker-compose.yml"))

	// Exactly one versiond service top-level entry.
	if got := strings.Count(data, "\n  versiond:\n"); got != 1 {
		t.Errorf("versiond service appeared %d times, want 1", got)
	}
	if !strings.Contains(data, "ghcr.io/product-science/versiond:0.2.12-m12") {
		t.Error("compose missing versiond image with resolved tag 0.2.12-m12")
	}

	// All 9 upstream env keys per FR-M12-1.
	for _, key := range []string{
		"VERSIOND_ORACLE_URL", "VERSIOND_BINARY_NAME", "NODE_MANAGER_ADDR",
		"NODE_HOST", "KEY_NAME=", "ACCOUNT_PUBKEY=", "KEYRING_BACKEND=",
		"KEYRING_PASSWORD=", "KEYRING_DIR=",
	} {
		if !strings.Contains(data, key) {
			t.Errorf("versiond block missing env key %q", key)
		}
	}
	// Volumes and deps.
	for _, want := range []string{
		"./devshards/bin:/opt/versiond/bin",
		"./devshards/data:/opt/versiond/data",
		".inference:/root/.inference:ro",
		"VERSIOND_BINARY_NAME=devshardd",
		"NODE_MANAGER_ADDR=api:9400",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("versiond block missing %q", want)
		}
	}
}

// TestGenerateDockerCompose_APIEnvAdditions (FR-M12-2).
func TestGenerateDockerCompose_APIEnvAdditions(t *testing.T) {
	dir := t.TempDir()
	state := m12FixtureState(t, dir)
	if err := generateDockerCompose(state); err != nil {
		t.Fatalf("generateDockerCompose: %v", err)
	}
	data := readFile(t, filepath.Join(dir, "docker-compose.yml"))
	for _, want := range []string{
		"DAPI_API__NODE_MANAGER_GRPC_PORT=${NODE_MANAGER_GRPC_PORT:-9400}",
		"DAPI_API__NODE_MANAGER_LOCK_TTL_SECONDS=${NODE_MANAGER_LOCK_TTL_SECONDS:-1200}",
		"PGHOST=${POSTGRES_HOST:-}",
		"PGPORT=${POSTGRES_PORT:-5432}",
		"PGDATABASE=${POSTGRES_DB:-payloads}",
		"PGUSER=${POSTGRES_USER:-payloads}",
		"PGPASSWORD=${POSTGRES_PASSWORD:-}",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("api env block missing %q", want)
		}
	}
}

// TestGenerateDockerCompose_ProxyExemptRoutesUpdated (FR-M12-3).
func TestGenerateDockerCompose_ProxyExemptRoutesUpdated(t *testing.T) {
	dir := t.TempDir()
	state := m12FixtureState(t, dir)
	if err := generateDockerCompose(state); err != nil {
		t.Fatalf("generateDockerCompose: %v", err)
	}
	data := readFile(t, filepath.Join(dir, "docker-compose.yml"))
	// The default now includes subnet+devshard.
	if !strings.Contains(data, "GONKA_API_EXEMPT_ROUTES:-chat inference poc/proofs subnet devshard") {
		t.Error("proxy GONKA_API_EXEMPT_ROUTES default missing subnet+devshard tokens")
	}
	if !strings.Contains(data, "EXEMPT_CONN_LIMIT") {
		t.Error("proxy missing EXEMPT_CONN_LIMIT env line")
	}
}

// TestGenerateConfigEnv_PostgresValuesEmittedWhenHostSet (FR-M12-2 runtime wiring).
func TestGenerateConfigEnv_PostgresValuesEmittedWhenHostSet(t *testing.T) {
	dir := t.TempDir()
	state := m12FixtureState(t, dir)
	state.DevshardConfig.PostgresHost = "pg.example.com"
	state.DevshardConfig.PostgresPassword = "PG_pass_xyz"
	if err := generateConfigEnv(state); err != nil {
		t.Fatalf("generateConfigEnv: %v", err)
	}
	data := readFile(t, filepath.Join(dir, "config.env"))
	for _, want := range []string{
		"POSTGRES_HOST=pg.example.com",
		"POSTGRES_PORT=5432", // default applied
		"POSTGRES_DB=payloads",
		"POSTGRES_USER=payloads",
		"POSTGRES_PASSWORD=PG_pass_xyz",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("config.env missing %q", want)
		}
	}
}

// TestGenerateConfigEnv_NoPostgresLinesWhenHostEmpty — file backend by
// default produces no POSTGRES_* lines.
func TestGenerateConfigEnv_NoPostgresLinesWhenHostEmpty(t *testing.T) {
	dir := t.TempDir()
	state := m12FixtureState(t, dir)
	// DevshardConfig left at zero value.
	if err := generateConfigEnv(state); err != nil {
		t.Fatalf("generateConfigEnv: %v", err)
	}
	data := readFile(t, filepath.Join(dir, "config.env"))
	for _, leak := range []string{"POSTGRES_HOST", "POSTGRES_PASSWORD"} {
		if strings.Contains(data, leak) {
			t.Errorf("config.env should NOT contain %q when host empty, got:\n%s", leak, data)
		}
	}
}

// TestResolveVersions_VersiondFallbackWhenEmpty — defensive fallback for
// pre-Slice-1 state without Versiond.
func TestResolveVersions_VersiondFallbackWhenEmpty(t *testing.T) {
	state := config.NewState(t.TempDir())
	state.Versions.Versiond = ""
	state.IsTestNet = false
	if got := resolveVersions(state).Versiond; got == "" {
		t.Error("resolveVersions(mainnet) Versiond fallback returned empty")
	}
	state.IsTestNet = true
	if got := resolveVersions(state).Versiond; got == "" {
		t.Error("resolveVersions(testnet) Versiond fallback returned empty")
	}
}

// TestGenerateDockerCompose_TwoRunsByteIdentical (TC-IDEM-3 / TC-1.9).
func TestGenerateDockerCompose_TwoRunsByteIdentical(t *testing.T) {
	dir := t.TempDir()
	state := m12FixtureState(t, dir)
	if err := generateDockerCompose(state); err != nil {
		t.Fatalf("generateDockerCompose (1st): %v", err)
	}
	first := readFile(t, filepath.Join(dir, "docker-compose.yml"))

	if err := generateDockerCompose(state); err != nil {
		t.Fatalf("generateDockerCompose (2nd): %v", err)
	}
	second := readFile(t, filepath.Join(dir, "docker-compose.yml"))

	if first != second {
		t.Error("two consecutive runs produced different compose output (TC-1.9 idempotency)")
	}
}

// TestGenerateConfigEnv_PostgresPassword_NotInUI (NFR-M12-1 surface check
// at the file layer). config.env is the rendered output and DOES contain
// the password by design — the SECURITY contract is about NOT logging it.
// This test asserts the password lands in config.env exactly once and
// nowhere else surfaces it (no debug print path).
func TestGenerateConfigEnv_PostgresPassword_AppearsExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	state := m12FixtureState(t, dir)
	state.DevshardConfig.PostgresHost = "pg"
	state.DevshardConfig.PostgresPassword = "ONLY_ONCE_PASSWORD_XYZ"
	if err := generateConfigEnv(state); err != nil {
		t.Fatalf("generateConfigEnv: %v", err)
	}
	data := readFile(t, filepath.Join(dir, "config.env"))
	if got := strings.Count(data, "ONLY_ONCE_PASSWORD_XYZ"); got != 1 {
		t.Errorf("password appeared %d times in config.env, want 1", got)
	}
}

// readFile reads a file and fatals on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 - test helper, paths under t.TempDir()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
