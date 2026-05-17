// Package devshard holds the verbatim upstream service blocks copied from
// gonka-ai/gonka:main/deploy/join/docker-compose.yml at PRD time (per
// PRD R-M12-1: hardcoded locally to avoid drift between FetchImageVersions
// snapshots and migrate-devshardd invocations), the substitution helper
// that renders them with state-derived values, and the image allowlist
// regex enforcing INV-M12-3/4.
//
// Security invariant: this package MUST NOT import log, log/slog,
// fmt printing functions, or internal/ui. The substitution path must
// never echo state values to any stream; all error messages MUST be
// field-name-only and MUST NOT contain state values (per security-auditor
// pre-review 2026-05-17).
package devshard

import (
	"errors"
	"strings"

	"github.com/inc4/gonka-nop/internal/config"
)

// Sentinel placeholders substituted by RenderVersiondBlock. Factored into
// constants to satisfy goconst (each appears in both VersiondServiceBlock
// and the substitution function) and to keep the substitution targets in
// a single auditable place.
const (
	phVersiondTag     = "${VERSIOND_TAG}"
	phKeyName         = "${KEY_NAME}"
	phAccountPubKey   = "${ACCOUNT_PUBKEY}"
	phKeyringPassword = "${KEYRING_PASSWORD}" // #nosec G101 - placeholder token name, NOT a credential value
)

// VersiondServiceBlock is the upstream-canonical versiond service definition.
//
// WARNING: rendered output contains plaintext KEYRING_PASSWORD; calling code
// (Slice 5 compose-template renderer) MUST write the final compose file at
// mode 0600. See INV-M12-2 — versiond and api share KEY_NAME/KEYRING_PASSWORD
// from a single source (state.json).
//
// Placeholders ${KEY_NAME}, ${ACCOUNT_PUBKEY}, ${KEYRING_PASSWORD},
// ${VERSIOND_TAG} are substituted by RenderVersiondBlock. The upstream
// ${KEYRING_BACKEND:-file} substitution is left for docker-compose to
// evaluate at container start.
const VersiondServiceBlock = `  versiond:
    container_name: versiond
    image: ghcr.io/product-science/versiond:${VERSIOND_TAG}
    environment:
      - VERSIOND_ORACLE_URL=http://api:9100/versions
      - VERSIOND_BINARY_NAME=devshardd
      - NODE_MANAGER_ADDR=api:9400
      - NODE_HOST=node
      - KEY_NAME=${KEY_NAME}
      - ACCOUNT_PUBKEY=${ACCOUNT_PUBKEY}
      - KEYRING_BACKEND=${KEYRING_BACKEND:-file}
      - KEYRING_PASSWORD=${KEYRING_PASSWORD}
      - KEYRING_DIR=/root/.inference
    volumes:
      - .inference:/root/.inference:ro
      - ./devshards/bin:/opt/versiond/bin
      - ./devshards/data:/opt/versiond/data
    depends_on:
      - api
    restart: always
`

// APIEnvAdditions are the 7 env vars upstream adds to the api service for
// devshard integration (FR-M12-2). Values use upstream's ${VAR:-default}
// substitution syntax — docker compose evaluates them at container start
// from the operator's shell env (set by NOP at compose-up time from
// state.DevshardConfig values via the upcoming Slice 5 wiring).
const APIEnvAdditions = `      - DAPI_API__NODE_MANAGER_GRPC_PORT=${NODE_MANAGER_GRPC_PORT:-9400}
      - DAPI_API__NODE_MANAGER_LOCK_TTL_SECONDS=${NODE_MANAGER_LOCK_TTL_SECONDS:-1200}
      - PGHOST=${POSTGRES_HOST:-}
      - PGPORT=${POSTGRES_PORT:-5432}
      - PGDATABASE=${POSTGRES_DB:-payloads}
      - PGUSER=${POSTGRES_USER:-payloads}
      - PGPASSWORD=${POSTGRES_PASSWORD:-}
`

// ProxyEnvAdditions are the 2 env additions upstream applies to the proxy
// service (FR-M12-3). GONKA_API_EXEMPT_ROUTES is the full upstream value
// (already including subnet+devshard tokens); EXEMPT_CONN_LIMIT is the
// upstream default. The compose-template renderer in Slice 5 replaces the
// existing proxy EXEMPT_ROUTES line with the version below.
const ProxyEnvAdditions = `      - GONKA_API_EXEMPT_ROUTES=chat inference poc/proofs subnet devshard
      - EXEMPT_CONN_LIMIT=200
`

// RenderVersiondBlock substitutes state-derived placeholders in
// VersiondServiceBlock and returns the rendered YAML snippet ready to
// insert into a compose file.
//
// IMPORTANT: the returned YAML contains plaintext KEYRING_PASSWORD —
// callers MUST write the final compose at mode 0600. Same convention as
// the existing api service env (INV-M12-2).
//
// Substitution uses strings.ReplaceAll against fixed ${...} tokens — NOT
// os.Expand or text/template — so state values may safely contain literal
// `$` characters (e.g., a password with `$1$` bcrypt-style markers) without
// shell-expansion surprises.
//
// All required state fields (KeyName, AccountPubKey, KeyringPassword) and
// the versiondTag must be non-empty; otherwise an error naming the missing
// field is returned. Error messages NEVER contain state values.
func RenderVersiondBlock(state *config.State, versiondTag string) (string, error) {
	if state == nil {
		return "", errors.New("state is nil")
	}
	if versiondTag == "" {
		return "", errors.New("versiond tag is empty")
	}
	if state.KeyName == "" {
		return "", errors.New("state.KeyName is required for versiond rendering (complete phase 04 key management first)")
	}
	if state.AccountPubKey == "" {
		return "", errors.New("state.AccountPubKey is required for versiond rendering")
	}
	if state.KeyringPassword == "" {
		return "", errors.New("state.KeyringPassword is required for versiond rendering")
	}

	out := VersiondServiceBlock
	out = strings.ReplaceAll(out, phVersiondTag, versiondTag)
	out = strings.ReplaceAll(out, phKeyName, state.KeyName)
	out = strings.ReplaceAll(out, phAccountPubKey, state.AccountPubKey)
	out = strings.ReplaceAll(out, phKeyringPassword, state.KeyringPassword)
	return out, nil
}
